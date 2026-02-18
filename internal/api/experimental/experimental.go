// Package experimental implements REST API handlers for experimental features
// including location sharing, live location, message effects, super reactions,
// AI-powered message summarization, voice transcription, collaborative whiteboards,
// video recording, code snippets, and kanban boards.
// Mounted under /api/v1/experimental and /api/v1/channels/{channelID}/experimental.
package experimental

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"

	"github.com/amityvox/amityvox/internal/api/apiutil"
	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/events"
)

// Handler implements experimental feature REST API endpoints.
type Handler struct {
	Pool     *pgxpool.Pool
	EventBus *events.Bus
	Logger   *slog.Logger
}

// newID generates a new ULID string.
func newID() string {
	return ulid.Make().String()
}

// msgEntry is a lightweight message representation used for summarization.
type msgEntry struct {
	ID       string
	Content  string
	AuthorID string
}

// =============================================================================
// Location Sharing
// =============================================================================

type shareLocationRequest struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Accuracy  *float64 `json:"accuracy,omitempty"`
	Altitude  *float64 `json:"altitude,omitempty"`
	Label     *string  `json:"label,omitempty"`
	Live      bool     `json:"live"`
	Duration  int      `json:"duration"` // seconds, for live sharing
}

type updateLiveLocationRequest struct {
	Latitude  float64  `json:"latitude"`
	Longitude float64  `json:"longitude"`
	Accuracy  *float64 `json:"accuracy,omitempty"`
	Altitude  *float64 `json:"altitude,omitempty"`
}

// HandleShareLocation creates a one-time or live location share in a channel.
// POST /api/v1/channels/{channelID}/experimental/location
func (h *Handler) HandleShareLocation(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	var req shareLocationRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	if req.Latitude < -90 || req.Latitude > 90 {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_latitude", "Latitude must be between -90 and 90")
		return
	}
	if req.Longitude < -180 || req.Longitude > 180 {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_longitude", "Longitude must be between -180 and 180")
		return
	}

	// Verify user has access to channel.
	var exists bool
	err := h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(
			SELECT 1 FROM channels c
			LEFT JOIN guild_members gm ON gm.guild_id = c.guild_id AND gm.user_id = $2
			LEFT JOIN channel_recipients cr ON cr.channel_id = c.id AND cr.user_id = $2
			WHERE c.id = $1 AND (gm.user_id IS NOT NULL OR cr.user_id IS NOT NULL OR c.guild_id IS NULL)
		)`, channelID, userID).Scan(&exists)
	if err != nil || !exists {
		apiutil.WriteError(w, http.StatusForbidden, "no_access", "You do not have access to this channel")
		return
	}

	id := newID()

	var expiresAt *time.Time
	if req.Live && req.Duration > 0 {
		if req.Duration > 28800 { // max 8 hours
			req.Duration = 28800
		}
		t := time.Now().Add(time.Duration(req.Duration) * time.Second)
		expiresAt = &t
	}

	_, err = h.Pool.Exec(r.Context(),
		`INSERT INTO location_shares (id, user_id, channel_id, latitude, longitude, accuracy, altitude, label, live, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		id, userID, channelID, req.Latitude, req.Longitude, req.Accuracy, req.Altitude, req.Label, req.Live, expiresAt)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to share location", err)
		return
	}

	result := map[string]interface{}{
		"id":         id,
		"user_id":    userID,
		"channel_id": channelID,
		"latitude":   req.Latitude,
		"longitude":  req.Longitude,
		"accuracy":   req.Accuracy,
		"altitude":   req.Altitude,
		"label":      req.Label,
		"live":       req.Live,
		"expires_at": expiresAt,
		"created_at": time.Now().UTC(),
	}

	// Publish event for real-time updates.
	if h.EventBus != nil {
		h.EventBus.PublishChannelEvent(r.Context(), "amityvox.channel.location_share", "LOCATION_SHARE_CREATE", channelID, result)
	}

	apiutil.WriteJSON(w, http.StatusCreated, result)
}

// HandleUpdateLiveLocation updates coordinates for an active live location share.
// PATCH /api/v1/channels/{channelID}/experimental/location/{locationID}
func (h *Handler) HandleUpdateLiveLocation(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	locationID := chi.URLParam(r, "locationID")

	var req updateLiveLocationRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	if req.Latitude < -90 || req.Latitude > 90 || req.Longitude < -180 || req.Longitude > 180 {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_coordinates", "Invalid coordinates")
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`UPDATE location_shares
		 SET latitude = $1, longitude = $2, accuracy = COALESCE($3, accuracy),
		     altitude = COALESCE($4, altitude), updated_at = NOW()
		 WHERE id = $5 AND user_id = $6 AND channel_id = $7 AND live = true
		   AND (expires_at IS NULL OR expires_at > NOW())`,
		req.Latitude, req.Longitude, req.Accuracy, req.Altitude, locationID, userID, channelID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to update location", err)
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Active live location share not found")
		return
	}

	update := map[string]interface{}{
		"id":         locationID,
		"user_id":    userID,
		"channel_id": channelID,
		"latitude":   req.Latitude,
		"longitude":  req.Longitude,
		"accuracy":   req.Accuracy,
		"altitude":   req.Altitude,
		"updated_at": time.Now().UTC(),
	}

	if h.EventBus != nil {
		h.EventBus.PublishChannelEvent(r.Context(), "amityvox.channel.location_share", "LOCATION_SHARE_UPDATE", channelID, update)
	}

	apiutil.WriteJSON(w, http.StatusOK, update)
}

