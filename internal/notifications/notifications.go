// Package notifications implements WebPush notifications for AmityVox. When a user
// is mentioned, receives a DM, or has other notification-worthy events, the server
// sends push notifications to all registered browser/device subscriptions via the
// Web Push protocol (RFC 8030 + RFC 8291 + RFC 8292).
package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/models"
)

// Notification levels.
const (
	LevelAll      = "all"
	LevelMentions = "mentions"
	LevelNone     = "none"
)

// PushSubscription represents a browser/device push subscription.
type PushSubscription struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Endpoint  string    `json:"endpoint"`
	KeyP256dh string    `json:"key_p256dh"`
	KeyAuth   string    `json:"key_auth"`
	UserAgent string    `json:"user_agent,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	LastUsed  time.Time `json:"last_used"`
}

// NotificationPreferences holds a user's notification settings for a guild (or global).
type NotificationPreferences struct {
	UserID            string     `json:"user_id"`
	GuildID           string     `json:"guild_id,omitempty"`
	Level             string     `json:"level"`
	SuppressHere      bool       `json:"suppress_here"`
	SuppressRoles     bool       `json:"suppress_roles"`
	MutedUntil        *time.Time `json:"muted_until,omitempty"`
}

// PushPayload is the JSON structure sent in push notifications.
type PushPayload struct {
	Type      string `json:"type"`                 // "message", "mention", "dm", "friend_request"
	Title     string `json:"title"`
	Body      string `json:"body"`
	Icon      string `json:"icon,omitempty"`
	URL       string `json:"url,omitempty"`         // Deep link path
	ChannelID string `json:"channel_id,omitempty"`
	GuildID   string `json:"guild_id,omitempty"`
	MessageID string `json:"message_id,omitempty"`
}

// Service manages push subscriptions and sends WebPush notifications.
type Service struct {
	pool       *pgxpool.Pool
	logger     *slog.Logger
	vapidPub   string
	vapidPriv  string
	vapidEmail string
	bus        *events.Bus
}

// Config holds configuration for the notification service.
type Config struct {
	Pool             *pgxpool.Pool
	Logger           *slog.Logger
	VAPIDPublicKey   string
	VAPIDPrivateKey  string
	VAPIDContactEmail string
	Bus              *events.Bus
}

// NewService creates a new notification service.
func NewService(cfg Config) *Service {
	return &Service{
		pool:       cfg.Pool,
		logger:     cfg.Logger,
		vapidPub:   cfg.VAPIDPublicKey,
		vapidPriv:  cfg.VAPIDPrivateKey,
		vapidEmail: cfg.VAPIDContactEmail,
		bus:        cfg.Bus,
	}
}

// Enabled returns true if VAPID keys are configured.
func (s *Service) Enabled() bool {
	return s.vapidPub != "" && s.vapidPriv != ""
}

// --- Push Subscription Handlers ---

// HandleSubscribe handles POST /api/v1/notifications/subscriptions.
// Registers a new push subscription for the authenticated user.
func (s *Service) HandleSubscribe(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var req struct {
		Endpoint  string `json:"endpoint"`
		KeyP256dh string `json:"key_p256dh"`
		KeyAuth   string `json:"key_auth"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Endpoint == "" || req.KeyP256dh == "" || req.KeyAuth == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "endpoint, key_p256dh, and key_auth are required")
		return
	}

	id := models.NewULID().String()
	_, err := s.pool.Exec(r.Context(),
		`INSERT INTO push_subscriptions (id, user_id, endpoint, key_p256dh, key_auth, user_agent, created_at, last_used)
		 VALUES ($1, $2, $3, $4, $5, $6, now(), now())
		 ON CONFLICT (user_id, endpoint) DO UPDATE SET
		   key_p256dh = EXCLUDED.key_p256dh,
		   key_auth = EXCLUDED.key_auth,
		   last_used = now()`,
		id, userID, req.Endpoint, req.KeyP256dh, req.KeyAuth, r.UserAgent(),
	)
	if err != nil {
		s.logger.Error("failed to store push subscription", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to register subscription")
		return
	}

	writeJSON(w, http.StatusCreated, PushSubscription{
		ID:        id,
		UserID:    userID,
		Endpoint:  req.Endpoint,
		KeyP256dh: req.KeyP256dh,
		KeyAuth:   req.KeyAuth,
		UserAgent: r.UserAgent(),
		CreatedAt: time.Now().UTC(),
		LastUsed:  time.Now().UTC(),
	})
}

