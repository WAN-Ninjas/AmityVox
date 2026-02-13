package api

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"

	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/models"
	"github.com/amityvox/amityvox/internal/permissions"
	"github.com/amityvox/amityvox/internal/voice"
)

// newULID generates a new ULID for resource IDs.
func newVoiceULID() string {
	return ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader).String()
}

// liveKitPublicURL returns the browser-facing LiveKit URL, falling back to the
// internal URL if no public URL is configured.
func (s *Server) liveKitPublicURL() string {
	if s.Config.LiveKit.PublicURL != "" {
		return s.Config.LiveKit.PublicURL
	}
	return s.Config.LiveKit.URL
}

// handleVoiceJoin generates a LiveKit token for a user to join a voice channel.
// POST /api/v1/voice/{channelID}/join
func (s *Server) handleVoiceJoin(w http.ResponseWriter, r *http.Request) {
	if s.Voice == nil {
		WriteError(w, http.StatusServiceUnavailable, "voice_disabled", "Voice is not enabled on this instance")
		return
	}

	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	// Verify channel exists and is a voice channel.
	var channelType string
	var guildID *string
	err := s.DB.Pool.QueryRow(r.Context(),
		`SELECT channel_type, guild_id FROM channels WHERE id = $1`, channelID,
	).Scan(&channelType, &guildID)
	if err == pgx.ErrNoRows {
		WriteError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
		return
	}
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get channel")
		return
	}

	if channelType != models.ChannelTypeVoice && channelType != models.ChannelTypeStage {
		WriteError(w, http.StatusBadRequest, "not_voice_channel", "This is not a voice channel")
		return
	}

	// Check Connect permission.
	if guildID != nil {
		if !checkGuildPerm(r.Context(), s.DB.Pool, *guildID, userID, permissions.Connect) {
			WriteError(w, http.StatusForbidden, "missing_permission", "You need CONNECT permission")
			return
		}
	}

	// Check Speak permission for publish rights.
	canSpeak := true
	if guildID != nil {
		canSpeak = checkGuildPerm(r.Context(), s.DB.Pool, *guildID, userID, permissions.Speak)
	}

	// Ensure the LiveKit room exists.
	if err := s.Voice.EnsureRoom(r.Context(), channelID); err != nil {
		s.Logger.Error("failed to ensure voice room", "error", err.Error())
	}

	// Fetch user profile for LiveKit participant metadata.
	var username string
	var displayName, avatarID *string
	err = s.DB.Pool.QueryRow(r.Context(),
		`SELECT username, display_name, avatar_id FROM users WHERE id = $1`, userID,
	).Scan(&username, &displayName, &avatarID)
	if err != nil {
		s.Logger.Error("failed to fetch user for voice metadata", "error", err.Error())
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to fetch user")
		return
	}

	metaMap := map[string]interface{}{
		"userId":   userID,
		"username": username,
	}
	if displayName != nil {
		metaMap["displayName"] = *displayName
	}
	if avatarID != nil {
		metaMap["avatarId"] = *avatarID
	}
	metaBytes, _ := json.Marshal(metaMap)

	// Generate LiveKit token.
	token, err := s.Voice.GenerateToken(userID, channelID, canSpeak, true, canSpeak, string(metaBytes))
	if err != nil {
		s.Logger.Error("failed to generate voice token", "error", err.Error())
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to generate voice token")
		return
	}

	// Update voice state.
	gID := ""
	if guildID != nil {
		gID = *guildID
	}
	s.Voice.UpdateVoiceState(userID, gID, channelID, false, false)

	// Publish VOICE_STATE_UPDATE event with user info for other clients.
	voiceEvent := map[string]interface{}{
		"user_id":    userID,
		"guild_id":   gID,
		"channel_id": channelID,
		"username":   username,
		"self_mute":  false,
		"self_deaf":  false,
		"action":     "join",
	}
	if displayName != nil {
		voiceEvent["display_name"] = *displayName
	}
	if avatarID != nil {
		voiceEvent["avatar_id"] = *avatarID
	}
	s.EventBus.PublishJSON(r.Context(), events.SubjectVoiceStateUpdate, "VOICE_STATE_UPDATE", voiceEvent)

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"token":      token,
		"url":        s.liveKitPublicURL(),
		"channel_id": channelID,
	})
}

// handleVoiceLeave disconnects a user from a voice channel.
// POST /api/v1/voice/{channelID}/leave
func (s *Server) handleVoiceLeave(w http.ResponseWriter, r *http.Request) {
	if s.Voice == nil {
		WriteError(w, http.StatusServiceUnavailable, "voice_disabled", "Voice is not enabled on this instance")
		return
	}

	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	// Get guild ID for the event.
	var guildID *string
	s.DB.Pool.QueryRow(r.Context(),
		`SELECT guild_id FROM channels WHERE id = $1`, channelID).Scan(&guildID)

	gID := ""
	if guildID != nil {
		gID = *guildID
	}

	// Remove participant from LiveKit if possible.
	_ = s.Voice.RemoveParticipant(r.Context(), channelID, userID)

	// Clear voice state.
	s.Voice.UpdateVoiceState(userID, gID, "", false, false)

	// Publish VOICE_STATE_UPDATE with nil channel (disconnected).
	s.EventBus.PublishJSON(r.Context(), events.SubjectVoiceStateUpdate, "VOICE_STATE_UPDATE", map[string]interface{}{
		"user_id":    userID,
		"guild_id":   gID,
		"channel_id": nil,
	})

	WriteNoContent(w)
}