// HandleStopLiveLocation ends an active live location share.
// DELETE /api/v1/channels/{channelID}/experimental/location/{locationID}
func (h *Handler) HandleStopLiveLocation(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	locationID := chi.URLParam(r, "locationID")

	tag, err := h.Pool.Exec(r.Context(),
		`UPDATE location_shares SET live = false, expires_at = NOW(), updated_at = NOW()
		 WHERE id = $1 AND user_id = $2 AND channel_id = $3 AND live = true`,
		locationID, userID, channelID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to stop live location", err)
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Active live location share not found")
		return
	}

	if h.EventBus != nil {
		h.EventBus.PublishChannelEvent(r.Context(), "amityvox.channel.location_share", "LOCATION_SHARE_STOP", channelID, map[string]string{
			"id":         locationID,
			"user_id":    userID,
			"channel_id": channelID,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleGetLocationShares returns active location shares in a channel.
// GET /api/v1/channels/{channelID}/experimental/locations
func (h *Handler) HandleGetLocationShares(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")

	rows, err := h.Pool.Query(r.Context(),
		`SELECT ls.id, ls.user_id, ls.channel_id, ls.latitude, ls.longitude,
		        ls.accuracy, ls.altitude, ls.label, ls.live, ls.expires_at,
		        ls.created_at, ls.updated_at, u.username, u.display_name, u.avatar_id
		 FROM location_shares ls
		 JOIN users u ON u.id = ls.user_id
		 WHERE ls.channel_id = $1
		   AND (ls.live = false OR ls.expires_at IS NULL OR ls.expires_at > NOW())
		 ORDER BY ls.created_at DESC
		 LIMIT 50`, channelID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get location shares", err)
		return
	}
	defer rows.Close()

	type locationShare struct {
		ID          string     `json:"id"`
		UserID      string     `json:"user_id"`
		ChannelID   string     `json:"channel_id"`
		Latitude    float64    `json:"latitude"`
		Longitude   float64    `json:"longitude"`
		Accuracy    *float64   `json:"accuracy,omitempty"`
		Altitude    *float64   `json:"altitude,omitempty"`
		Label       *string    `json:"label,omitempty"`
		Live        bool       `json:"live"`
		ExpiresAt   *time.Time `json:"expires_at,omitempty"`
		CreatedAt   time.Time  `json:"created_at"`
		UpdatedAt   time.Time  `json:"updated_at"`
		Username    string     `json:"username"`
		DisplayName *string    `json:"display_name,omitempty"`
		AvatarID    *string    `json:"avatar_id,omitempty"`
	}

	locations := make([]locationShare, 0)
	for rows.Next() {
		var l locationShare
		if err := rows.Scan(&l.ID, &l.UserID, &l.ChannelID, &l.Latitude, &l.Longitude,
			&l.Accuracy, &l.Altitude, &l.Label, &l.Live, &l.ExpiresAt,
			&l.CreatedAt, &l.UpdatedAt, &l.Username, &l.DisplayName, &l.AvatarID); err != nil {
			continue
		}
		locations = append(locations, l)
	}

	apiutil.WriteJSON(w, http.StatusOK, locations)
}

// =============================================================================
// Message Effects (confetti, fireworks, etc.)
// =============================================================================

type createEffectRequest struct {
	EffectType string                 `json:"effect_type"`
	Config     map[string]interface{} `json:"config"`
}

// HandleCreateMessageEffect triggers a visual effect on a message.
// POST /api/v1/channels/{channelID}/messages/{messageID}/effects
func (h *Handler) HandleCreateMessageEffect(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")

	var req createEffectRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	validEffects := map[string]bool{
		"confetti": true, "fireworks": true, "hearts": true,
		"snow": true, "spotlight": true, "shake": true,
		"party_popper": true, "sparkles": true,
	}
	if !validEffects[req.EffectType] {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_effect_type",
			fmt.Sprintf("Effect type must be one of: %s", strings.Join(effectTypeKeys(validEffects), ", ")))
		return
	}

	// Verify message exists in channel.
	var msgExists bool
	err := h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM messages WHERE id = $1 AND channel_id = $2)`,
		messageID, channelID).Scan(&msgExists)
	if err != nil || !msgExists {
		apiutil.WriteError(w, http.StatusNotFound, "message_not_found", "Message not found in channel")
		return
	}

	configJSON, _ := json.Marshal(req.Config)
	id := newID()

	_, err = h.Pool.Exec(r.Context(),
		`INSERT INTO message_effects (id, message_id, channel_id, user_id, effect_type, config)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		id, messageID, channelID, userID, req.EffectType, configJSON)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to create effect", err)
		return
	}

	result := map[string]interface{}{
		"id":          id,
		"message_id":  messageID,
		"channel_id":  channelID,
		"user_id":     userID,
		"effect_type": req.EffectType,
		"config":      req.Config,
		"created_at":  time.Now().UTC(),
	}

	if h.EventBus != nil {
		h.EventBus.PublishChannelEvent(r.Context(), "amityvox.message.effect", "MESSAGE_EFFECT_CREATE", channelID, result)
	}

	apiutil.WriteJSON(w, http.StatusCreated, result)
}

func effectTypeKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// =============================================================================
// Super Reactions (animated reactions with particles)
// =============================================================================

type superReactionRequest struct {
	Emoji     string `json:"emoji"`
	Intensity int    `json:"intensity"` // 1-5
}

