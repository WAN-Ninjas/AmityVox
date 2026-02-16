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
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/amityvox/amityvox/internal/auth"
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
	SuppressEveryone  bool       `json:"suppress_everyone"`
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
}

// Config holds configuration for the notification service.
type Config struct {
	Pool             *pgxpool.Pool
	Logger           *slog.Logger
	VAPIDPublicKey   string
	VAPIDPrivateKey  string
	VAPIDContactEmail string
}

// NewService creates a new notification service.
func NewService(cfg Config) *Service {
	return &Service{
		pool:       cfg.Pool,
		logger:     cfg.Logger,
		vapidPub:   cfg.VAPIDPublicKey,
		vapidPriv:  cfg.VAPIDPrivateKey,
		vapidEmail: cfg.VAPIDContactEmail,
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

	query := `SELECT user_id, guild_id, level, suppress_everyone, suppress_roles, muted_until
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
		&prefs.SuppressEveryone, &prefs.SuppressRoles, &mutedUntil,
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
		SuppressEveryone *bool      `json:"suppress_everyone"`
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

	suppressEveryone := false
	if req.SuppressEveryone != nil {
		suppressEveryone = *req.SuppressEveryone
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
		`INSERT INTO notification_preferences (user_id, guild_id, level, suppress_everyone, suppress_roles, muted_until)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (user_id, guild_id) DO UPDATE SET
		   level = EXCLUDED.level,
		   suppress_everyone = EXCLUDED.suppress_everyone,
		   suppress_roles = EXCLUDED.suppress_roles,
		   muted_until = EXCLUDED.muted_until`,
		userID, guildIDVal, level, suppressEveryone, suppressRoles, req.MutedUntil,
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
		SuppressEveryone: suppressEveryone,
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

	if req.Level == "" {
		req.Level = LevelMentions
	}
	validLevels := map[string]bool{LevelAll: true, LevelMentions: true, LevelNone: true}
	if !validLevels[req.Level] {
		writeError(w, http.StatusBadRequest, "invalid_level", "Level must be all, mentions, or none")
		return
	}

	_, err := s.pool.Exec(r.Context(),
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
func (s *Service) ShouldNotify(ctx context.Context, userID, guildID, channelID string, isMention, isDM, isEveryone bool) bool {
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
				return isMention || isEveryone || isDM
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
	var suppressEveryone, suppressRoles bool
	var mutedUntil *time.Time

	err := s.pool.QueryRow(ctx,
		`SELECT level, suppress_everyone, suppress_roles, muted_until
		 FROM notification_preferences
		 WHERE user_id = $1 AND guild_id = $2`,
		userID, guildID,
	).Scan(&level, &suppressEveryone, &suppressRoles, &mutedUntil)

	if err != nil {
		// No guild preferences — check global.
		err = s.pool.QueryRow(ctx,
			`SELECT level, suppress_everyone, suppress_roles, muted_until
			 FROM notification_preferences
			 WHERE user_id = $1 AND guild_id = '__global__'`,
			userID,
		).Scan(&level, &suppressEveryone, &suppressRoles, &mutedUntil)
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
		if isEveryone && suppressEveryone {
			return false
		}
		return isMention || isEveryone
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