// HandleListSubscriptions handles GET /api/v1/notifications/subscriptions.
func (s *Service) HandleListSubscriptions(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	rows, err := s.pool.Query(r.Context(),
		`SELECT id, user_id, endpoint, key_p256dh, key_auth, user_agent, created_at, last_used
		 FROM push_subscriptions WHERE user_id = $1 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to query subscriptions")
		return
	}
	defer rows.Close()

	subs := []PushSubscription{}
	for rows.Next() {
		var sub PushSubscription
		var ua *string
		if err := rows.Scan(&sub.ID, &sub.UserID, &sub.Endpoint, &sub.KeyP256dh, &sub.KeyAuth, &ua, &sub.CreatedAt, &sub.LastUsed); err != nil {
			continue
		}
		if ua != nil {
			sub.UserAgent = *ua
		}
		subs = append(subs, sub)
	}

	writeJSON(w, http.StatusOK, subs)
}

// HandleUnsubscribe handles DELETE /api/v1/notifications/subscriptions/{subscriptionID}.
func (s *Service) HandleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	subID := chi.URLParam(r, "subscriptionID")

	result, err := s.pool.Exec(r.Context(),
		`DELETE FROM push_subscriptions WHERE id = $1 AND user_id = $2`,
		subID, userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete subscription")
		return
	}
	if result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Subscription not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleGetVAPIDKey handles GET /api/v1/notifications/vapid-key.
// Returns the public VAPID key for client subscription.
func (s *Service) HandleGetVAPIDKey(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"vapid_public_key": s.vapidPub,
	})
}

// --- Notification Preferences Handlers ---

// HandleGetPreferences handles GET /api/v1/notifications/preferences.
func (s *Service) HandleGetPreferences(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := r.URL.Query().Get("guild_id")

	var prefs NotificationPreferences
	var mutedUntil *time.Time

	query := `SELECT user_id, guild_id, level, suppress_here, suppress_roles, muted_until
	          FROM notification_preferences WHERE user_id = $1`
	args := []interface{}{userID}
	if guildID != "" {
		query += ` AND guild_id = $2`
		args = append(args, guildID)
	} else {
		query += ` AND guild_id = '__global__'`
	}

	err := s.pool.QueryRow(r.Context(), query, args...).Scan(
		&prefs.UserID, &prefs.GuildID, &prefs.Level,
		&prefs.SuppressHere, &prefs.SuppressRoles, &mutedUntil,
	)
	if err == pgx.ErrNoRows {
		// Return defaults.
		prefs = NotificationPreferences{
			UserID:  userID,
			GuildID: guildID,
			Level:   LevelMentions,
		}
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to query preferences")
		return
	}

	prefs.MutedUntil = mutedUntil
	writeJSON(w, http.StatusOK, prefs)
}

// HandleUpdatePreferences handles PATCH /api/v1/notifications/preferences.
func (s *Service) HandleUpdatePreferences(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var req struct {
		GuildID          *string    `json:"guild_id"`
		Level            *string    `json:"level"`
		SuppressHere *bool      `json:"suppress_here"`
		SuppressRoles    *bool      `json:"suppress_roles"`
		MutedUntil       *time.Time `json:"muted_until"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	level := LevelMentions
	if req.Level != nil {
		level = *req.Level
	}
	validLevels := map[string]bool{LevelAll: true, LevelMentions: true, LevelNone: true}
	if !validLevels[level] {
		writeError(w, http.StatusBadRequest, "invalid_level", "Level must be all, mentions, or none")
		return
	}

	suppressHere := false
	if req.SuppressHere != nil {
		suppressHere = *req.SuppressHere
	}
	suppressRoles := false
	if req.SuppressRoles != nil {
		suppressRoles = *req.SuppressRoles
	}

	guildIDVal := "__global__"
	if req.GuildID != nil && *req.GuildID != "" {
		guildIDVal = *req.GuildID
	}

	_, err := s.pool.Exec(r.Context(),
		`INSERT INTO notification_preferences (user_id, guild_id, level, suppress_here, suppress_roles, muted_until)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (user_id, guild_id) DO UPDATE SET
		   level = EXCLUDED.level,
		   suppress_here = EXCLUDED.suppress_here,
		   suppress_roles = EXCLUDED.suppress_roles,
		   muted_until = EXCLUDED.muted_until`,
		userID, guildIDVal, level, suppressHere, suppressRoles, req.MutedUntil,
	)
	if err != nil {
		s.logger.Error("failed to update notification preferences", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update preferences")
		return
	}

	guildIDStr := guildIDVal
	if guildIDStr == "__global__" {
		guildIDStr = ""
	}

	writeJSON(w, http.StatusOK, NotificationPreferences{
		UserID:           userID,
		GuildID:          guildIDStr,
		Level:            level,
		SuppressHere: suppressHere,
		SuppressRoles:    suppressRoles,
		MutedUntil:       req.MutedUntil,
	})
}

// --- Channel Notification Preferences ---

// ChannelNotificationPreference holds a user's notification settings for a single channel.
type ChannelNotificationPreference struct {
	UserID     string     `json:"user_id"`
	ChannelID  string     `json:"channel_id"`
	Level      string     `json:"level"`
	MutedUntil *time.Time `json:"muted_until,omitempty"`
}

// HandleGetChannelPreferences handles GET /api/v1/notifications/preferences/channels.
// Returns all per-channel notification preferences for the authenticated user.
func (s *Service) HandleGetChannelPreferences(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	rows, err := s.pool.Query(r.Context(),
		`SELECT user_id, channel_id, level, muted_until
		 FROM channel_notification_preferences WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to query channel preferences")
		return
	}
	defer rows.Close()

	prefs := []ChannelNotificationPreference{}
	for rows.Next() {
		var p ChannelNotificationPreference
		if err := rows.Scan(&p.UserID, &p.ChannelID, &p.Level, &p.MutedUntil); err != nil {
			continue
		}
		prefs = append(prefs, p)
	}

	writeJSON(w, http.StatusOK, prefs)
}

// HandleUpdateChannelPreference handles PATCH /api/v1/notifications/preferences/channels.
// Creates or updates a per-channel notification preference.
func (s *Service) HandleUpdateChannelPreference(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var req struct {
		ChannelID  string     `json:"channel_id"`
		Level      string     `json:"level"`
		MutedUntil *time.Time `json:"muted_until"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.ChannelID == "" {
		writeError(w, http.StatusBadRequest, "missing_channel_id", "channel_id is required")
		return
	}

	// Ensure the user can access this channel.
	var allowed bool
	err := s.pool.QueryRow(r.Context(), `
		SELECT EXISTS(
			SELECT 1
			FROM channels c
			JOIN guild_members gm ON gm.guild_id = c.guild_id
			WHERE c.id = $1 AND gm.user_id = $2
		)`, req.ChannelID, userID).Scan(&allowed)
	if err != nil || !allowed {
		writeError(w, http.StatusForbidden, "not_member", "You are not a member of this channel's guild")
		return
	}

	if req.Level == "" {
		req.Level = LevelMentions
	}
	validLevels := map[string]bool{LevelAll: true, LevelMentions: true, LevelNone: true}
	if !validLevels[req.Level] {
		writeError(w, http.StatusBadRequest, "invalid_level", "Level must be all, mentions, or none")
		return
	}

	_, err = s.pool.Exec(r.Context(),
		`INSERT INTO channel_notification_preferences (user_id, channel_id, level, muted_until)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (user_id, channel_id) DO UPDATE SET
		   level = EXCLUDED.level,
		   muted_until = EXCLUDED.muted_until`,
		userID, req.ChannelID, req.Level, req.MutedUntil,
	)
	if err != nil {
		s.logger.Error("failed to update channel notification preference", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update channel preference")
		return
	}

	writeJSON(w, http.StatusOK, ChannelNotificationPreference{
		UserID:     userID,
		ChannelID:  req.ChannelID,
		Level:      req.Level,
		MutedUntil: req.MutedUntil,
	})
}

// HandleDeleteChannelPreference handles DELETE /api/v1/notifications/preferences/channels/{channelID}.
// Removes a per-channel preference so the channel inherits from guild/global settings.
func (s *Service) HandleDeleteChannelPreference(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	if channelID == "" {
		writeError(w, http.StatusBadRequest, "missing_channel_id", "Channel ID is required")
		return
	}

	// Ensure the user can access this channel.
	var allowed bool
	if err := s.pool.QueryRow(r.Context(), `
		SELECT EXISTS(
			SELECT 1
			FROM channels c
			JOIN guild_members gm ON gm.guild_id = c.guild_id
			WHERE c.id = $1 AND gm.user_id = $2
		)`, channelID, userID).Scan(&allowed); err != nil || !allowed {
		writeError(w, http.StatusForbidden, "not_member", "You are not a member of this channel's guild")
		return
	}

	result, err := s.pool.Exec(r.Context(),
		`DELETE FROM channel_notification_preferences WHERE user_id = $1 AND channel_id = $2`,
		userID, channelID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete channel preference")
		return
	}
	if result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "No channel preference found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Push Delivery ---

// SendToUser sends a push notification to all of a user's registered subscriptions.
func (s *Service) SendToUser(ctx context.Context, userID string, payload PushPayload) error {
	if !s.Enabled() {
		return nil
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling push payload: %w", err)
	}

	rows, err := s.pool.Query(ctx,
		`SELECT id, endpoint, key_p256dh, key_auth FROM push_subscriptions WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("querying push subscriptions: %w", err)
	}
	defer rows.Close()

	var staleIDs []string

	for rows.Next() {
		var id, endpoint, p256dh, authKey string
		if err := rows.Scan(&id, &endpoint, &p256dh, &authKey); err != nil {
			continue
		}

		sub := &webpush.Subscription{
			Endpoint: endpoint,
			Keys: webpush.Keys{
				P256dh: p256dh,
				Auth:   authKey,
			},
		}

		resp, err := webpush.SendNotification(payloadJSON, sub, &webpush.Options{
			VAPIDPublicKey:  s.vapidPub,
			VAPIDPrivateKey: s.vapidPriv,
			Subscriber:      s.vapidEmail,
			TTL:             86400,
		})
		if err != nil {
			s.logger.Debug("push send failed",
				slog.String("user_id", userID),
				slog.String("endpoint", endpoint[:min(50, len(endpoint))]),
				slog.String("error", err.Error()),
			)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound {
			staleIDs = append(staleIDs, id)
			continue
		}

		// Update last_used timestamp.
		s.pool.Exec(ctx,
			`UPDATE push_subscriptions SET last_used = now() WHERE id = $1`, id)
	}

	// Clean up stale subscriptions (gone/not found).
	for _, id := range staleIDs {
		s.pool.Exec(ctx,
			`DELETE FROM push_subscriptions WHERE id = $1`, id)
		s.logger.Debug("removed stale push subscription", slog.String("id", id))
	}

	return nil
}

// ShouldNotify checks if a user should receive a notification for this event based
// on their notification preferences. Resolution order: Channel > Guild > Global > Default(mentions).
func (s *Service) ShouldNotify(ctx context.Context, userID, guildID, channelID string, isMention, isDM, isHere bool) bool {
	// Check channel-level preferences first (most specific).
	if channelID != "" {
		var chLevel string
		var chMutedUntil *time.Time
		err := s.pool.QueryRow(ctx,
			`SELECT level, muted_until FROM channel_notification_preferences
			 WHERE user_id = $1 AND channel_id = $2`,
			userID, channelID,
		).Scan(&chLevel, &chMutedUntil)

		if err == nil {
			// Channel preference exists — check muted_until first.
			if chMutedUntil != nil && time.Now().Before(*chMutedUntil) {
				return false
			}
			switch chLevel {
			case LevelNone:
				return false
			case LevelAll:
				return true
			case LevelMentions:
				return isMention || isHere || isDM
			}
		}
	}

	// No channel-level pref found — fall through to guild/global.
	// For DMs with no channel-level override, always notify.
	if isDM {
		return true
	}

	// Load guild-specific preferences, falling back to global.
	var level string
	var suppressHere, suppressRoles bool
	var mutedUntil *time.Time

	err := s.pool.QueryRow(ctx,
		`SELECT level, suppress_here, suppress_roles, muted_until
		 FROM notification_preferences
		 WHERE user_id = $1 AND guild_id = $2`,
		userID, guildID,
	).Scan(&level, &suppressHere, &suppressRoles, &mutedUntil)

	if err != nil {
		// No guild preferences — check global.
		err = s.pool.QueryRow(ctx,
			`SELECT level, suppress_here, suppress_roles, muted_until
			 FROM notification_preferences
			 WHERE user_id = $1 AND guild_id = '__global__'`,
			userID,
		).Scan(&level, &suppressHere, &suppressRoles, &mutedUntil)
		if err != nil {
			level = LevelMentions // Default.
		}
	}

	// Check muted.
	if mutedUntil != nil && time.Now().Before(*mutedUntil) {
		return false
	}

	switch level {
	case LevelNone:
		return false
	case LevelAll:
		return true
	case LevelMentions:
		if isHere && suppressHere {
			return false
		}
		return isMention || isHere
	}

	return isMention
}

// CleanupStaleSubscriptions removes push subscriptions unused for longer than maxAge.
func (s *Service) CleanupStaleSubscriptions(ctx context.Context, maxAge time.Duration) error {
	cutoff := time.Now().Add(-maxAge)
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM push_subscriptions WHERE last_used < $1`, cutoff)
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 {
		s.logger.Info("cleaned stale push subscriptions",
			slog.Int64("deleted", tag.RowsAffected()))
	}
	return nil
}

// --- Persistent Notification CRUD ---

// HandleListNotifications handles GET /api/v1/notifications.
// Returns paginated notifications with optional cursor, type, and unread filters.
func (s *Service) HandleListNotifications(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	before := r.URL.Query().Get("before")
	limitStr := r.URL.Query().Get("limit")
	typeFilter := r.URL.Query().Get("type")
	unreadOnly := r.URL.Query().Get("unread_only") == "true"

	limit := 50
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	query := `SELECT id, user_id, type, category, guild_id, guild_name, guild_icon_id,
	          channel_id, channel_name, message_id, actor_id, actor_name, actor_avatar_id,
	          content, metadata, read, created_at
	          FROM notifications WHERE user_id = $1`
	args := []interface{}{userID}
	argIdx := 2

	if before != "" {
		query += fmt.Sprintf(` AND id < $%d`, argIdx)
		args = append(args, before)
		argIdx++
	}
	if typeFilter != "" {
		query += fmt.Sprintf(` AND type = $%d`, argIdx)
		args = append(args, typeFilter)
		argIdx++
	}
	if unreadOnly {
		query += ` AND NOT read`
	}

	query += ` ORDER BY id DESC LIMIT $` + strconv.Itoa(argIdx)
	args = append(args, limit)

	rows, err := s.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to query notifications")
		return
	}
	defer rows.Close()

	notifs := []models.Notification{}
	for rows.Next() {
		var n models.Notification
		if err := rows.Scan(&n.ID, &n.UserID, &n.Type, &n.Category, &n.GuildID, &n.GuildName,
			&n.GuildIconID, &n.ChannelID, &n.ChannelName, &n.MessageID, &n.ActorID, &n.ActorName,
			&n.ActorAvatarID, &n.Content, &n.Metadata, &n.Read, &n.CreatedAt); err != nil {
			continue
		}
		notifs = append(notifs, n)
	}

	writeJSON(w, http.StatusOK, notifs)
}

// HandleUpdateNotification handles PATCH /api/v1/notifications/{id}.
// Updates the read status of a single notification.
func (s *Service) HandleUpdateNotification(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	notifID := chi.URLParam(r, "id")

	var req struct {
		Read *bool `json:"read"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if req.Read == nil {
		writeError(w, http.StatusBadRequest, "missing_field", "read field is required")
		return
	}

	var n models.Notification
	err := s.pool.QueryRow(r.Context(),
		`UPDATE notifications SET read = $1 WHERE id = $2 AND user_id = $3
		 RETURNING id, user_id, type, category, guild_id, guild_name, guild_icon_id,
		 channel_id, channel_name, message_id, actor_id, actor_name, actor_avatar_id,
		 content, metadata, read, created_at`,
		*req.Read, notifID, userID,
	).Scan(&n.ID, &n.UserID, &n.Type, &n.Category, &n.GuildID, &n.GuildName,
		&n.GuildIconID, &n.ChannelID, &n.ChannelName, &n.MessageID, &n.ActorID, &n.ActorName,
		&n.ActorAvatarID, &n.Content, &n.Metadata, &n.Read, &n.CreatedAt)

	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "not_found", "Notification not found")
		return
	} else if err != nil {
		s.logger.Error("failed to update notification", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update notification")
		return
	}

	if s.bus != nil {
		_ = s.bus.PublishUserEvent(r.Context(), events.SubjectNotificationUpdate, "NOTIFICATION_UPDATE", userID, n)
	}

	writeJSON(w, http.StatusOK, n)
}

// HandleDeleteNotification handles DELETE /api/v1/notifications/{id}.
func (s *Service) HandleDeleteNotification(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	notifID := chi.URLParam(r, "id")

	result, err := s.pool.Exec(r.Context(),
		`DELETE FROM notifications WHERE id = $1 AND user_id = $2`,
		notifID, userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete notification")
		return
	}
	if result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Notification not found")
		return
	}

	if s.bus != nil {
		_ = s.bus.PublishUserEvent(r.Context(), events.SubjectNotificationDelete, "NOTIFICATION_DELETE", userID, map[string]string{"id": notifID})
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleMarkAllRead handles POST /api/v1/notifications/mark-all-read.
func (s *Service) HandleMarkAllRead(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	tag, err := s.pool.Exec(r.Context(),
		`UPDATE notifications SET read = true WHERE user_id = $1 AND NOT read`,
		userID,
	)
	if err != nil {
		s.logger.Error("failed to mark all notifications read", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to mark all read")
		return
	}

	if s.bus != nil {
		_ = s.bus.PublishUserEvent(r.Context(), events.SubjectNotificationUpdate, "NOTIFICATION_MARK_ALL_READ", userID, map[string]int64{"updated": tag.RowsAffected()})
	}

	writeJSON(w, http.StatusOK, map[string]int64{"updated": tag.RowsAffected()})
}

// HandleClearAll handles DELETE /api/v1/notifications.
func (s *Service) HandleClearAll(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	_, err := s.pool.Exec(r.Context(),
		`DELETE FROM notifications WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to clear notifications")
		return
	}

	if s.bus != nil {
		_ = s.bus.PublishUserEvent(r.Context(), events.SubjectNotificationDelete, "NOTIFICATION_CLEAR_ALL", userID, nil)
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleSearchNotifications handles GET /api/v1/notifications/search.
func (s *Service) HandleSearchNotifications(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	q := r.URL.Query().Get("q")
	before := r.URL.Query().Get("before")
	limitStr := r.URL.Query().Get("limit")

	if q == "" {
		writeError(w, http.StatusBadRequest, "missing_query", "q parameter is required")
		return
	}

	limit := 50
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	pattern := "%" + q + "%"
	query := `SELECT id, user_id, type, category, guild_id, guild_name, guild_icon_id,
	          channel_id, channel_name, message_id, actor_id, actor_name, actor_avatar_id,
	          content, metadata, read, created_at
	          FROM notifications WHERE user_id = $1
	          AND (content ILIKE $2 OR actor_name ILIKE $2 OR guild_name ILIKE $2)`
	args := []interface{}{userID, pattern}
	argIdx := 3

	if before != "" {
		query += fmt.Sprintf(` AND id < $%d`, argIdx)
		args = append(args, before)
		argIdx++
	}

	query += ` ORDER BY id DESC LIMIT $` + strconv.Itoa(argIdx)
	args = append(args, limit)

	rows, err := s.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to search notifications")
		return
	}
	defer rows.Close()

	notifs := []models.Notification{}
	for rows.Next() {
		var n models.Notification
		if err := rows.Scan(&n.ID, &n.UserID, &n.Type, &n.Category, &n.GuildID, &n.GuildName,
			&n.GuildIconID, &n.ChannelID, &n.ChannelName, &n.MessageID, &n.ActorID, &n.ActorName,
			&n.ActorAvatarID, &n.Content, &n.Metadata, &n.Read, &n.CreatedAt); err != nil {
			continue
		}
		notifs = append(notifs, n)
	}

	writeJSON(w, http.StatusOK, notifs)
}

// HandleGetUnreadCount handles GET /api/v1/notifications/unread-count.
func (s *Service) HandleGetUnreadCount(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var count int
	err := s.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND NOT read`,
		userID,
	).Scan(&count)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to count unread")
		return
	}

	writeJSON(w, http.StatusOK, map[string]int{"count": count})
}

// HandleGetTypePreferences handles GET /api/v1/notifications/type-preferences.
func (s *Service) HandleGetTypePreferences(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	rows, err := s.pool.Query(r.Context(),
		`SELECT user_id, type, in_app, push, sound FROM notification_type_preferences WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to query type preferences")
		return
	}
	defer rows.Close()

	prefs := []models.NotificationTypePreference{}
	for rows.Next() {
		var p models.NotificationTypePreference
		if err := rows.Scan(&p.UserID, &p.Type, &p.InApp, &p.Push, &p.Sound); err != nil {
			continue
		}
		prefs = append(prefs, p)
	}

	writeJSON(w, http.StatusOK, prefs)
}

// HandleUpdateTypePreferences handles PUT /api/v1/notifications/type-preferences.
func (s *Service) HandleUpdateTypePreferences(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var req struct {
		Preferences []struct {
			Type  string `json:"type"`
			InApp bool   `json:"in_app"`
			Push  bool   `json:"push"`
			Sound bool   `json:"sound"`
		} `json:"preferences"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	for _, p := range req.Preferences {
		if p.Type == "" {
			continue
		}
		_, err := s.pool.Exec(r.Context(),
			`INSERT INTO notification_type_preferences (user_id, type, in_app, push, sound)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (user_id, type) DO UPDATE SET
			   in_app = EXCLUDED.in_app,
			   push = EXCLUDED.push,
			   sound = EXCLUDED.sound`,
			userID, p.Type, p.InApp, p.Push, p.Sound,
		)
		if err != nil {
			s.logger.Error("failed to upsert type preference",
				slog.String("type", p.Type),
				slog.String("error", err.Error()))
		}
	}

	// Return the full set after update.
	s.HandleGetTypePreferences(w, r)
}

// --- Notification Creation (used by notification worker) ---

// CreateNotification inserts a notification into the database and publishes
// a NOTIFICATION_CREATE event via NATS for real-time delivery. It checks
// the user's per-type preferences before inserting (skips if in_app disabled).
// If push is enabled for this type, it also sends a web push notification.
func (s *Service) CreateNotification(ctx context.Context, bus *events.Bus, n *models.Notification) error {
	// Check per-type preferences.
	var inApp, push bool
	inApp = true  // default
	push = true   // default

	err := s.pool.QueryRow(ctx,
		`SELECT in_app, push FROM notification_type_preferences WHERE user_id = $1 AND type = $2`,
		n.UserID, n.Type,
	).Scan(&inApp, &push)
	if err != nil && err != pgx.ErrNoRows {
		s.logger.Warn("failed to check type preferences", slog.String("error", err.Error()))
	}

	// Generate ULID if not set.
	if n.ID == "" {
		n.ID = models.NewULID().String()
	}

	// Set category if not set.
	if n.Category == "" {
		n.Category = models.NotificationCategoryForType(n.Type)
	}

	n.Read = false
	if n.CreatedAt.IsZero() {
		n.CreatedAt = time.Now().UTC()
	}

	// Insert notification if in-app delivery is enabled.
	if inApp {
		_, err = s.pool.Exec(ctx,
			`INSERT INTO notifications (id, user_id, type, category, guild_id, guild_name, guild_icon_id,
			 channel_id, channel_name, message_id, actor_id, actor_name, actor_avatar_id,
			 content, metadata, read, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`,
			n.ID, n.UserID, n.Type, n.Category, n.GuildID, n.GuildName, n.GuildIconID,
			n.ChannelID, n.ChannelName, n.MessageID, n.ActorID, n.ActorName, n.ActorAvatarID,
			n.Content, n.Metadata, false, n.CreatedAt,
		)
		if err != nil {
			return fmt.Errorf("inserting notification: %w", err)
		}

		// Publish NOTIFICATION_CREATE via NATS for real-time delivery.
		if bus != nil {
			_ = bus.PublishUserEvent(ctx, events.SubjectNotificationCreate, "NOTIFICATION_CREATE", n.UserID, n)
		}
	}

	// Send web push if enabled.
	if push && s.Enabled() {
		body := ""
		if n.Content != nil {
			body = *n.Content
		}
		title := n.ActorName
		url := ""
		if n.GuildID != nil && n.ChannelID != nil {
			url = fmt.Sprintf("/app/guilds/%s/channels/%s", *n.GuildID, *n.ChannelID)
			if n.GuildName != nil && n.ChannelName != nil {
				title = fmt.Sprintf("%s in #%s (%s)", n.ActorName, *n.ChannelName, *n.GuildName)
			}
		} else if n.ChannelID != nil {
			url = fmt.Sprintf("/app/dms/%s", *n.ChannelID)
		}

		_ = s.SendToUser(ctx, n.UserID, PushPayload{
			Type:      n.Type,
			Title:     title,
			Body:      body,
			URL:       url,
			ChannelID: derefString(n.ChannelID),
			GuildID:   derefString(n.GuildID),
			MessageID: derefString(n.MessageID),
		})
	}

	return nil
}

// CleanupOldNotifications removes notifications older than 90 days.
func (s *Service) CleanupOldNotifications(ctx context.Context) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM notifications WHERE created_at < now() - INTERVAL '90 days'`)
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 {
		s.logger.Info("cleaned old notifications",
			slog.Int64("deleted", tag.RowsAffected()))
	}
	return nil
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{"data": data})
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{"code": code, "message": message},
	})
}