// HandleAddSuperReaction adds an animated super reaction to a message.
// POST /api/v1/channels/{channelID}/messages/{messageID}/super-reactions
func (h *Handler) HandleAddSuperReaction(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")

	var req superReactionRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	if !apiutil.RequireNonEmpty(w, "emoji", req.Emoji) {
		return
	}
	if req.Intensity < 1 || req.Intensity > 5 {
		req.Intensity = 1
	}

	// Verify message exists.
	var msgExists bool
	err := h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM messages WHERE id = $1 AND channel_id = $2)`,
		messageID, channelID).Scan(&msgExists)
	if err != nil || !msgExists {
		apiutil.WriteError(w, http.StatusNotFound, "message_not_found", "Message not found")
		return
	}

	id := newID()
	_, err = h.Pool.Exec(r.Context(),
		`INSERT INTO super_reactions (id, message_id, user_id, emoji, intensity)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (message_id, user_id, emoji) DO UPDATE SET intensity = $5`,
		id, messageID, userID, req.Emoji, req.Intensity)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to add super reaction", err)
		return
	}

	result := map[string]interface{}{
		"id":         id,
		"message_id": messageID,
		"user_id":    userID,
		"emoji":      req.Emoji,
		"intensity":  req.Intensity,
	}

	if h.EventBus != nil {
		h.EventBus.PublishChannelEvent(r.Context(), "amityvox.message.reaction_add", "SUPER_REACTION_ADD", channelID, result)
	}

	apiutil.WriteJSON(w, http.StatusCreated, result)
}

// HandleGetSuperReactions returns super reactions on a message.
// GET /api/v1/channels/{channelID}/messages/{messageID}/super-reactions
func (h *Handler) HandleGetSuperReactions(w http.ResponseWriter, r *http.Request) {
	messageID := chi.URLParam(r, "messageID")

	rows, err := h.Pool.Query(r.Context(),
		`SELECT sr.id, sr.message_id, sr.user_id, sr.emoji, sr.intensity, sr.created_at,
		        u.username, u.display_name, u.avatar_id
		 FROM super_reactions sr
		 JOIN users u ON u.id = sr.user_id
		 WHERE sr.message_id = $1
		 ORDER BY sr.created_at ASC`, messageID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get super reactions", err)
		return
	}
	defer rows.Close()

	type superReaction struct {
		ID          string    `json:"id"`
		MessageID   string    `json:"message_id"`
		UserID      string    `json:"user_id"`
		Emoji       string    `json:"emoji"`
		Intensity   int       `json:"intensity"`
		CreatedAt   time.Time `json:"created_at"`
		Username    string    `json:"username"`
		DisplayName *string   `json:"display_name,omitempty"`
		AvatarID    *string   `json:"avatar_id,omitempty"`
	}

	reactions := make([]superReaction, 0)
	for rows.Next() {
		var sr superReaction
		if err := rows.Scan(&sr.ID, &sr.MessageID, &sr.UserID, &sr.Emoji, &sr.Intensity,
			&sr.CreatedAt, &sr.Username, &sr.DisplayName, &sr.AvatarID); err != nil {
			continue
		}
		reactions = append(reactions, sr)
	}

	apiutil.WriteJSON(w, http.StatusOK, reactions)
}

// =============================================================================
// AI-Powered Message Summarization
// =============================================================================

type summarizeRequest struct {
	MessageCount int    `json:"message_count"` // how many messages to summarize (max 500)
	FromID       string `json:"from_id"`       // optional: start from this message
}

// HandleSummarizeMessages summarizes recent messages in a channel using local NLP.
// POST /api/v1/channels/{channelID}/experimental/summarize
func (h *Handler) HandleSummarizeMessages(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	var req summarizeRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	if req.MessageCount <= 0 || req.MessageCount > 500 {
		req.MessageCount = 100
	}

	// Fetch messages to summarize.
	var rows pgx.Rows
	var err error
	if req.FromID != "" {
		rows, err = h.Pool.Query(r.Context(),
			`SELECT id, content, author_id FROM messages
			 WHERE channel_id = $1 AND id >= $2 AND content IS NOT NULL AND content != ''
			 ORDER BY id ASC LIMIT $3`,
			channelID, req.FromID, req.MessageCount)
	} else {
		rows, err = h.Pool.Query(r.Context(),
			`SELECT id, content, author_id FROM messages
			 WHERE channel_id = $1 AND content IS NOT NULL AND content != ''
			 ORDER BY id DESC LIMIT $2`,
			channelID, req.MessageCount)
	}
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to fetch messages", err)
		return
	}
	defer rows.Close()

	messages := make([]msgEntry, 0)
	for rows.Next() {
		var m msgEntry
		if err := rows.Scan(&m.ID, &m.Content, &m.AuthorID); err != nil {
			continue
		}
		messages = append(messages, m)
	}

	if len(messages) == 0 {
		apiutil.WriteError(w, http.StatusBadRequest, "no_messages", "No messages found to summarize")
		return
	}

	// Build extractive summary: identify key sentences based on frequency and position.
	summary := buildExtractiveSummary(messages)

	// Determine message range.
	fromMsgID := messages[0].ID
	toMsgID := messages[len(messages)-1].ID

	id := newID()
	_, err = h.Pool.Exec(r.Context(),
		`INSERT INTO message_summaries (id, channel_id, requested_by, summary, message_count, from_message_id, to_message_id, model)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, 'extractive')`,
		id, channelID, userID, summary, len(messages), fromMsgID, toMsgID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to store summary", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"id":               id,
		"channel_id":       channelID,
		"summary":          summary,
		"message_count":    len(messages),
		"from_message_id":  fromMsgID,
		"to_message_id":    toMsgID,
		"model":            "extractive",
		"created_at":       time.Now().UTC(),
	})
}

// buildExtractiveSummary creates a basic extractive summary by selecting
// representative sentences from messages based on length and uniqueness.
func buildExtractiveSummary(messages []msgEntry) string {
	// Collect unique authors and key messages.
	authors := make(map[string]int)
	var selectedMessages []string
	totalChars := 0

	for _, m := range messages {
		authors[m.AuthorID]++
	}

	// Select messages that are substantial (>20 chars) and not duplicative.
	seen := make(map[string]bool)
	for _, m := range messages {
		trimmed := strings.TrimSpace(m.Content)
		if utf8.RuneCountInString(trimmed) < 20 {
			continue
		}
		// Use first 60 chars as key for dedup.
		key := trimmed
		if utf8.RuneCountInString(key) > 60 {
			key = string([]rune(key)[:60])
		}
		if seen[key] {
			continue
		}
		seen[key] = true

		if totalChars+utf8.RuneCountInString(trimmed) > 2000 {
			break
		}
		selectedMessages = append(selectedMessages, trimmed)
		totalChars += utf8.RuneCountInString(trimmed)

		if len(selectedMessages) >= 10 {
			break
		}
	}

	// Build summary header.
	header := fmt.Sprintf("Summary of %d messages from %d participants:\n\n", len(messages), len(authors))

	if len(selectedMessages) == 0 {
		return header + "The conversation consisted mostly of short messages and reactions."
	}

	var sb strings.Builder
	sb.WriteString(header)
	for i, msg := range selectedMessages {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("- ")
		sb.WriteString(msg)
	}

	return sb.String()
}

// HandleGetSummaries returns past channel summaries.
// GET /api/v1/channels/{channelID}/experimental/summaries
func (h *Handler) HandleGetSummaries(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, channel_id, requested_by, summary, message_count,
		        from_message_id, to_message_id, model, created_at
		 FROM message_summaries
		 WHERE channel_id = $1
		 ORDER BY created_at DESC
		 LIMIT 20`, channelID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get summaries", err)
		return
	}
	defer rows.Close()

	type summaryEntry struct {
		ID            string    `json:"id"`
		ChannelID     string    `json:"channel_id"`
		RequestedBy   string    `json:"requested_by"`
		Summary       string    `json:"summary"`
		MessageCount  int       `json:"message_count"`
		FromMessageID *string   `json:"from_message_id,omitempty"`
		ToMessageID   *string   `json:"to_message_id,omitempty"`
		Model         string    `json:"model"`
		CreatedAt     time.Time `json:"created_at"`
	}

	summaries := make([]summaryEntry, 0)
	for rows.Next() {
		var s summaryEntry
		if err := rows.Scan(&s.ID, &s.ChannelID, &s.RequestedBy, &s.Summary,
			&s.MessageCount, &s.FromMessageID, &s.ToMessageID, &s.Model, &s.CreatedAt); err != nil {
			continue
		}
		summaries = append(summaries, s)
	}

	apiutil.WriteJSON(w, http.StatusOK, summaries)
}

// =============================================================================
// Voice Transcription
// =============================================================================

type transcriptionSettingsRequest struct {
	Enabled  *bool   `json:"enabled"`
	Language *string `json:"language"`
}