// handleVoiceServerMute server-mutes/unmutes a user in a voice channel.
// POST /api/v1/voice/{channelID}/members/{userID}/mute
func (s *Server) handleVoiceServerMute(w http.ResponseWriter, r *http.Request) {
	if s.Voice == nil {
		WriteError(w, http.StatusServiceUnavailable, "voice_disabled", "Voice is not enabled on this instance")
		return
	}

	actorID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	targetUserID := chi.URLParam(r, "userID")

	// Get guild ID.
	var guildID *string
	err := s.DB.Pool.QueryRow(r.Context(),
		`SELECT guild_id FROM channels WHERE id = $1`, channelID).Scan(&guildID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
		return
	}
	if guildID == nil {
		WriteError(w, http.StatusBadRequest, "not_guild_channel", "Cannot server-mute in DM channels")
		return
	}

	// Check MuteMembers permission.
	if !checkGuildPerm(r.Context(), s.DB.Pool, *guildID, actorID, permissions.MuteMembers) {
		WriteError(w, http.StatusForbidden, "missing_permission", "You need MUTE_MEMBERS permission")
		return
	}

	// Parse request body.
	var req struct {
		Muted bool `json:"muted"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	// Check target is in this voice channel.
	vs := s.Voice.GetVoiceState(targetUserID)
	if vs == nil || vs.ChannelID != channelID {
		WriteError(w, http.StatusBadRequest, "not_in_channel", "Target user is not in this voice channel")
		return
	}

	// Apply mute via LiveKit.
	if err := s.Voice.MuteParticipant(r.Context(), channelID, targetUserID, req.Muted); err != nil {
		s.Logger.Error("failed to mute participant", "error", err.Error())
	}

	// Update voice state.
	s.Voice.SetServerMute(targetUserID, req.Muted)

	// Publish VOICE_STATE_UPDATE.
	s.EventBus.PublishJSON(r.Context(), events.SubjectVoiceStateUpdate, "VOICE_STATE_UPDATE", map[string]interface{}{
		"user_id":    targetUserID,
		"guild_id":   *guildID,
		"channel_id": channelID,
		"self_mute":  vs.SelfMute,
		"self_deaf":  vs.SelfDeaf,
		"muted":      req.Muted,
		"deafened":   vs.Deafened,
	})

	WriteNoContent(w)
}

// handleVoiceServerDeafen server-deafens/undeafens a user in a voice channel.
// POST /api/v1/voice/{channelID}/members/{userID}/deafen
func (s *Server) handleVoiceServerDeafen(w http.ResponseWriter, r *http.Request) {
	if s.Voice == nil {
		WriteError(w, http.StatusServiceUnavailable, "voice_disabled", "Voice is not enabled on this instance")
		return
	}

	actorID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	targetUserID := chi.URLParam(r, "userID")

	// Get guild ID.
	var guildID *string
	err := s.DB.Pool.QueryRow(r.Context(),
		`SELECT guild_id FROM channels WHERE id = $1`, channelID).Scan(&guildID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
		return
	}
	if guildID == nil {
		WriteError(w, http.StatusBadRequest, "not_guild_channel", "Cannot server-deafen in DM channels")
		return
	}

	// Check DeafenMembers permission.
	if !checkGuildPerm(r.Context(), s.DB.Pool, *guildID, actorID, permissions.DeafenMembers) {
		WriteError(w, http.StatusForbidden, "missing_permission", "You need DEAFEN_MEMBERS permission")
		return
	}

	// Parse request body.
	var req struct {
		Deafened bool `json:"deafened"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	// Check target is in this voice channel.
	vs := s.Voice.GetVoiceState(targetUserID)
	if vs == nil || vs.ChannelID != channelID {
		WriteError(w, http.StatusBadRequest, "not_in_channel", "Target user is not in this voice channel")
		return
	}

	// Apply deafen via LiveKit.
	if err := s.Voice.DeafenParticipant(r.Context(), channelID, targetUserID, req.Deafened); err != nil {
		s.Logger.Error("failed to deafen participant", "error", err.Error())
	}

	// Update voice state.
	s.Voice.SetServerDeafen(targetUserID, req.Deafened)

	// Publish VOICE_STATE_UPDATE.
	s.EventBus.PublishJSON(r.Context(), events.SubjectVoiceStateUpdate, "VOICE_STATE_UPDATE", map[string]interface{}{
		"user_id":    targetUserID,
		"guild_id":   *guildID,
		"channel_id": channelID,
		"self_mute":  vs.SelfMute,
		"self_deaf":  vs.SelfDeaf,
		"muted":      vs.Muted,
		"deafened":   req.Deafened,
	})

	WriteNoContent(w)
}

// handleVoiceMoveUser moves a user from one voice channel to another.
// POST /api/v1/voice/{channelID}/members/{userID}/move
func (s *Server) handleVoiceMoveUser(w http.ResponseWriter, r *http.Request) {
	if s.Voice == nil {
		WriteError(w, http.StatusServiceUnavailable, "voice_disabled", "Voice is not enabled on this instance")
		return
	}

	actorID := auth.UserIDFromContext(r.Context())
	sourceChannelID := chi.URLParam(r, "channelID")
	targetUserID := chi.URLParam(r, "userID")

	// Get guild ID from source channel.
	var guildID *string
	err := s.DB.Pool.QueryRow(r.Context(),
		`SELECT guild_id FROM channels WHERE id = $1`, sourceChannelID).Scan(&guildID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "channel_not_found", "Source channel not found")
		return
	}
	if guildID == nil {
		WriteError(w, http.StatusBadRequest, "not_guild_channel", "Cannot move members in DM channels")
		return
	}

	// Check MoveMembers permission.
	if !checkGuildPerm(r.Context(), s.DB.Pool, *guildID, actorID, permissions.MoveMembers) {
		WriteError(w, http.StatusForbidden, "missing_permission", "You need MOVE_MEMBERS permission")
		return
	}

	// Parse request body.
	var req struct {
		TargetChannelID string `json:"target_channel_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TargetChannelID == "" {
		WriteError(w, http.StatusBadRequest, "invalid_body", "target_channel_id is required")
		return
	}

	// Verify target channel exists, is in the same guild, and is voice/stage.
	var targetType string
	var targetGuildID *string
	err = s.DB.Pool.QueryRow(r.Context(),
		`SELECT channel_type, guild_id FROM channels WHERE id = $1`, req.TargetChannelID,
	).Scan(&targetType, &targetGuildID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "channel_not_found", "Target channel not found")
		return
	}
	if targetGuildID == nil || *targetGuildID != *guildID {
		WriteError(w, http.StatusBadRequest, "wrong_guild", "Target channel must be in the same guild")
		return
	}
	if targetType != models.ChannelTypeVoice && targetType != models.ChannelTypeStage {
		WriteError(w, http.StatusBadRequest, "not_voice_channel", "Target must be a voice or stage channel")
		return
	}

	// Check target is in the source voice channel.
	vs := s.Voice.GetVoiceState(targetUserID)
	if vs == nil || vs.ChannelID != sourceChannelID {
		WriteError(w, http.StatusBadRequest, "not_in_channel", "Target user is not in the source voice channel")
		return
	}

	// Remove from old room.
	_ = s.Voice.RemoveParticipant(r.Context(), sourceChannelID, targetUserID)

	// Ensure target room exists.
	_ = s.Voice.EnsureRoom(r.Context(), req.TargetChannelID)

	// Update voice state.
	s.Voice.UpdateVoiceState(targetUserID, *guildID, req.TargetChannelID, vs.SelfMute, vs.SelfDeaf)

	// Publish VOICE_STATE_UPDATE for the move (new channel).
	s.EventBus.PublishJSON(r.Context(), events.SubjectVoiceStateUpdate, "VOICE_STATE_UPDATE", map[string]interface{}{
		"user_id":    targetUserID,
		"guild_id":   *guildID,
		"channel_id": req.TargetChannelID,
		"self_mute":  vs.SelfMute,
		"self_deaf":  vs.SelfDeaf,
		"muted":      vs.Muted,
		"deafened":   vs.Deafened,
	})

	// Generate a new token for the target channel so the client can reconnect.
	canSpeak := checkGuildPerm(r.Context(), s.DB.Pool, *guildID, targetUserID, permissions.Speak)
	token, err := s.Voice.GenerateToken(targetUserID, req.TargetChannelID, canSpeak, true, canSpeak, "")
	if err != nil {
		s.Logger.Error("failed to generate move token", "error", err.Error())
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to generate voice token for move")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"token":      token,
		"url":        s.liveKitPublicURL(),
		"channel_id": req.TargetChannelID,
	})
}

// handleGetVoiceStates returns the current voice states for a channel.
// GET /api/v1/voice/{channelID}/states
func (s *Server) handleGetVoiceStates(w http.ResponseWriter, r *http.Request) {
	if s.Voice == nil {
		WriteError(w, http.StatusServiceUnavailable, "voice_disabled", "Voice is not enabled on this instance")
		return
	}

	channelID := chi.URLParam(r, "channelID")
	states := s.Voice.GetChannelVoiceStates(channelID)
	if states == nil {
		WriteJSON(w, http.StatusOK, []struct{}{})
		return
	}

	WriteJSON(w, http.StatusOK, states)
}

// --- Voice Preferences Handlers ---

// handleGetVoicePreferences returns the current user's voice preferences.
// GET /api/v1/voice/preferences
func (s *Server) handleGetVoicePreferences(w http.ResponseWriter, r *http.Request) {
	if s.Voice == nil {
		WriteError(w, http.StatusServiceUnavailable, "voice_disabled", "Voice is not enabled on this instance")
		return
	}

	userID := auth.UserIDFromContext(r.Context())
	prefs, err := s.Voice.GetVoicePreferences(r.Context(), userID)
	if err != nil {
		s.Logger.Error("failed to get voice preferences", "error", err.Error())
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get voice preferences")
		return
	}

	WriteJSON(w, http.StatusOK, prefs)
}

// handleUpdateVoicePreferences updates the current user's voice preferences.
// PATCH /api/v1/voice/preferences
func (s *Server) handleUpdateVoicePreferences(w http.ResponseWriter, r *http.Request) {
	if s.Voice == nil {
		WriteError(w, http.StatusServiceUnavailable, "voice_disabled", "Voice is not enabled on this instance")
		return
	}

	userID := auth.UserIDFromContext(r.Context())

	var req struct {
		InputMode        *string  `json:"input_mode"`
		PTTKey           *string  `json:"ptt_key"`
		VADThreshold     *float64 `json:"vad_threshold"`
		NoiseSuppression *bool    `json:"noise_suppression"`
		EchoCancellation *bool    `json:"echo_cancellation"`
		AutoGainControl  *bool    `json:"auto_gain_control"`
		InputVolume      *float64 `json:"input_volume"`
		OutputVolume     *float64 `json:"output_volume"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	// Validate input_mode if provided.
	if req.InputMode != nil && *req.InputMode != "vad" && *req.InputMode != "ptt" {
		WriteError(w, http.StatusBadRequest, "invalid_input_mode", "Input mode must be 'vad' or 'ptt'")
		return
	}

	// Validate thresholds.
	if req.VADThreshold != nil && (*req.VADThreshold < 0.0 || *req.VADThreshold > 1.0) {
		WriteError(w, http.StatusBadRequest, "invalid_threshold", "VAD threshold must be between 0.0 and 1.0")
		return
	}
	if req.InputVolume != nil && (*req.InputVolume < 0.0 || *req.InputVolume > 2.0) {
		WriteError(w, http.StatusBadRequest, "invalid_volume", "Input volume must be between 0.0 and 2.0")
		return
	}
	if req.OutputVolume != nil && (*req.OutputVolume < 0.0 || *req.OutputVolume > 2.0) {
		WriteError(w, http.StatusBadRequest, "invalid_volume", "Output volume must be between 0.0 and 2.0")
		return
	}

	// Load existing preferences and apply updates.
	prefs, err := s.Voice.GetVoicePreferences(r.Context(), userID)
	if err != nil {
		s.Logger.Error("failed to get voice preferences", "error", err.Error())
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get voice preferences")
		return
	}

	if req.InputMode != nil {
		prefs.InputMode = *req.InputMode
	}
	if req.PTTKey != nil {
		prefs.PTTKey = *req.PTTKey
	}
	if req.VADThreshold != nil {
		prefs.VADThreshold = *req.VADThreshold
	}
	if req.NoiseSuppression != nil {
		prefs.NoiseSuppression = *req.NoiseSuppression
	}
	if req.EchoCancellation != nil {
		prefs.EchoCancellation = *req.EchoCancellation
	}
	if req.AutoGainControl != nil {
		prefs.AutoGainControl = *req.AutoGainControl
	}
	if req.InputVolume != nil {
		prefs.InputVolume = *req.InputVolume
	}
	if req.OutputVolume != nil {
		prefs.OutputVolume = *req.OutputVolume
	}

	if err := s.Voice.UpdateVoicePreferences(r.Context(), prefs); err != nil {
		s.Logger.Error("failed to update voice preferences", "error", err.Error())
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update voice preferences")
		return
	}

	// Also update in-memory voice state if user is connected.
	s.Voice.SetInputMode(userID, prefs.InputMode)

	WriteJSON(w, http.StatusOK, prefs)
}

// handleSetInputMode updates the user's active voice input mode (PTT/VAD).
// POST /api/v1/voice/{channelID}/input-mode
func (s *Server) handleSetInputMode(w http.ResponseWriter, r *http.Request) {
	if s.Voice == nil {
		WriteError(w, http.StatusServiceUnavailable, "voice_disabled", "Voice is not enabled on this instance")
		return
	}

	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	var req struct {
		Mode string `json:"mode"` // "vad" or "ptt"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if req.Mode != "vad" && req.Mode != "ptt" {
		WriteError(w, http.StatusBadRequest, "invalid_mode", "Mode must be 'vad' or 'ptt'")
		return
	}

	// Verify user is in this channel.
	vs := s.Voice.GetVoiceState(userID)
	if vs == nil || vs.ChannelID != channelID {
		WriteError(w, http.StatusBadRequest, "not_in_channel", "You are not in this voice channel")
		return
	}

	s.Voice.SetInputMode(userID, req.Mode)

	// Get guild ID for event.
	gID := vs.GuildID

	// Publish VOICE_STATE_UPDATE with input mode.
	s.EventBus.PublishJSON(r.Context(), events.SubjectVoiceStateUpdate, "VOICE_STATE_UPDATE", map[string]interface{}{
		"user_id":    userID,
		"guild_id":   gID,
		"channel_id": channelID,
		"input_mode": req.Mode,
	})

	WriteNoContent(w)
}

// --- Priority Speaker Handlers ---

// handleSetPrioritySpeaker toggles priority speaker for a user in a voice channel.
// POST /api/v1/voice/{channelID}/members/{userID}/priority
func (s *Server) handleSetPrioritySpeaker(w http.ResponseWriter, r *http.Request) {
	if s.Voice == nil {
		WriteError(w, http.StatusServiceUnavailable, "voice_disabled", "Voice is not enabled on this instance")
		return
	}

	actorID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	targetUserID := chi.URLParam(r, "userID")

	// Get guild ID.
	var guildID *string
	err := s.DB.Pool.QueryRow(r.Context(),
		`SELECT guild_id FROM channels WHERE id = $1`, channelID).Scan(&guildID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
		return
	}
	if guildID == nil {
		WriteError(w, http.StatusBadRequest, "not_guild_channel", "Priority speaker requires a guild channel")
		return
	}

	// Check PrioritySpeaker permission. Users can set themselves if they have the perm,
	// or an admin can set others.
	if actorID != targetUserID {
		if !checkGuildPerm(r.Context(), s.DB.Pool, *guildID, actorID, permissions.MoveMembers) {
			WriteError(w, http.StatusForbidden, "missing_permission", "You need MOVE_MEMBERS permission to set others as priority speaker")
			return
		}
	} else {
		if !checkGuildPerm(r.Context(), s.DB.Pool, *guildID, actorID, permissions.PrioritySpeaker) {
			WriteError(w, http.StatusForbidden, "missing_permission", "You need PRIORITY_SPEAKER permission")
			return
		}
	}

	var req struct {
		Priority bool `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	// Verify target is in this channel.
	vs := s.Voice.GetVoiceState(targetUserID)
	if vs == nil || vs.ChannelID != channelID {
		WriteError(w, http.StatusBadRequest, "not_in_channel", "Target user is not in this voice channel")
		return
	}

	s.Voice.SetPrioritySpeaker(targetUserID, req.Priority)

	// Publish VOICE_STATE_UPDATE with priority speaker flag.
	s.EventBus.PublishJSON(r.Context(), events.SubjectVoiceStateUpdate, "VOICE_STATE_UPDATE", map[string]interface{}{
		"user_id":          targetUserID,
		"guild_id":         *guildID,
		"channel_id":       channelID,
		"priority_speaker": req.Priority,
	})

	WriteNoContent(w)
}

// --- Soundboard Handlers ---

// handleGetSoundboardSounds returns all sounds for a guild.
// GET /api/v1/guilds/{guildID}/soundboard/sounds
func (s *Server) handleGetSoundboardSounds(w http.ResponseWriter, r *http.Request) {
	if s.Voice == nil {
		WriteError(w, http.StatusServiceUnavailable, "voice_disabled", "Voice is not enabled on this instance")
		return
	}

	guildID := chi.URLParam(r, "guildID")
	userID := auth.UserIDFromContext(r.Context())

	// Verify membership.
	var exists bool
	s.DB.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM guild_members WHERE guild_id = $1 AND user_id = $2)`,
		guildID, userID).Scan(&exists)
	if !exists {
		WriteError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	sounds, err := s.Voice.GetSoundboardSounds(r.Context(), guildID)
	if err != nil {
		s.Logger.Error("failed to get soundboard sounds", "error", err.Error())
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get sounds")
		return
	}
	if sounds == nil {
		sounds = []voice.SoundboardSound{}
	}

	WriteJSON(w, http.StatusOK, sounds)
}

// handleCreateSoundboardSound creates a new soundboard sound.
// POST /api/v1/guilds/{guildID}/soundboard/sounds
func (s *Server) handleCreateSoundboardSound(w http.ResponseWriter, r *http.Request) {
	if s.Voice == nil {
		WriteError(w, http.StatusServiceUnavailable, "voice_disabled", "Voice is not enabled on this instance")
		return
	}

	guildID := chi.URLParam(r, "guildID")
	userID := auth.UserIDFromContext(r.Context())

	// Check ManageGuild permission for creating sounds.
	if !checkGuildPerm(r.Context(), s.DB.Pool, guildID, userID, permissions.ManageGuild) {
		WriteError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_GUILD permission to create sounds")
		return
	}

	var req struct {
		Name       string  `json:"name"`
		FileURL    string  `json:"file_url"`
		Volume     float64 `json:"volume"`
		DurationMs int     `json:"duration_ms"`
		Emoji      *string `json:"emoji,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Name == "" {
		WriteError(w, http.StatusBadRequest, "name_required", "Sound name is required")
		return
	}
	if req.FileURL == "" {
		WriteError(w, http.StatusBadRequest, "file_required", "File URL is required")
		return
	}
	if req.DurationMs <= 0 || req.DurationMs > 5000 {
		WriteError(w, http.StatusBadRequest, "invalid_duration", "Duration must be between 1 and 5000 milliseconds")
		return
	}
	if req.Volume <= 0 {
		req.Volume = 1.0
	}
	if req.Volume > 2.0 {
		req.Volume = 2.0
	}

	// Check sound limit.
	cfg, err := s.Voice.GetSoundboardConfig(r.Context(), guildID)
	if err != nil {
		s.Logger.Error("failed to get soundboard config", "error", err.Error())
	}
	count, err := s.Voice.CountSoundboardSounds(r.Context(), guildID)
	if err != nil {
		s.Logger.Error("failed to count sounds", "error", err.Error())
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to count sounds")
		return
	}
	if count >= cfg.MaxSounds {
		WriteError(w, http.StatusConflict, "sound_limit_reached",
			"This guild has reached its maximum number of soundboard sounds")
		return
	}

	sound := &voice.SoundboardSound{
		ID:         newVoiceULID(),
		GuildID:    guildID,
		Name:       req.Name,
		FileURL:    req.FileURL,
		Volume:     req.Volume,
		DurationMs: req.DurationMs,
		Emoji:      req.Emoji,
		CreatorID:  userID,
	}

	if err := s.Voice.CreateSoundboardSound(r.Context(), sound); err != nil {
		s.Logger.Error("failed to create soundboard sound", "error", err.Error())
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create sound")
		return
	}

	// Publish event for real-time updates.
	s.EventBus.PublishJSON(r.Context(), events.SubjectGuildUpdate, "SOUNDBOARD_SOUND_CREATE", map[string]interface{}{
		"guild_id": guildID,
		"sound":    sound,
	})

	WriteJSON(w, http.StatusCreated, sound)
}

// handleDeleteSoundboardSound deletes a soundboard sound.
// DELETE /api/v1/guilds/{guildID}/soundboard/sounds/{soundID}
func (s *Server) handleDeleteSoundboardSound(w http.ResponseWriter, r *http.Request) {
	if s.Voice == nil {
		WriteError(w, http.StatusServiceUnavailable, "voice_disabled", "Voice is not enabled on this instance")
		return
	}

	guildID := chi.URLParam(r, "guildID")
	soundID := chi.URLParam(r, "soundID")
	userID := auth.UserIDFromContext(r.Context())

	// Check ManageGuild permission.
	if !checkGuildPerm(r.Context(), s.DB.Pool, guildID, userID, permissions.ManageGuild) {
		WriteError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_GUILD permission to delete sounds")
		return
	}

	// Verify sound belongs to this guild.
	sound, err := s.Voice.GetSoundboardSound(r.Context(), soundID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "sound_not_found", "Sound not found")
		return
	}
	if sound.GuildID != guildID {
		WriteError(w, http.StatusNotFound, "sound_not_found", "Sound not found in this guild")
		return
	}

	if err := s.Voice.DeleteSoundboardSound(r.Context(), soundID); err != nil {
		s.Logger.Error("failed to delete soundboard sound", "error", err.Error())
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete sound")
		return
	}

	// Publish event.
	s.EventBus.PublishJSON(r.Context(), events.SubjectGuildUpdate, "SOUNDBOARD_SOUND_DELETE", map[string]interface{}{
		"guild_id": guildID,
		"sound_id": soundID,
	})

	WriteNoContent(w)
}

// handlePlaySoundboardSound plays a sound in the user's current voice channel.
// POST /api/v1/guilds/{guildID}/soundboard/sounds/{soundID}/play
func (s *Server) handlePlaySoundboardSound(w http.ResponseWriter, r *http.Request) {
	if s.Voice == nil {
		WriteError(w, http.StatusServiceUnavailable, "voice_disabled", "Voice is not enabled on this instance")
		return
	}

	guildID := chi.URLParam(r, "guildID")
	soundID := chi.URLParam(r, "soundID")
	userID := auth.UserIDFromContext(r.Context())

	// Verify user is in a voice channel in this guild.
	vs := s.Voice.GetVoiceState(userID)
	if vs == nil || vs.GuildID != guildID {
		WriteError(w, http.StatusBadRequest, "not_in_voice", "You must be in a voice channel in this guild")
		return
	}

	// Get soundboard config for cooldown.
	cfg, err := s.Voice.GetSoundboardConfig(r.Context(), guildID)
	if err != nil {
		s.Logger.Error("failed to get soundboard config", "error", err.Error())
	}
	if !cfg.Enabled {
		WriteError(w, http.StatusForbidden, "soundboard_disabled", "Soundboard is disabled in this guild")
		return
	}

	// Check cooldown.
	onCooldown, err := s.Voice.CheckSoundboardCooldown(r.Context(), userID, guildID, cfg.CooldownSeconds)
	if err != nil {
		s.Logger.Error("failed to check cooldown", "error", err.Error())
	}
	if onCooldown {
		WriteError(w, http.StatusTooManyRequests, "cooldown_active", "Please wait before playing another sound")
		return
	}

	// Verify sound exists and belongs to this guild.
	sound, err := s.Voice.GetSoundboardSound(r.Context(), soundID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "sound_not_found", "Sound not found")
		return
	}
	if sound.GuildID != guildID {
		WriteError(w, http.StatusNotFound, "sound_not_found", "Sound not found in this guild")
		return
	}

	// Log the play for cooldown tracking and increment play count.
	logID := newVoiceULID()
	_ = s.Voice.LogSoundboardPlay(r.Context(), logID, soundID, guildID, vs.ChannelID, userID)
	_ = s.Voice.IncrementSoundPlayCount(r.Context(), soundID)

	// Publish SOUNDBOARD_PLAY event so all clients in the channel can play the audio.
	s.EventBus.PublishJSON(r.Context(), events.SubjectVoiceStateUpdate, "SOUNDBOARD_PLAY", map[string]interface{}{
		"guild_id":   guildID,
		"channel_id": vs.ChannelID,
		"sound_id":   soundID,
		"sound_name": sound.Name,
		"file_url":   sound.FileURL,
		"volume":     sound.Volume,
		"user_id":    userID,
	})

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"played":   true,
		"sound_id": soundID,
	})
}

// handleGetSoundboardConfig returns the soundboard configuration for a guild.
// GET /api/v1/guilds/{guildID}/soundboard/config
func (s *Server) handleGetSoundboardConfig(w http.ResponseWriter, r *http.Request) {
	if s.Voice == nil {
		WriteError(w, http.StatusServiceUnavailable, "voice_disabled", "Voice is not enabled on this instance")
		return
	}

	guildID := chi.URLParam(r, "guildID")
	userID := auth.UserIDFromContext(r.Context())

	// Verify membership.
	var exists bool
	s.DB.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM guild_members WHERE guild_id = $1 AND user_id = $2)`,
		guildID, userID).Scan(&exists)
	if !exists {
		WriteError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	cfg, err := s.Voice.GetSoundboardConfig(r.Context(), guildID)
	if err != nil {
		s.Logger.Error("failed to get soundboard config", "error", err.Error())
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get config")
		return
	}

	WriteJSON(w, http.StatusOK, cfg)
}

// handleUpdateSoundboardConfig updates the soundboard configuration for a guild.
// PATCH /api/v1/guilds/{guildID}/soundboard/config
func (s *Server) handleUpdateSoundboardConfig(w http.ResponseWriter, r *http.Request) {
	if s.Voice == nil {
		WriteError(w, http.StatusServiceUnavailable, "voice_disabled", "Voice is not enabled on this instance")
		return
	}

	guildID := chi.URLParam(r, "guildID")
	userID := auth.UserIDFromContext(r.Context())

	// Check ManageGuild permission.
	if !checkGuildPerm(r.Context(), s.DB.Pool, guildID, userID, permissions.ManageGuild) {
		WriteError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_GUILD permission")
		return
	}

	var req struct {
		Enabled         *bool `json:"enabled"`
		MaxSounds       *int  `json:"max_sounds"`
		CooldownSeconds *int  `json:"cooldown_seconds"`
		AllowExternal   *bool `json:"allow_external"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	// Load existing config.
	cfg, err := s.Voice.GetSoundboardConfig(r.Context(), guildID)
	if err != nil {
		s.Logger.Error("failed to get soundboard config", "error", err.Error())
	}

	if req.Enabled != nil {
		cfg.Enabled = *req.Enabled
	}
	if req.MaxSounds != nil {
		if *req.MaxSounds < 1 || *req.MaxSounds > 100 {
			WriteError(w, http.StatusBadRequest, "invalid_max_sounds", "Max sounds must be between 1 and 100")
			return
		}
		cfg.MaxSounds = *req.MaxSounds
	}
	if req.CooldownSeconds != nil {
		if *req.CooldownSeconds < 0 || *req.CooldownSeconds > 300 {
			WriteError(w, http.StatusBadRequest, "invalid_cooldown", "Cooldown must be between 0 and 300 seconds")
			return
		}
		cfg.CooldownSeconds = *req.CooldownSeconds
	}
	if req.AllowExternal != nil {
		cfg.AllowExternal = *req.AllowExternal
	}

	if err := s.Voice.UpdateSoundboardConfig(r.Context(), cfg); err != nil {
		s.Logger.Error("failed to update soundboard config", "error", err.Error())
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update config")
		return
	}

	WriteJSON(w, http.StatusOK, cfg)
}

// --- Voice Broadcast Handlers ---

// handleStartBroadcast starts a one-way audio broadcast in a voice channel.
// POST /api/v1/voice/{channelID}/broadcast/start
func (s *Server) handleStartBroadcast(w http.ResponseWriter, r *http.Request) {
	if s.Voice == nil {
		WriteError(w, http.StatusServiceUnavailable, "voice_disabled", "Voice is not enabled on this instance")
		return
	}

	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	// Verify channel exists and is voice/stage.
	var channelType string
	var guildID *string
	err := s.DB.Pool.QueryRow(r.Context(),
		`SELECT channel_type, guild_id FROM channels WHERE id = $1`, channelID,
	).Scan(&channelType, &guildID)
	if err == pgx.ErrNoRows {
		WriteError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
		return
	}
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get channel")
		return
	}
	if channelType != models.ChannelTypeVoice && channelType != models.ChannelTypeStage {
		WriteError(w, http.StatusBadRequest, "not_voice_channel", "Broadcasts require a voice or stage channel")
		return
	}

	// Check permission — Speak + PrioritySpeaker to start a broadcast.
	if guildID != nil {
		if !checkGuildPerm(r.Context(), s.DB.Pool, *guildID, userID, permissions.PrioritySpeaker) {
			WriteError(w, http.StatusForbidden, "missing_permission", "You need PRIORITY_SPEAKER permission to broadcast")
			return
		}
	}

	// Verify user is in this voice channel.
	vs := s.Voice.GetVoiceState(userID)
	if vs == nil || vs.ChannelID != channelID {
		WriteError(w, http.StatusBadRequest, "not_in_channel", "You must be in this voice channel to broadcast")
		return
	}

	// Check for existing active broadcast.
	existing, _ := s.Voice.GetActiveBroadcast(r.Context(), channelID)
	if existing != nil {
		WriteError(w, http.StatusConflict, "broadcast_active", "A broadcast is already active in this channel")
		return
	}

	var req struct {
		Title string `json:"title"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Title == "" {
		req.Title = "Live Broadcast"
	}

	gID := ""
	if guildID != nil {
		gID = *guildID
	}

	broadcast := &voice.VoiceBroadcast{
		ID:            newVoiceULID(),
		GuildID:       gID,
		ChannelID:     channelID,
		BroadcasterID: userID,
		Title:         req.Title,
	}

	if err := s.Voice.CreateBroadcast(r.Context(), broadcast); err != nil {
		s.Logger.Error("failed to create broadcast", "error", err.Error())
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to start broadcast")
		return
	}

	s.Voice.SetBroadcasting(userID, true)

	// Publish broadcast start event.
	s.EventBus.PublishJSON(r.Context(), events.SubjectVoiceStateUpdate, "VOICE_BROADCAST_START", map[string]interface{}{
		"broadcast_id":   broadcast.ID,
		"guild_id":       gID,
		"channel_id":     channelID,
		"broadcaster_id": userID,
		"title":          req.Title,
	})

	WriteJSON(w, http.StatusCreated, broadcast)
}

// handleStopBroadcast stops the active broadcast in a voice channel.
// POST /api/v1/voice/{channelID}/broadcast/stop
func (s *Server) handleStopBroadcast(w http.ResponseWriter, r *http.Request) {
	if s.Voice == nil {
		WriteError(w, http.StatusServiceUnavailable, "voice_disabled", "Voice is not enabled on this instance")
		return
	}

	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	// Get active broadcast.
	broadcast, err := s.Voice.GetActiveBroadcast(r.Context(), channelID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "no_broadcast", "No active broadcast in this channel")
		return
	}

	// Only the broadcaster or someone with MoveMembers permission can stop it.
	if broadcast.BroadcasterID != userID {
		var guildID *string
		s.DB.Pool.QueryRow(r.Context(),
			`SELECT guild_id FROM channels WHERE id = $1`, channelID).Scan(&guildID)
		if guildID == nil || !checkGuildPerm(r.Context(), s.DB.Pool, *guildID, userID, permissions.MoveMembers) {
			WriteError(w, http.StatusForbidden, "missing_permission", "Only the broadcaster or a moderator can stop the broadcast")
			return
		}
	}

	if err := s.Voice.EndBroadcast(r.Context(), broadcast.ID); err != nil {
		s.Logger.Error("failed to end broadcast", "error", err.Error())
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to stop broadcast")
		return
	}

	s.Voice.SetBroadcasting(broadcast.BroadcasterID, false)

	// Publish broadcast end event.
	s.EventBus.PublishJSON(r.Context(), events.SubjectVoiceStateUpdate, "VOICE_BROADCAST_END", map[string]interface{}{
		"broadcast_id":   broadcast.ID,
		"guild_id":       broadcast.GuildID,
		"channel_id":     channelID,
		"broadcaster_id": broadcast.BroadcasterID,
	})

	WriteNoContent(w)
}

// handleGetBroadcast returns the active broadcast in a voice channel.
// GET /api/v1/voice/{channelID}/broadcast
func (s *Server) handleGetBroadcast(w http.ResponseWriter, r *http.Request) {
	if s.Voice == nil {
		WriteError(w, http.StatusServiceUnavailable, "voice_disabled", "Voice is not enabled on this instance")
		return
	}

	channelID := chi.URLParam(r, "channelID")

	broadcast, err := s.Voice.GetActiveBroadcast(r.Context(), channelID)
	if err != nil {
		WriteJSON(w, http.StatusOK, nil)
		return
	}

	WriteJSON(w, http.StatusOK, broadcast)
}

// --- Screen Share Handlers ---

// handleStartScreenShare starts a screen sharing session in a voice channel.
// POST /api/v1/voice/{channelID}/screen-share/start
func (s *Server) handleStartScreenShare(w http.ResponseWriter, r *http.Request) {
	if s.Voice == nil {
		WriteError(w, http.StatusServiceUnavailable, "voice_disabled", "Voice is not enabled on this instance")
		return
	}

	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	// Verify channel exists and is voice/stage.
	var channelType string
	var guildID *string
	err := s.DB.Pool.QueryRow(r.Context(),
		`SELECT channel_type, guild_id FROM channels WHERE id = $1`, channelID,
	).Scan(&channelType, &guildID)
	if err == pgx.ErrNoRows {
		WriteError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
		return
	}
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get channel")
		return
	}
	if channelType != models.ChannelTypeVoice && channelType != models.ChannelTypeStage {
		WriteError(w, http.StatusBadRequest, "not_voice_channel", "Screen sharing requires a voice or stage channel")
		return
	}

	// Check Stream permission.
	if guildID != nil {
		if !checkGuildPerm(r.Context(), s.DB.Pool, *guildID, userID, permissions.Stream) {
			WriteError(w, http.StatusForbidden, "missing_permission", "You need STREAM permission to share your screen")
			return
		}
	}

	// Verify user is in this voice channel.
	vs := s.Voice.GetVoiceState(userID)
	if vs == nil || vs.ChannelID != channelID {
		WriteError(w, http.StatusBadRequest, "not_in_channel", "You must be in this voice channel to screen share")
		return
	}

	// Check if user already has an active screen share.
	existing, _ := s.Voice.GetActiveScreenShare(r.Context(), channelID, userID)
	if existing != nil {
		WriteError(w, http.StatusConflict, "already_sharing", "You already have an active screen share")
		return
	}

	var req struct {
		ShareType    string `json:"share_type"`    // "screen" or "window"
		Resolution   string `json:"resolution"`     // "720p", "1080p", "4k"
		Framerate    int    `json:"framerate"`      // 15, 30, 60
		AudioEnabled bool   `json:"audio_enabled"`
		MaxViewers   int    `json:"max_viewers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	// Validate and set defaults.
	if req.ShareType != "screen" && req.ShareType != "window" {
		req.ShareType = "screen"
	}
	validResolutions := map[string]bool{"720p": true, "1080p": true, "4k": true}
	if !validResolutions[req.Resolution] {
		req.Resolution = "1080p"
	}
	validFramerates := map[int]bool{15: true, 30: true, 60: true}
	if !validFramerates[req.Framerate] {
		req.Framerate = 30
	}
	if req.MaxViewers <= 0 || req.MaxViewers > 50 {
		req.MaxViewers = 50
	}

	session := &voice.ScreenShareSession{
		ID:           newVoiceULID(),
		ChannelID:    channelID,
		UserID:       userID,
		ShareType:    req.ShareType,
		Resolution:   req.Resolution,
		Framerate:    req.Framerate,
		AudioEnabled: req.AudioEnabled,
		MaxViewers:   req.MaxViewers,
	}

	if err := s.Voice.CreateScreenShareSession(r.Context(), session); err != nil {
		s.Logger.Error("failed to create screen share session", "error", err.Error())
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to start screen share")
		return
	}

	s.Voice.SetScreenSharing(userID, true)

	gID := ""
	if guildID != nil {
		gID = *guildID
	}

	// Publish screen share start event.
	s.EventBus.PublishJSON(r.Context(), events.SubjectVoiceStateUpdate, "SCREEN_SHARE_START", map[string]interface{}{
		"session_id":    session.ID,
		"guild_id":      gID,
		"channel_id":    channelID,
		"user_id":       userID,
		"share_type":    session.ShareType,
		"resolution":    session.Resolution,
		"framerate":     session.Framerate,
		"audio_enabled": session.AudioEnabled,
		"max_viewers":   session.MaxViewers,
	})

	// Generate a LiveKit token with screen share publish permission.
	canPublish := true
	token, err := s.Voice.GenerateToken(userID, channelID, canPublish, true, canPublish, "")
	if err != nil {
		s.Logger.Error("failed to generate screen share token", "error", err.Error())
	}

	WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"session": session,
		"token":   token,
		"url":     s.liveKitPublicURL(),
	})
}

// handleStopScreenShare stops the user's screen share in a voice channel.
// POST /api/v1/voice/{channelID}/screen-share/stop
func (s *Server) handleStopScreenShare(w http.ResponseWriter, r *http.Request) {
	if s.Voice == nil {
		WriteError(w, http.StatusServiceUnavailable, "voice_disabled", "Voice is not enabled on this instance")
		return
	}

	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	// Find active session.
	session, err := s.Voice.GetActiveScreenShare(r.Context(), channelID, userID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "no_screen_share", "No active screen share")
		return
	}

	if err := s.Voice.EndScreenShareSession(r.Context(), session.ID); err != nil {
		s.Logger.Error("failed to end screen share", "error", err.Error())
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to stop screen share")
		return
	}

	s.Voice.SetScreenSharing(userID, false)

	var guildID *string
	s.DB.Pool.QueryRow(r.Context(),
		`SELECT guild_id FROM channels WHERE id = $1`, channelID).Scan(&guildID)
	gID := ""
	if guildID != nil {
		gID = *guildID
	}

	// Publish screen share end event.
	s.EventBus.PublishJSON(r.Context(), events.SubjectVoiceStateUpdate, "SCREEN_SHARE_END", map[string]interface{}{
		"session_id": session.ID,
		"guild_id":   gID,
		"channel_id": channelID,
		"user_id":    userID,
	})

	WriteNoContent(w)
}

// handleUpdateScreenShare updates screen share settings (resolution, framerate, etc.).
// PATCH /api/v1/voice/{channelID}/screen-share
func (s *Server) handleUpdateScreenShare(w http.ResponseWriter, r *http.Request) {
	if s.Voice == nil {
		WriteError(w, http.StatusServiceUnavailable, "voice_disabled", "Voice is not enabled on this instance")
		return
	}

	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	session, err := s.Voice.GetActiveScreenShare(r.Context(), channelID, userID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "no_screen_share", "No active screen share")
		return
	}

	var req struct {
		Resolution   *string `json:"resolution"`
		Framerate    *int    `json:"framerate"`
		AudioEnabled *bool   `json:"audio_enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	// Build SET clause dynamically.
	updates := make(map[string]interface{})
	if req.Resolution != nil {
		validRes := map[string]bool{"720p": true, "1080p": true, "4k": true}
		if !validRes[*req.Resolution] {
			WriteError(w, http.StatusBadRequest, "invalid_resolution", "Resolution must be 720p, 1080p, or 4k")
			return
		}
		updates["resolution"] = *req.Resolution
		session.Resolution = *req.Resolution
	}
	if req.Framerate != nil {
		validFps := map[int]bool{15: true, 30: true, 60: true}
		if !validFps[*req.Framerate] {
			WriteError(w, http.StatusBadRequest, "invalid_framerate", "Framerate must be 15, 30, or 60")
			return
		}
		updates["framerate"] = *req.Framerate
		session.Framerate = *req.Framerate
	}
	if req.AudioEnabled != nil {
		updates["audio_enabled"] = *req.AudioEnabled
		session.AudioEnabled = *req.AudioEnabled
	}

	// Apply updates one at a time (simple approach, avoids dynamic query building).
	for col, val := range updates {
		_, err := s.DB.Pool.Exec(r.Context(),
			`UPDATE screen_share_sessions SET `+col+` = $1 WHERE id = $2`, val, session.ID)
		if err != nil {
			s.Logger.Error("failed to update screen share", "error", err.Error(), "column", col)
		}
	}

	var guildID *string
	s.DB.Pool.QueryRow(r.Context(),
		`SELECT guild_id FROM channels WHERE id = $1`, channelID).Scan(&guildID)
	gID := ""
	if guildID != nil {
		gID = *guildID
	}

	// Publish update event.
	s.EventBus.PublishJSON(r.Context(), events.SubjectVoiceStateUpdate, "SCREEN_SHARE_UPDATE", map[string]interface{}{
		"session_id":    session.ID,
		"guild_id":      gID,
		"channel_id":    channelID,
		"user_id":       userID,
		"resolution":    session.Resolution,
		"framerate":     session.Framerate,
		"audio_enabled": session.AudioEnabled,
	})

	WriteJSON(w, http.StatusOK, session)
}

// handleGetScreenShares returns all active screen shares in a voice channel.
// GET /api/v1/voice/{channelID}/screen-shares
func (s *Server) handleGetScreenShares(w http.ResponseWriter, r *http.Request) {
	if s.Voice == nil {
		WriteError(w, http.StatusServiceUnavailable, "voice_disabled", "Voice is not enabled on this instance")
		return
	}

	channelID := chi.URLParam(r, "channelID")

	sessions, err := s.Voice.GetChannelScreenShares(r.Context(), channelID)
	if err != nil {
		s.Logger.Error("failed to get screen shares", "error", err.Error())
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get screen shares")
		return
	}
	if sessions == nil {
		sessions = []voice.ScreenShareSession{}
	}

	WriteJSON(w, http.StatusOK, sessions)
}

// checkGuildPerm checks if a user has a specific permission in a guild.
// Used by voice handlers which are on *Server, not on a domain-specific Handler.
func checkGuildPerm(ctx context.Context, pool *pgxpool.Pool, guildID, userID string, perm uint64) bool {
	// Owner has all permissions.
	var ownerID string
	if err := pool.QueryRow(ctx, `SELECT owner_id FROM guilds WHERE id = $1`, guildID).Scan(&ownerID); err != nil {
		return false
	}
	if userID == ownerID {
		return true
	}

	// Admin flag.
	var userFlags int
	pool.QueryRow(ctx, `SELECT flags FROM users WHERE id = $1`, userID).Scan(&userFlags)
	if userFlags&models.UserFlagAdmin != 0 {
		return true
	}

	// Compute from default + role permissions.
	var defaultPerms int64
	pool.QueryRow(ctx, `SELECT default_permissions FROM guilds WHERE id = $1`, guildID).Scan(&defaultPerms)
	computed := uint64(defaultPerms)

	rows, _ := pool.Query(ctx,
		`SELECT r.permissions_allow, r.permissions_deny
		 FROM roles r
		 JOIN member_roles mr ON r.id = mr.role_id
		 WHERE mr.guild_id = $1 AND mr.user_id = $2
		 ORDER BY r.position DESC`,
		guildID, userID,
	)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var allow, deny int64
			rows.Scan(&allow, &deny)
			computed |= uint64(allow)
			computed &^= uint64(deny)
		}
	}

	if computed&permissions.Administrator != 0 {
		return true
	}
	return computed&perm != 0
}
