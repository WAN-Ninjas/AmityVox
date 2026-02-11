package api

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/models"
	"github.com/amityvox/amityvox/internal/permissions"
)

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

	// Generate LiveKit token.
	token, err := s.Voice.GenerateToken(userID, channelID, canSpeak, true, canSpeak)
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

	// Publish VOICE_STATE_UPDATE event.
	s.EventBus.PublishJSON(r.Context(), events.SubjectVoiceStateUpdate, "VOICE_STATE_UPDATE", map[string]interface{}{
		"user_id":    userID,
		"guild_id":   gID,
		"channel_id": channelID,
		"self_mute":  false,
		"self_deaf":  false,
	})

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"token":      token,
		"url":        s.Config.LiveKit.URL,
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