// HandleUpdateTranscriptionSettings updates a user's voice transcription opt-in for a channel.
// PATCH /api/v1/channels/{channelID}/experimental/transcription/settings
func (h *Handler) HandleUpdateTranscriptionSettings(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	var req transcriptionSettingsRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	enabled := false
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	language := "en"
	if req.Language != nil && *req.Language != "" {
		language = *req.Language
	}

	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO voice_transcription_settings (channel_id, user_id, enabled, language, updated_at)
		 VALUES ($1, $2, $3, $4, NOW())
		 ON CONFLICT (channel_id, user_id) DO UPDATE SET enabled = $3, language = $4, updated_at = NOW()`,
		channelID, userID, enabled, language)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to update settings", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"channel_id": channelID,
		"user_id":    userID,
		"enabled":    enabled,
		"language":   language,
	})
}

// HandleGetTranscriptionSettings returns the user's transcription settings for a channel.
// GET /api/v1/channels/{channelID}/experimental/transcription/settings
func (h *Handler) HandleGetTranscriptionSettings(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	var enabled bool
	var language string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT enabled, language FROM voice_transcription_settings
		 WHERE channel_id = $1 AND user_id = $2`,
		channelID, userID).Scan(&enabled, &language)
	if err == pgx.ErrNoRows {
		apiutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"channel_id": channelID,
			"user_id":    userID,
			"enabled":    false,
			"language":   "en",
		})
		return
	}
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get settings", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"channel_id": channelID,
		"user_id":    userID,
		"enabled":    enabled,
		"language":   language,
	})
}

// HandleGetTranscriptions returns voice transcriptions for a channel.
// GET /api/v1/channels/{channelID}/experimental/transcriptions
func (h *Handler) HandleGetTranscriptions(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")

	rows, err := h.Pool.Query(r.Context(),
		`SELECT vt.id, vt.channel_id, vt.user_id, vt.content, vt.confidence,
		        vt.language, vt.duration_ms, vt.started_at, vt.ended_at, vt.created_at,
		        u.username, u.display_name, u.avatar_id
		 FROM voice_transcriptions vt
		 JOIN users u ON u.id = vt.user_id
		 WHERE vt.channel_id = $1
		 ORDER BY vt.created_at DESC
		 LIMIT 100`, channelID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get transcriptions", err)
		return
	}
	defer rows.Close()

	type transcription struct {
		ID          string    `json:"id"`
		ChannelID   string    `json:"channel_id"`
		UserID      string    `json:"user_id"`
		Content     string    `json:"content"`
		Confidence  *float64  `json:"confidence,omitempty"`
		Language    string    `json:"language"`
		DurationMs  int       `json:"duration_ms"`
		StartedAt   time.Time `json:"started_at"`
		EndedAt     time.Time `json:"ended_at"`
		CreatedAt   time.Time `json:"created_at"`
		Username    string    `json:"username"`
		DisplayName *string   `json:"display_name,omitempty"`
		AvatarID    *string   `json:"avatar_id,omitempty"`
	}

	transcriptions := make([]transcription, 0)
	for rows.Next() {
		var t transcription
		if err := rows.Scan(&t.ID, &t.ChannelID, &t.UserID, &t.Content, &t.Confidence,
			&t.Language, &t.DurationMs, &t.StartedAt, &t.EndedAt, &t.CreatedAt,
			&t.Username, &t.DisplayName, &t.AvatarID); err != nil {
			continue
		}
		transcriptions = append(transcriptions, t)
	}

	apiutil.WriteJSON(w, http.StatusOK, transcriptions)
}

// =============================================================================
// Collaborative Whiteboard
// =============================================================================

type createWhiteboardRequest struct {
	Name            string `json:"name"`
	Width           int    `json:"width"`
	Height          int    `json:"height"`
	BackgroundColor string `json:"background_color"`
}

type updateWhiteboardRequest struct {
	State           json.RawMessage `json:"state"`
	Name            *string         `json:"name"`
	BackgroundColor *string         `json:"background_color"`
	Locked          *bool           `json:"locked"`
}

// HandleCreateWhiteboard creates a new collaborative whiteboard in a channel.
// POST /api/v1/channels/{channelID}/experimental/whiteboards
func (h *Handler) HandleCreateWhiteboard(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	var req createWhiteboardRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	if req.Name == "" {
		req.Name = "Untitled Whiteboard"
	}
	if req.Width <= 0 || req.Width > 4096 {
		req.Width = 1920
	}
	if req.Height <= 0 || req.Height > 4096 {
		req.Height = 1080
	}
	if req.BackgroundColor == "" {
		req.BackgroundColor = "#ffffff"
	}

	// Get guild_id from channel.
	var guildID *string
	h.Pool.QueryRow(r.Context(), `SELECT guild_id FROM channels WHERE id = $1`, channelID).Scan(&guildID)

	id := newID()
	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO whiteboards (id, channel_id, guild_id, name, creator_id, width, height, background_color)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		id, channelID, guildID, req.Name, userID, req.Width, req.Height, req.BackgroundColor)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to create whiteboard", err)
		return
	}

	// Auto-add creator as collaborator.
	h.Pool.Exec(r.Context(),
		`INSERT INTO whiteboard_collaborators (whiteboard_id, user_id, role) VALUES ($1, $2, 'admin')`,
		id, userID)

	apiutil.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"id":               id,
		"channel_id":       channelID,
		"guild_id":         guildID,
		"name":             req.Name,
		"creator_id":       userID,
		"width":            req.Width,
		"height":           req.Height,
		"background_color": req.BackgroundColor,
		"locked":           false,
		"created_at":       time.Now().UTC(),
	})
}

// HandleGetWhiteboards returns whiteboards in a channel.
// GET /api/v1/channels/{channelID}/experimental/whiteboards
func (h *Handler) HandleGetWhiteboards(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, channel_id, guild_id, name, creator_id, width, height,
		        background_color, locked, max_collaborators, created_at, updated_at
		 FROM whiteboards
		 WHERE channel_id = $1
		 ORDER BY created_at DESC
		 LIMIT 20`, channelID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get whiteboards", err)
		return
	}
	defer rows.Close()

	type whiteboard struct {
		ID               string    `json:"id"`
		ChannelID        string    `json:"channel_id"`
		GuildID          *string   `json:"guild_id,omitempty"`
		Name             string    `json:"name"`
		CreatorID        string    `json:"creator_id"`
		Width            int       `json:"width"`
		Height           int       `json:"height"`
		BackgroundColor  string    `json:"background_color"`
		Locked           bool      `json:"locked"`
		MaxCollaborators int       `json:"max_collaborators"`
		CreatedAt        time.Time `json:"created_at"`
		UpdatedAt        time.Time `json:"updated_at"`
	}

	whiteboards := make([]whiteboard, 0)
	for rows.Next() {
		var wb whiteboard
		if err := rows.Scan(&wb.ID, &wb.ChannelID, &wb.GuildID, &wb.Name, &wb.CreatorID,
			&wb.Width, &wb.Height, &wb.BackgroundColor, &wb.Locked, &wb.MaxCollaborators,
			&wb.CreatedAt, &wb.UpdatedAt); err != nil {
			continue
		}
		whiteboards = append(whiteboards, wb)
	}

	apiutil.WriteJSON(w, http.StatusOK, whiteboards)
}

// HandleUpdateWhiteboard updates whiteboard state (for real-time collaboration).
// PATCH /api/v1/channels/{channelID}/experimental/whiteboards/{whiteboardID}
func (h *Handler) HandleUpdateWhiteboard(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	whiteboardID := chi.URLParam(r, "whiteboardID")

	var req updateWhiteboardRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	// Check if whiteboard is locked (only admin can modify locked boards).
	var locked bool
	var creatorID string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT locked, creator_id FROM whiteboards WHERE id = $1`, whiteboardID).Scan(&locked, &creatorID)
	if err == pgx.ErrNoRows {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Whiteboard not found")
		return
	}
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get whiteboard", err)
		return
	}

	if locked && creatorID != userID {
		apiutil.WriteError(w, http.StatusForbidden, "whiteboard_locked", "This whiteboard is locked")
		return
	}

	// Apply updates.
	if req.State != nil {
		h.Pool.Exec(r.Context(),
			`UPDATE whiteboards SET state = $1, updated_at = NOW() WHERE id = $2`,
			req.State, whiteboardID)
	}
	if req.Name != nil {
		h.Pool.Exec(r.Context(),
			`UPDATE whiteboards SET name = $1, updated_at = NOW() WHERE id = $2`,
			*req.Name, whiteboardID)
	}
	if req.BackgroundColor != nil {
		h.Pool.Exec(r.Context(),
			`UPDATE whiteboards SET background_color = $1, updated_at = NOW() WHERE id = $2`,
			*req.BackgroundColor, whiteboardID)
	}
	if req.Locked != nil && creatorID == userID {
		h.Pool.Exec(r.Context(),
			`UPDATE whiteboards SET locked = $1, updated_at = NOW() WHERE id = $2`,
			*req.Locked, whiteboardID)
	}

	// Update collaborator last active time.
	h.Pool.Exec(r.Context(),
		`INSERT INTO whiteboard_collaborators (whiteboard_id, user_id, role, last_active_at)
		 VALUES ($1, $2, 'editor', NOW())
		 ON CONFLICT (whiteboard_id, user_id) DO UPDATE SET last_active_at = NOW()`,
		whiteboardID, userID)

	if h.EventBus != nil {
		h.EventBus.PublishChannelEvent(r.Context(), "amityvox.channel.whiteboard_update", "WHITEBOARD_UPDATE", channelID, map[string]interface{}{
			"whiteboard_id": whiteboardID,
			"user_id":       userID,
			"state":         req.State,
		})
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// HandleGetWhiteboardState returns the full state of a whiteboard for initial load.
// GET /api/v1/channels/{channelID}/experimental/whiteboards/{whiteboardID}
func (h *Handler) HandleGetWhiteboardState(w http.ResponseWriter, r *http.Request) {
	whiteboardID := chi.URLParam(r, "whiteboardID")

	var state json.RawMessage
	var name, bgColor, creatorID string
	var width, height, maxCollab int
	var locked bool
	var guildID *string
	var channelID string
	var createdAt, updatedAt time.Time

	err := h.Pool.QueryRow(r.Context(),
		`SELECT id, channel_id, guild_id, name, creator_id, state, width, height,
		        background_color, locked, max_collaborators, created_at, updated_at
		 FROM whiteboards WHERE id = $1`, whiteboardID).Scan(
		&whiteboardID, &channelID, &guildID, &name, &creatorID, &state,
		&width, &height, &bgColor, &locked, &maxCollab, &createdAt, &updatedAt)
	if err == pgx.ErrNoRows {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Whiteboard not found")
		return
	}
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get whiteboard", err)
		return
	}

	// Get active collaborators.
	colRows, _ := h.Pool.Query(r.Context(),
		`SELECT wc.user_id, wc.role, wc.cursor_x, wc.cursor_y, wc.last_active_at,
		        u.username, u.display_name, u.avatar_id
		 FROM whiteboard_collaborators wc
		 JOIN users u ON u.id = wc.user_id
		 WHERE wc.whiteboard_id = $1
		 ORDER BY wc.last_active_at DESC`, whiteboardID)

	type collaborator struct {
		UserID      string    `json:"user_id"`
		Role        string    `json:"role"`
		CursorX     float64   `json:"cursor_x"`
		CursorY     float64   `json:"cursor_y"`
		LastActive  time.Time `json:"last_active_at"`
		Username    string    `json:"username"`
		DisplayName *string   `json:"display_name,omitempty"`
		AvatarID    *string   `json:"avatar_id,omitempty"`
	}

	collaborators := make([]collaborator, 0)
	if colRows != nil {
		defer colRows.Close()
		for colRows.Next() {
			var c collaborator
			if err := colRows.Scan(&c.UserID, &c.Role, &c.CursorX, &c.CursorY,
				&c.LastActive, &c.Username, &c.DisplayName, &c.AvatarID); err != nil {
				continue
			}
			collaborators = append(collaborators, c)
		}
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"id":                whiteboardID,
		"channel_id":        channelID,
		"guild_id":          guildID,
		"name":              name,
		"creator_id":        creatorID,
		"state":             state,
		"width":             width,
		"height":            height,
		"background_color":  bgColor,
		"locked":            locked,
		"max_collaborators": maxCollab,
		"collaborators":     collaborators,
		"created_at":        createdAt,
		"updated_at":        updatedAt,
	})
}

// =============================================================================
// Code Snippets
// =============================================================================

type createCodeSnippetRequest struct {
	Title    string `json:"title"`
	Language string `json:"language"`
	Code     string `json:"code"`
	Stdin    string `json:"stdin"`
	Runnable bool   `json:"runnable"`
}

// HandleCreateCodeSnippet creates a code snippet in a channel.
// POST /api/v1/channels/{channelID}/experimental/code-snippets
func (h *Handler) HandleCreateCodeSnippet(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	var req createCodeSnippetRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	if !apiutil.RequireNonEmpty(w, "code", req.Code) {
		return
	}
	if !apiutil.ValidateStringLength(w, "code", req.Code, 0, 100000) {
		return
	}
	if req.Language == "" {
		req.Language = "plaintext"
	}

	// Validate language against supported list.
	supportedLangs := map[string]bool{
		"plaintext": true, "javascript": true, "typescript": true, "python": true,
		"go": true, "rust": true, "java": true, "c": true, "cpp": true,
		"csharp": true, "ruby": true, "php": true, "swift": true, "kotlin": true,
		"html": true, "css": true, "sql": true, "bash": true, "shell": true,
		"json": true, "yaml": true, "xml": true, "toml": true, "markdown": true,
		"lua": true, "perl": true, "r": true, "scala": true, "haskell": true,
		"elixir": true, "erlang": true, "clojure": true, "dart": true, "zig": true,
	}
	if !supportedLangs[strings.ToLower(req.Language)] {
		req.Language = "plaintext"
	}

	id := newID()
	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO code_snippets (id, channel_id, author_id, title, language, code, stdin, runnable)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		id, channelID, userID, req.Title, req.Language, req.Code, req.Stdin, req.Runnable)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to create code snippet", err)
		return
	}

	result := map[string]interface{}{
		"id":         id,
		"channel_id": channelID,
		"author_id":  userID,
		"title":      req.Title,
		"language":   req.Language,
		"code":       req.Code,
		"stdin":      req.Stdin,
		"runnable":   req.Runnable,
		"created_at": time.Now().UTC(),
	}

	if h.EventBus != nil {
		h.EventBus.PublishChannelEvent(r.Context(), "amityvox.channel.code_snippet", "CODE_SNIPPET_CREATE", channelID, result)
	}

	apiutil.WriteJSON(w, http.StatusCreated, result)
}

// HandleGetCodeSnippet returns a single code snippet.
// GET /api/v1/channels/{channelID}/experimental/code-snippets/{snippetID}
func (h *Handler) HandleGetCodeSnippet(w http.ResponseWriter, r *http.Request) {
	snippetID := chi.URLParam(r, "snippetID")

	type snippet struct {
		ID          string     `json:"id"`
		ChannelID   string     `json:"channel_id"`
		MessageID   *string    `json:"message_id,omitempty"`
		AuthorID    string     `json:"author_id"`
		Title       *string    `json:"title,omitempty"`
		Language    string     `json:"language"`
		Code        string     `json:"code"`
		Stdin       *string    `json:"stdin,omitempty"`
		Output      *string    `json:"output,omitempty"`
		OutputError *string    `json:"output_error,omitempty"`
		ExitCode    *int       `json:"exit_code,omitempty"`
		RuntimeMs   *int       `json:"runtime_ms,omitempty"`
		Runnable    bool       `json:"runnable"`
		Public      bool       `json:"public"`
		CreatedAt   time.Time  `json:"created_at"`
		UpdatedAt   time.Time  `json:"updated_at"`
	}

	var s snippet
	err := h.Pool.QueryRow(r.Context(),
		`SELECT id, channel_id, message_id, author_id, title, language, code, stdin,
		        output, output_error, exit_code, runtime_ms, runnable, public, created_at, updated_at
		 FROM code_snippets WHERE id = $1`, snippetID).Scan(
		&s.ID, &s.ChannelID, &s.MessageID, &s.AuthorID, &s.Title, &s.Language,
		&s.Code, &s.Stdin, &s.Output, &s.OutputError, &s.ExitCode, &s.RuntimeMs,
		&s.Runnable, &s.Public, &s.CreatedAt, &s.UpdatedAt)
	if err == pgx.ErrNoRows {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Code snippet not found")
		return
	}
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get code snippet", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, s)
}

// HandleRunCodeSnippet executes a code snippet in a sandboxed environment.
// POST /api/v1/channels/{channelID}/experimental/code-snippets/{snippetID}/run
func (h *Handler) HandleRunCodeSnippet(w http.ResponseWriter, r *http.Request) {
	snippetID := chi.URLParam(r, "snippetID")

	var language, code string
	var stdin *string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT language, code, stdin FROM code_snippets WHERE id = $1 AND runnable = true`,
		snippetID).Scan(&language, &code, &stdin)
	if err == pgx.ErrNoRows {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Runnable code snippet not found")
		return
	}
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get code snippet", err)
		return
	}

	// NOTE: Server-side code execution requires a sandboxed runtime (not yet implemented).
	// For now, return a stub response indicating the feature needs configuration.
	output := fmt.Sprintf("[AmityVox] Server-side execution for %s is not yet configured.\nCode preview:\n%s",
		language, truncate(code, 200))
	exitCode := 0
	runtimeMs := 0

	// Cache the output.
	h.Pool.Exec(r.Context(),
		`UPDATE code_snippets SET output = $1, exit_code = $2, runtime_ms = $3, updated_at = NOW()
		 WHERE id = $4`,
		output, exitCode, runtimeMs, snippetID)

	apiutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"id":         snippetID,
		"output":     output,
		"exit_code":  exitCode,
		"runtime_ms": runtimeMs,
		"language":   language,
	})
}

func truncate(s string, maxLen int) string {
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxLen]) + "..."
}

// =============================================================================
// Video Recording
// =============================================================================

type createVideoRecordingRequest struct {
	Title         string `json:"title"`
	S3Key         string `json:"s3_key"`
	S3Bucket      string `json:"s3_bucket"`
	DurationMs    int    `json:"duration_ms"`
	FileSizeBytes int64  `json:"file_size_bytes"`
	Width         *int   `json:"width"`
	Height        *int   `json:"height"`
	ThumbnailKey  string `json:"thumbnail_s3_key"`
}

// HandleCreateVideoRecording registers a new video recording (client uploads to S3, then registers).
// POST /api/v1/channels/{channelID}/experimental/recordings
func (h *Handler) HandleCreateVideoRecording(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	var req createVideoRecordingRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	if req.S3Key == "" || req.S3Bucket == "" {
		apiutil.WriteError(w, http.StatusBadRequest, "missing_s3", "S3 key and bucket are required")
		return
	}

	id := newID()
	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO video_recordings (id, channel_id, user_id, title, s3_key, s3_bucket,
		 duration_ms, file_size_bytes, width, height, thumbnail_s3_key, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'ready')`,
		id, channelID, userID, req.Title, req.S3Key, req.S3Bucket,
		req.DurationMs, req.FileSizeBytes, req.Width, req.Height, req.ThumbnailKey)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to register recording", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"id":              id,
		"channel_id":      channelID,
		"user_id":         userID,
		"title":           req.Title,
		"duration_ms":     req.DurationMs,
		"file_size_bytes": req.FileSizeBytes,
		"status":          "ready",
		"created_at":      time.Now().UTC(),
	})
}

// HandleGetRecordings returns video recordings in a channel.
// GET /api/v1/channels/{channelID}/experimental/recordings
func (h *Handler) HandleGetRecordings(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")

	rows, err := h.Pool.Query(r.Context(),
		`SELECT vr.id, vr.channel_id, vr.user_id, vr.title, vr.duration_ms,
		        vr.file_size_bytes, vr.width, vr.height, vr.status, vr.created_at,
		        u.username, u.display_name, u.avatar_id
		 FROM video_recordings vr
		 JOIN users u ON u.id = vr.user_id
		 WHERE vr.channel_id = $1
		 ORDER BY vr.created_at DESC
		 LIMIT 50`, channelID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get recordings", err)
		return
	}
	defer rows.Close()

	type recording struct {
		ID            string    `json:"id"`
		ChannelID     string    `json:"channel_id"`
		UserID        string    `json:"user_id"`
		Title         *string   `json:"title,omitempty"`
		DurationMs    int       `json:"duration_ms"`
		FileSizeBytes int64     `json:"file_size_bytes"`
		Width         *int      `json:"width,omitempty"`
		Height        *int      `json:"height,omitempty"`
		Status        string    `json:"status"`
		CreatedAt     time.Time `json:"created_at"`
		Username      string    `json:"username"`
		DisplayName   *string   `json:"display_name,omitempty"`
		AvatarID      *string   `json:"avatar_id,omitempty"`
	}

	recordings := make([]recording, 0)
	for rows.Next() {
		var rec recording
		if err := rows.Scan(&rec.ID, &rec.ChannelID, &rec.UserID, &rec.Title,
			&rec.DurationMs, &rec.FileSizeBytes, &rec.Width, &rec.Height,
			&rec.Status, &rec.CreatedAt, &rec.Username, &rec.DisplayName, &rec.AvatarID); err != nil {
			continue
		}
		recordings = append(recordings, rec)
	}

	apiutil.WriteJSON(w, http.StatusOK, recordings)
}

// =============================================================================
// Kanban Board
// =============================================================================

type createKanbanBoardRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type createKanbanColumnRequest struct {
	Name     string  `json:"name"`
	Color    string  `json:"color"`
	WipLimit *int    `json:"wip_limit"`
}

type createKanbanCardRequest struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Color       string   `json:"color"`
	AssigneeIDs []string `json:"assignee_ids"`
	LabelIDs    []string `json:"label_ids"`
	DueDate     *string  `json:"due_date"` // ISO 8601
}

type moveKanbanCardRequest struct {
	ColumnID string `json:"column_id"`
	Position int    `json:"position"`
}

// HandleCreateKanbanBoard creates a kanban board in a channel.
// POST /api/v1/channels/{channelID}/experimental/kanban
func (h *Handler) HandleCreateKanbanBoard(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	var req createKanbanBoardRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		req.Name = "Project Board"
	}

	// Get guild_id from channel.
	var guildID string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT guild_id FROM channels WHERE id = $1 AND guild_id IS NOT NULL`, channelID).Scan(&guildID)
	if err != nil {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_channel", "Channel must belong to a guild")
		return
	}

	boardID := newID()
	_, err = h.Pool.Exec(r.Context(),
		`INSERT INTO kanban_boards (id, channel_id, guild_id, name, description, creator_id)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		boardID, channelID, guildID, req.Name, req.Description, userID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to create board", err)
		return
	}

	// Create default columns: To Do, In Progress, Done.
	defaultColumns := []struct{ Name, Color string }{
		{"To Do", "#6b7280"},
		{"In Progress", "#3b82f6"},
		{"Done", "#22c55e"},
	}
	for i, col := range defaultColumns {
		colID := newID()
		h.Pool.Exec(r.Context(),
			`INSERT INTO kanban_columns (id, board_id, name, color, position)
			 VALUES ($1, $2, $3, $4, $5)`,
			colID, boardID, col.Name, col.Color, i)
	}

	apiutil.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"id":          boardID,
		"channel_id":  channelID,
		"guild_id":    guildID,
		"name":        req.Name,
		"description": req.Description,
		"creator_id":  userID,
		"created_at":  time.Now().UTC(),
	})
}

// HandleGetKanbanBoard returns a full kanban board with columns and cards.
// GET /api/v1/channels/{channelID}/experimental/kanban/{boardID}
func (h *Handler) HandleGetKanbanBoard(w http.ResponseWriter, r *http.Request) {
	boardID := chi.URLParam(r, "boardID")

	var name, guildID, creatorID, channelID string
	var description *string
	var createdAt, updatedAt time.Time
	err := h.Pool.QueryRow(r.Context(),
		`SELECT id, channel_id, guild_id, name, description, creator_id, created_at, updated_at
		 FROM kanban_boards WHERE id = $1`, boardID).Scan(
		&boardID, &channelID, &guildID, &name, &description, &creatorID, &createdAt, &updatedAt)
	if err == pgx.ErrNoRows {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Kanban board not found")
		return
	}
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get board", err)
		return
	}

	// Fetch columns.
	colRows, err := h.Pool.Query(r.Context(),
		`SELECT id, name, color, position, wip_limit FROM kanban_columns
		 WHERE board_id = $1 ORDER BY position ASC`, boardID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get columns", err)
		return
	}
	defer colRows.Close()

	type card struct {
		ID          string     `json:"id"`
		ColumnID    string     `json:"column_id"`
		Title       string     `json:"title"`
		Description *string    `json:"description,omitempty"`
		Color       *string    `json:"color,omitempty"`
		Position    int        `json:"position"`
		AssigneeIDs []string   `json:"assignee_ids"`
		LabelIDs    []string   `json:"label_ids"`
		DueDate     *time.Time `json:"due_date,omitempty"`
		Completed   bool       `json:"completed"`
		CreatorID   string     `json:"creator_id"`
		CreatedAt   time.Time  `json:"created_at"`
	}

	type column struct {
		ID       string  `json:"id"`
		Name     string  `json:"name"`
		Color    string  `json:"color"`
		Position int     `json:"position"`
		WipLimit *int    `json:"wip_limit,omitempty"`
		Cards    []card  `json:"cards"`
	}

	columns := make([]column, 0)
	columnIDs := make([]string, 0)
	for colRows.Next() {
		var c column
		if err := colRows.Scan(&c.ID, &c.Name, &c.Color, &c.Position, &c.WipLimit); err != nil {
			continue
		}
		c.Cards = make([]card, 0)
		columns = append(columns, c)
		columnIDs = append(columnIDs, c.ID)
	}
	colRows.Close()

	// Fetch all cards for this board.
	if len(columnIDs) > 0 {
		cardRows, err := h.Pool.Query(r.Context(),
			`SELECT id, column_id, title, description, color, position,
			        assignee_ids, label_ids, due_date, completed, creator_id, created_at
			 FROM kanban_cards
			 WHERE board_id = $1
			 ORDER BY position ASC`, boardID)
		if err == nil {
			defer cardRows.Close()
			for cardRows.Next() {
				var c card
				if err := cardRows.Scan(&c.ID, &c.ColumnID, &c.Title, &c.Description,
					&c.Color, &c.Position, &c.AssigneeIDs, &c.LabelIDs,
					&c.DueDate, &c.Completed, &c.CreatorID, &c.CreatedAt); err != nil {
					continue
				}
				if c.AssigneeIDs == nil {
					c.AssigneeIDs = []string{}
				}
				if c.LabelIDs == nil {
					c.LabelIDs = []string{}
				}
				// Place card in correct column.
				for i := range columns {
					if columns[i].ID == c.ColumnID {
						columns[i].Cards = append(columns[i].Cards, c)
						break
					}
				}
			}
		}
	}

	// Fetch labels.
	labelRows, _ := h.Pool.Query(r.Context(),
		`SELECT id, name, color FROM kanban_labels WHERE board_id = $1 ORDER BY name`, boardID)
	type label struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	labels := make([]label, 0)
	if labelRows != nil {
		defer labelRows.Close()
		for labelRows.Next() {
			var l label
			if err := labelRows.Scan(&l.ID, &l.Name, &l.Color); err != nil {
				continue
			}
			labels = append(labels, l)
		}
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"id":          boardID,
		"channel_id":  channelID,
		"guild_id":    guildID,
		"name":        name,
		"description": description,
		"creator_id":  creatorID,
		"columns":     columns,
		"labels":      labels,
		"created_at":  createdAt,
		"updated_at":  updatedAt,
	})
}

// HandleCreateKanbanColumn adds a column to a kanban board.
// POST /api/v1/channels/{channelID}/experimental/kanban/{boardID}/columns
func (h *Handler) HandleCreateKanbanColumn(w http.ResponseWriter, r *http.Request) {
	boardID := chi.URLParam(r, "boardID")

	var req createKanbanColumnRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}
	if !apiutil.RequireNonEmpty(w, "name", req.Name) {
		return
	}
	if req.Color == "" {
		req.Color = "#6366f1"
	}

	// Get next position.
	var maxPos int
	h.Pool.QueryRow(r.Context(),
		`SELECT COALESCE(MAX(position), -1) FROM kanban_columns WHERE board_id = $1`, boardID).Scan(&maxPos)

	colID := newID()
	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO kanban_columns (id, board_id, name, color, position, wip_limit)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		colID, boardID, req.Name, req.Color, maxPos+1, req.WipLimit)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to create column", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"id":        colID,
		"board_id":  boardID,
		"name":      req.Name,
		"color":     req.Color,
		"position":  maxPos + 1,
		"wip_limit": req.WipLimit,
	})
}

// HandleCreateKanbanCard adds a card to a kanban column.
// POST /api/v1/channels/{channelID}/experimental/kanban/{boardID}/columns/{columnID}/cards
func (h *Handler) HandleCreateKanbanCard(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	boardID := chi.URLParam(r, "boardID")
	columnID := chi.URLParam(r, "columnID")

	var req createKanbanCardRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}
	if !apiutil.RequireNonEmpty(w, "title", req.Title) {
		return
	}

	// Get next position.
	var maxPos int
	h.Pool.QueryRow(r.Context(),
		`SELECT COALESCE(MAX(position), -1) FROM kanban_cards WHERE column_id = $1`, columnID).Scan(&maxPos)

	var dueDate *time.Time
	if req.DueDate != nil && *req.DueDate != "" {
		if t, err := time.Parse(time.RFC3339, *req.DueDate); err == nil {
			dueDate = &t
		}
	}
	if req.AssigneeIDs == nil {
		req.AssigneeIDs = []string{}
	}
	if req.LabelIDs == nil {
		req.LabelIDs = []string{}
	}

	cardID := newID()
	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO kanban_cards (id, column_id, board_id, title, description, color, position,
		 assignee_ids, label_ids, due_date, creator_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		cardID, columnID, boardID, req.Title, req.Description, req.Color,
		maxPos+1, req.AssigneeIDs, req.LabelIDs, dueDate, userID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to create card", err)
		return
	}

	result := map[string]interface{}{
		"id":           cardID,
		"column_id":    columnID,
		"board_id":     boardID,
		"title":        req.Title,
		"description":  req.Description,
		"color":        req.Color,
		"position":     maxPos + 1,
		"assignee_ids": req.AssigneeIDs,
		"label_ids":    req.LabelIDs,
		"due_date":     dueDate,
		"creator_id":   userID,
		"completed":    false,
		"created_at":   time.Now().UTC(),
	}

	if h.EventBus != nil {
		h.EventBus.PublishChannelEvent(r.Context(), "amityvox.channel.kanban_update", "KANBAN_CARD_CREATE", channelID, result)
	}

	apiutil.WriteJSON(w, http.StatusCreated, result)
}

// HandleMoveKanbanCard moves a card to a different column or position.
// PATCH /api/v1/channels/{channelID}/experimental/kanban/{boardID}/cards/{cardID}/move
func (h *Handler) HandleMoveKanbanCard(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")
	cardID := chi.URLParam(r, "cardID")

	var req moveKanbanCardRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}
	if !apiutil.RequireNonEmpty(w, "column_id", req.ColumnID) {
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`UPDATE kanban_cards SET column_id = $1, position = $2, updated_at = NOW()
		 WHERE id = $3`,
		req.ColumnID, req.Position, cardID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to move card", err)
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Card not found")
		return
	}

	if h.EventBus != nil {
		h.EventBus.PublishChannelEvent(r.Context(), "amityvox.channel.kanban_update", "KANBAN_CARD_MOVE", channelID, map[string]interface{}{
			"card_id":   cardID,
			"column_id": req.ColumnID,
			"position":  req.Position,
		})
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "moved"})
}

// HandleDeleteKanbanCard deletes a card from a kanban board.
// DELETE /api/v1/channels/{channelID}/experimental/kanban/{boardID}/cards/{cardID}
func (h *Handler) HandleDeleteKanbanCard(w http.ResponseWriter, r *http.Request) {
	cardID := chi.URLParam(r, "cardID")

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM kanban_cards WHERE id = $1`, cardID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to delete card", err)
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Card not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
