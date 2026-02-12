// Package integrations implements REST API handlers for external service
// integrations including ActivityPub (Mastodon/Lemmy), RSS feeds, calendar
// sync (Google Calendar/CalDAV), email-to-channel gateway, and SMS bridge.
// Mounted under /api/v1/guilds/{guildID}/integrations.
package integrations

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/models"
)

// Handler implements integration-related REST API endpoints.
type Handler struct {
	Pool     *pgxpool.Pool
	EventBus *events.Bus
	Logger   *slog.Logger
}

// --- Types ---

// Integration represents a guild integration record.
type Integration struct {
	ID              string    `json:"id"`
	GuildID         string    `json:"guild_id"`
	IntegrationType string    `json:"integration_type"`
	ChannelID       string    `json:"channel_id"`
	Name            string    `json:"name"`
	Enabled         bool      `json:"enabled"`
	Config          any       `json:"config"`
	CreatedBy       string    `json:"created_by"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// ActivityPubFollow represents a followed ActivityPub actor.
type ActivityPubFollow struct {
	ID              string     `json:"id"`
	IntegrationID   string     `json:"integration_id"`
	ActorURI        string     `json:"actor_uri"`
	ActorInbox      *string    `json:"actor_inbox,omitempty"`
	ActorName       *string    `json:"actor_name,omitempty"`
	ActorHandle     *string    `json:"actor_handle,omitempty"`
	ActorAvatarURL  *string    `json:"actor_avatar_url,omitempty"`
	LastFetchedAt   *time.Time `json:"last_fetched_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

// RSSFeed represents an RSS feed subscription.
type RSSFeed struct {
	ID                   string     `json:"id"`
	IntegrationID        string     `json:"integration_id"`
	FeedURL              string     `json:"feed_url"`
	Title                *string    `json:"title,omitempty"`
	Description          *string    `json:"description,omitempty"`
	LastItemID           *string    `json:"last_item_id,omitempty"`
	LastItemPublishedAt  *time.Time `json:"last_item_published_at,omitempty"`
	CheckIntervalSeconds int        `json:"check_interval_seconds"`
	LastCheckedAt        *time.Time `json:"last_checked_at,omitempty"`
	ErrorCount           int        `json:"error_count"`
	LastError            *string    `json:"last_error,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
}

// CalendarConnection represents a calendar sync connection.
type CalendarConnection struct {
	ID             string     `json:"id"`
	IntegrationID  string     `json:"integration_id"`
	Provider       string     `json:"provider"`
	CalendarURL    *string    `json:"calendar_url,omitempty"`
	CalendarName   *string    `json:"calendar_name,omitempty"`
	SyncDirection  string     `json:"sync_direction"`
	LastSyncedAt   *time.Time `json:"last_synced_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// EmailGateway represents an email-to-channel gateway configuration.
type EmailGateway struct {
	ID                  string   `json:"id"`
	IntegrationID       string   `json:"integration_id"`
	EmailAddress        string   `json:"email_address"`
	AllowedSenders      []string `json:"allowed_senders"`
	StripSignatures     bool     `json:"strip_signatures"`
	MaxAttachmentSizeMB int      `json:"max_attachment_size_mb"`
	CreatedAt           time.Time `json:"created_at"`
}

// SMSBridge represents an SMS bridge configuration.
type SMSBridge struct {
	ID             string   `json:"id"`
	IntegrationID  string   `json:"integration_id"`
	Provider       string   `json:"provider"`
	PhoneNumber    string   `json:"phone_number"`
	AllowedNumbers []string `json:"allowed_numbers"`
	CreatedAt      time.Time `json:"created_at"`
}

// BridgeConnection represents a Telegram/Slack/IRC bridge connection.
type BridgeConnection struct {
	ID        string    `json:"id"`
	GuildID   string    `json:"guild_id"`
	BridgeType string   `json:"bridge_type"`
	ChannelID string    `json:"channel_id"`
	RemoteID  string    `json:"remote_id"`
	Enabled   bool      `json:"enabled"`
	Config    any       `json:"config"`
	Status    string    `json:"status"`
	LastError *string   `json:"last_error,omitempty"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// --- Request types ---

type createIntegrationRequest struct {
	IntegrationType string `json:"integration_type"`
	ChannelID       string `json:"channel_id"`
	Name            string `json:"name"`
	Config          any    `json:"config"`
}

type updateIntegrationRequest struct {
	Name    *string `json:"name,omitempty"`
	Enabled *bool   `json:"enabled,omitempty"`
	Config  any     `json:"config,omitempty"`
}

type addActivityPubFollowRequest struct {
	ActorURI   string  `json:"actor_uri"`
	ActorName  *string `json:"actor_name,omitempty"`
	ActorHandle *string `json:"actor_handle,omitempty"`
}

type addRSSFeedRequest struct {
	FeedURL              string `json:"feed_url"`
	Title                string `json:"title"`
	CheckIntervalSeconds int    `json:"check_interval_seconds"`
}

type createCalendarConnectionRequest struct {
	Provider      string  `json:"provider"`
	CalendarURL   *string `json:"calendar_url,omitempty"`
	CalendarName  *string `json:"calendar_name,omitempty"`
	SyncDirection string  `json:"sync_direction"`
}

type createEmailGatewayRequest struct {
	AllowedSenders      []string `json:"allowed_senders"`
	StripSignatures     *bool    `json:"strip_signatures,omitempty"`
	MaxAttachmentSizeMB *int     `json:"max_attachment_size_mb,omitempty"`
}

type createSMSBridgeRequest struct {
	Provider       string   `json:"provider"`
	PhoneNumber    string   `json:"phone_number"`
	APIKey         string   `json:"api_key"`
	APISecret      string   `json:"api_secret"`
	AccountSID     string   `json:"account_sid"`
	AllowedNumbers []string `json:"allowed_numbers"`
}

type createBridgeConnectionRequest struct {
	BridgeType string `json:"bridge_type"`
	ChannelID  string `json:"channel_id"`
	RemoteID   string `json:"remote_id"`
	Config     any    `json:"config"`
}

type updateBridgeConnectionRequest struct {
	Enabled   *bool   `json:"enabled,omitempty"`
	RemoteID  *string `json:"remote_id,omitempty"`
	Config    any     `json:"config,omitempty"`
}

// ============================================================
// Integration CRUD
// ============================================================

// HandleListIntegrations lists all integrations for a guild.
// GET /api/v1/guilds/{guildID}/integrations
func (h *Handler) HandleListIntegrations(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")
	typeFilter := r.URL.Query().Get("type")

	query := `SELECT id, guild_id, integration_type, channel_id, name, enabled,
	                  config, created_by, created_at, updated_at
	           FROM guild_integrations WHERE guild_id = $1`
	args := []any{guildID}

	if typeFilter != "" {
		query += " AND integration_type = $2"
		args = append(args, typeFilter)
	}
	query += " ORDER BY created_at DESC"

	rows, err := h.Pool.Query(r.Context(), query, args...)
	if err != nil {
		h.Logger.Error("failed to list integrations", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list integrations")
		return
	}
	defer rows.Close()

	integrations := []Integration{}
	for rows.Next() {
		var i Integration
		var configJSON []byte
		if err := rows.Scan(&i.ID, &i.GuildID, &i.IntegrationType, &i.ChannelID,
			&i.Name, &i.Enabled, &configJSON, &i.CreatedBy, &i.CreatedAt, &i.UpdatedAt); err != nil {
			h.Logger.Error("failed to scan integration", slog.String("error", err.Error()))
			continue
		}
		json.Unmarshal(configJSON, &i.Config)
		integrations = append(integrations, i)
	}

	writeJSON(w, http.StatusOK, integrations)
}

// HandleCreateIntegration creates a new integration for a guild.
// POST /api/v1/guilds/{guildID}/integrations
func (h *Handler) HandleCreateIntegration(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	var req createIntegrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	// Validate integration type.
	validTypes := map[string]bool{
		"activitypub": true, "rss": true, "calendar": true,
		"email": true, "sms": true,
	}
	if !validTypes[req.IntegrationType] {
		writeError(w, http.StatusBadRequest, "invalid_type", "Integration type must be one of: activitypub, rss, calendar, email, sms")
		return
	}

	if req.ChannelID == "" {
		writeError(w, http.StatusBadRequest, "missing_channel", "Channel ID is required")
		return
	}
	if req.Name == "" || utf8.RuneCountInString(req.Name) > 100 {
		writeError(w, http.StatusBadRequest, "invalid_name", "Name is required and must be at most 100 characters")
		return
	}

	// Verify user is a guild member.
	if err := h.requireGuildMember(r, guildID, userID); err != nil {
		writeError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	// Verify the channel belongs to this guild.
	var channelGuildID *string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT guild_id FROM channels WHERE id = $1`, req.ChannelID).Scan(&channelGuildID)
	if err != nil || channelGuildID == nil || *channelGuildID != guildID {
		writeError(w, http.StatusBadRequest, "invalid_channel", "Channel does not belong to this guild")
		return
	}

	integrationID := models.NewULID().String()
	configJSON, _ := json.Marshal(req.Config)

	var integration Integration
	err = h.Pool.QueryRow(r.Context(),
		`INSERT INTO guild_integrations (id, guild_id, integration_type, channel_id, name, enabled, config, created_by, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, true, $6, $7, NOW(), NOW())
		 RETURNING id, guild_id, integration_type, channel_id, name, enabled, config, created_by, created_at, updated_at`,
		integrationID, guildID, req.IntegrationType, req.ChannelID, req.Name, configJSON, userID,
	).Scan(&integration.ID, &integration.GuildID, &integration.IntegrationType,
		&integration.ChannelID, &integration.Name, &integration.Enabled,
		&configJSON, &integration.CreatedBy, &integration.CreatedAt, &integration.UpdatedAt)
	if err != nil {
		h.Logger.Error("failed to create integration", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create integration")
		return
	}
	json.Unmarshal(configJSON, &integration.Config)

	h.Logger.Info("integration created",
		slog.String("guild_id", guildID),
		slog.String("integration_id", integrationID),
		slog.String("type", req.IntegrationType),
	)

	writeJSON(w, http.StatusCreated, integration)
}

// HandleGetIntegration retrieves a single integration.
// GET /api/v1/guilds/{guildID}/integrations/{integrationID}
func (h *Handler) HandleGetIntegration(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")
	integrationID := chi.URLParam(r, "integrationID")

	var i Integration
	var configJSON []byte
	err := h.Pool.QueryRow(r.Context(),
		`SELECT id, guild_id, integration_type, channel_id, name, enabled,
		        config, created_by, created_at, updated_at
		 FROM guild_integrations WHERE id = $1 AND guild_id = $2`,
		integrationID, guildID,
	).Scan(&i.ID, &i.GuildID, &i.IntegrationType, &i.ChannelID,
		&i.Name, &i.Enabled, &configJSON, &i.CreatedBy, &i.CreatedAt, &i.UpdatedAt)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "not_found", "Integration not found")
		return
	}
	if err != nil {
		h.Logger.Error("failed to get integration", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get integration")
		return
	}
	json.Unmarshal(configJSON, &i.Config)

	writeJSON(w, http.StatusOK, i)
}

// HandleUpdateIntegration updates an integration's name, enabled status, or config.
// PATCH /api/v1/guilds/{guildID}/integrations/{integrationID}
func (h *Handler) HandleUpdateIntegration(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")
	integrationID := chi.URLParam(r, "integrationID")

	var req updateIntegrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	// Build dynamic update.
	setClauses := []string{"updated_at = NOW()"}
	args := []any{integrationID, guildID}
	argIdx := 3

	if req.Name != nil {
		if utf8.RuneCountInString(*req.Name) > 100 {
			writeError(w, http.StatusBadRequest, "invalid_name", "Name must be at most 100 characters")
			return
		}
		setClauses = append(setClauses, "name = $"+itoa(argIdx))
		args = append(args, *req.Name)
		argIdx++
	}
	if req.Enabled != nil {
		setClauses = append(setClauses, "enabled = $"+itoa(argIdx))
		args = append(args, *req.Enabled)
		argIdx++
	}
	if req.Config != nil {
		configJSON, _ := json.Marshal(req.Config)
		setClauses = append(setClauses, "config = $"+itoa(argIdx))
		args = append(args, configJSON)
		argIdx++
	}

	query := "UPDATE guild_integrations SET " + strings.Join(setClauses, ", ") +
		" WHERE id = $1 AND guild_id = $2" +
		" RETURNING id, guild_id, integration_type, channel_id, name, enabled, config, created_by, created_at, updated_at"

	var i Integration
	var configJSON []byte
	err := h.Pool.QueryRow(r.Context(), query, args...).Scan(
		&i.ID, &i.GuildID, &i.IntegrationType, &i.ChannelID,
		&i.Name, &i.Enabled, &configJSON, &i.CreatedBy, &i.CreatedAt, &i.UpdatedAt)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "not_found", "Integration not found")
		return
	}
	if err != nil {
		h.Logger.Error("failed to update integration", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update integration")
		return
	}
	json.Unmarshal(configJSON, &i.Config)

	writeJSON(w, http.StatusOK, i)
}

// HandleDeleteIntegration deletes an integration and all associated resources.
// DELETE /api/v1/guilds/{guildID}/integrations/{integrationID}
func (h *Handler) HandleDeleteIntegration(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")
	integrationID := chi.URLParam(r, "integrationID")

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM guild_integrations WHERE id = $1 AND guild_id = $2`,
		integrationID, guildID)
	if err != nil {
		h.Logger.Error("failed to delete integration", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete integration")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Integration not found")
		return
	}

	h.Logger.Info("integration deleted",
		slog.String("guild_id", guildID),
		slog.String("integration_id", integrationID),
	)

	w.WriteHeader(http.StatusNoContent)
}

// ============================================================
// ActivityPub Follow Management
// ============================================================

// HandleListActivityPubFollows lists ActivityPub actors followed by an integration.
// GET /api/v1/guilds/{guildID}/integrations/{integrationID}/activitypub/follows
func (h *Handler) HandleListActivityPubFollows(w http.ResponseWriter, r *http.Request) {
	integrationID := chi.URLParam(r, "integrationID")

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, integration_id, actor_uri, actor_inbox, actor_name, actor_handle,
		        actor_avatar_url, last_fetched_at, created_at
		 FROM activitypub_follows WHERE integration_id = $1 ORDER BY created_at DESC`,
		integrationID)
	if err != nil {
		h.Logger.Error("failed to list follows", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list follows")
		return
	}
	defer rows.Close()

	follows := []ActivityPubFollow{}
	for rows.Next() {
		var f ActivityPubFollow
		if err := rows.Scan(&f.ID, &f.IntegrationID, &f.ActorURI, &f.ActorInbox,
			&f.ActorName, &f.ActorHandle, &f.ActorAvatarURL, &f.LastFetchedAt, &f.CreatedAt); err != nil {
			continue
		}
		follows = append(follows, f)
	}

	writeJSON(w, http.StatusOK, follows)
}

// HandleAddActivityPubFollow adds a new ActivityPub actor to follow.
// POST /api/v1/guilds/{guildID}/integrations/{integrationID}/activitypub/follows
func (h *Handler) HandleAddActivityPubFollow(w http.ResponseWriter, r *http.Request) {
	integrationID := chi.URLParam(r, "integrationID")

	var req addActivityPubFollowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.ActorURI == "" {
		writeError(w, http.StatusBadRequest, "missing_uri", "Actor URI is required")
		return
	}
	if _, err := url.ParseRequestURI(req.ActorURI); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_uri", "Actor URI must be a valid URL")
		return
	}

	// Verify integration exists and is activitypub type.
	var intType string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT integration_type FROM guild_integrations WHERE id = $1`, integrationID).Scan(&intType)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "not_found", "Integration not found")
		return
	}
	if intType != "activitypub" {
		writeError(w, http.StatusBadRequest, "wrong_type", "Integration is not an ActivityPub integration")
		return
	}

	followID := models.NewULID().String()
	var f ActivityPubFollow
	err = h.Pool.QueryRow(r.Context(),
		`INSERT INTO activitypub_follows (id, integration_id, actor_uri, actor_name, actor_handle, created_at)
		 VALUES ($1, $2, $3, $4, $5, NOW())
		 RETURNING id, integration_id, actor_uri, actor_inbox, actor_name, actor_handle,
		           actor_avatar_url, last_fetched_at, created_at`,
		followID, integrationID, req.ActorURI, req.ActorName, req.ActorHandle,
	).Scan(&f.ID, &f.IntegrationID, &f.ActorURI, &f.ActorInbox, &f.ActorName,
		&f.ActorHandle, &f.ActorAvatarURL, &f.LastFetchedAt, &f.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			writeError(w, http.StatusConflict, "already_following", "Already following this actor")
			return
		}
		h.Logger.Error("failed to add follow", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to add follow")
		return
	}

	writeJSON(w, http.StatusCreated, f)
}

// HandleRemoveActivityPubFollow removes an ActivityPub follow.
// DELETE /api/v1/guilds/{guildID}/integrations/{integrationID}/activitypub/follows/{followID}
func (h *Handler) HandleRemoveActivityPubFollow(w http.ResponseWriter, r *http.Request) {
	followID := chi.URLParam(r, "followID")
	integrationID := chi.URLParam(r, "integrationID")

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM activitypub_follows WHERE id = $1 AND integration_id = $2`,
		followID, integrationID)
	if err != nil {
		h.Logger.Error("failed to remove follow", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to remove follow")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Follow not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ============================================================
// RSS Feed Management
// ============================================================

// HandleListRSSFeeds lists RSS feeds for an integration.
// GET /api/v1/guilds/{guildID}/integrations/{integrationID}/rss/feeds
func (h *Handler) HandleListRSSFeeds(w http.ResponseWriter, r *http.Request) {
	integrationID := chi.URLParam(r, "integrationID")

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, integration_id, feed_url, title, description, last_item_id,
		        last_item_published_at, check_interval_seconds, last_checked_at,
		        error_count, last_error, created_at
		 FROM rss_feeds WHERE integration_id = $1 ORDER BY created_at DESC`,
		integrationID)
	if err != nil {
		h.Logger.Error("failed to list feeds", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list feeds")
		return
	}
	defer rows.Close()

	feeds := []RSSFeed{}
	for rows.Next() {
		var f RSSFeed
		if err := rows.Scan(&f.ID, &f.IntegrationID, &f.FeedURL, &f.Title, &f.Description,
			&f.LastItemID, &f.LastItemPublishedAt, &f.CheckIntervalSeconds, &f.LastCheckedAt,
			&f.ErrorCount, &f.LastError, &f.CreatedAt); err != nil {
			continue
		}
		feeds = append(feeds, f)
	}

	writeJSON(w, http.StatusOK, feeds)
}

// HandleAddRSSFeed adds a new RSS feed subscription.
// POST /api/v1/guilds/{guildID}/integrations/{integrationID}/rss/feeds
func (h *Handler) HandleAddRSSFeed(w http.ResponseWriter, r *http.Request) {
	integrationID := chi.URLParam(r, "integrationID")

	var req addRSSFeedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.FeedURL == "" {
		writeError(w, http.StatusBadRequest, "missing_url", "Feed URL is required")
		return
	}
	if _, err := url.ParseRequestURI(req.FeedURL); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_url", "Feed URL must be a valid URL")
		return
	}

	// Validate check interval (minimum 5 minutes, maximum 24 hours).
	if req.CheckIntervalSeconds < 300 {
		req.CheckIntervalSeconds = 300
	}
	if req.CheckIntervalSeconds > 86400 {
		req.CheckIntervalSeconds = 86400
	}

	// Verify integration exists and is rss type.
	var intType string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT integration_type FROM guild_integrations WHERE id = $1`, integrationID).Scan(&intType)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "not_found", "Integration not found")
		return
	}
	if intType != "rss" {
		writeError(w, http.StatusBadRequest, "wrong_type", "Integration is not an RSS integration")
		return
	}

	feedID := models.NewULID().String()
	var f RSSFeed
	err = h.Pool.QueryRow(r.Context(),
		`INSERT INTO rss_feeds (id, integration_id, feed_url, title, check_interval_seconds, created_at)
		 VALUES ($1, $2, $3, $4, $5, NOW())
		 RETURNING id, integration_id, feed_url, title, description, last_item_id,
		           last_item_published_at, check_interval_seconds, last_checked_at,
		           error_count, last_error, created_at`,
		feedID, integrationID, req.FeedURL, req.Title, req.CheckIntervalSeconds,
	).Scan(&f.ID, &f.IntegrationID, &f.FeedURL, &f.Title, &f.Description,
		&f.LastItemID, &f.LastItemPublishedAt, &f.CheckIntervalSeconds, &f.LastCheckedAt,
		&f.ErrorCount, &f.LastError, &f.CreatedAt)
	if err != nil {
		h.Logger.Error("failed to add RSS feed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to add RSS feed")
		return
	}

	writeJSON(w, http.StatusCreated, f)
}

// HandleRemoveRSSFeed removes an RSS feed subscription.
// DELETE /api/v1/guilds/{guildID}/integrations/{integrationID}/rss/feeds/{feedID}
func (h *Handler) HandleRemoveRSSFeed(w http.ResponseWriter, r *http.Request) {
	feedID := chi.URLParam(r, "feedID")
	integrationID := chi.URLParam(r, "integrationID")

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM rss_feeds WHERE id = $1 AND integration_id = $2`,
		feedID, integrationID)
	if err != nil {
		h.Logger.Error("failed to remove RSS feed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to remove feed")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Feed not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ============================================================
// Calendar Sync Management
// ============================================================

// HandleListCalendarConnections lists calendar connections for an integration.
// GET /api/v1/guilds/{guildID}/integrations/{integrationID}/calendar/connections
func (h *Handler) HandleListCalendarConnections(w http.ResponseWriter, r *http.Request) {
	integrationID := chi.URLParam(r, "integrationID")

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, integration_id, provider, calendar_url, calendar_name,
		        sync_direction, last_synced_at, created_at
		 FROM calendar_connections WHERE integration_id = $1 ORDER BY created_at DESC`,
		integrationID)
	if err != nil {
		h.Logger.Error("failed to list calendar connections", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list calendar connections")
		return
	}
	defer rows.Close()

	connections := []CalendarConnection{}
	for rows.Next() {
		var c CalendarConnection
		if err := rows.Scan(&c.ID, &c.IntegrationID, &c.Provider, &c.CalendarURL,
			&c.CalendarName, &c.SyncDirection, &c.LastSyncedAt, &c.CreatedAt); err != nil {
			continue
		}
		connections = append(connections, c)
	}

	writeJSON(w, http.StatusOK, connections)
}

// HandleCreateCalendarConnection creates a new calendar connection.
// POST /api/v1/guilds/{guildID}/integrations/{integrationID}/calendar/connections
func (h *Handler) HandleCreateCalendarConnection(w http.ResponseWriter, r *http.Request) {
	integrationID := chi.URLParam(r, "integrationID")

	var req createCalendarConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	validProviders := map[string]bool{"google": true, "caldav": true, "ical_url": true}
	if !validProviders[req.Provider] {
		writeError(w, http.StatusBadRequest, "invalid_provider", "Provider must be one of: google, caldav, ical_url")
		return
	}

	validDirections := map[string]bool{"import": true, "export": true, "both": true}
	if !validDirections[req.SyncDirection] {
		req.SyncDirection = "import"
	}

	// Verify integration exists and is calendar type.
	var intType string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT integration_type FROM guild_integrations WHERE id = $1`, integrationID).Scan(&intType)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "not_found", "Integration not found")
		return
	}
	if intType != "calendar" {
		writeError(w, http.StatusBadRequest, "wrong_type", "Integration is not a calendar integration")
		return
	}

	connID := models.NewULID().String()
	var c CalendarConnection
	err = h.Pool.QueryRow(r.Context(),
		`INSERT INTO calendar_connections (id, integration_id, provider, calendar_url, calendar_name, sync_direction, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, NOW())
		 RETURNING id, integration_id, provider, calendar_url, calendar_name,
		           sync_direction, last_synced_at, created_at`,
		connID, integrationID, req.Provider, req.CalendarURL, req.CalendarName, req.SyncDirection,
	).Scan(&c.ID, &c.IntegrationID, &c.Provider, &c.CalendarURL,
		&c.CalendarName, &c.SyncDirection, &c.LastSyncedAt, &c.CreatedAt)
	if err != nil {
		h.Logger.Error("failed to create calendar connection", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create calendar connection")
		return
	}

	writeJSON(w, http.StatusCreated, c)
}

// HandleDeleteCalendarConnection deletes a calendar connection.
// DELETE /api/v1/guilds/{guildID}/integrations/{integrationID}/calendar/connections/{connectionID}
func (h *Handler) HandleDeleteCalendarConnection(w http.ResponseWriter, r *http.Request) {
	connectionID := chi.URLParam(r, "connectionID")
	integrationID := chi.URLParam(r, "integrationID")

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM calendar_connections WHERE id = $1 AND integration_id = $2`,
		connectionID, integrationID)
	if err != nil {
		h.Logger.Error("failed to delete calendar connection", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete calendar connection")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Calendar connection not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleSyncCalendar triggers an immediate calendar sync.
// POST /api/v1/guilds/{guildID}/integrations/{integrationID}/calendar/connections/{connectionID}/sync
func (h *Handler) HandleSyncCalendar(w http.ResponseWriter, r *http.Request) {
	connectionID := chi.URLParam(r, "connectionID")
	integrationID := chi.URLParam(r, "integrationID")

	// Update the last_synced_at to now (the actual sync would be done by a background worker).
	tag, err := h.Pool.Exec(r.Context(),
		`UPDATE calendar_connections SET last_synced_at = NOW() WHERE id = $1 AND integration_id = $2`,
		connectionID, integrationID)
	if err != nil {
		h.Logger.Error("failed to trigger calendar sync", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to trigger sync")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Calendar connection not found")
		return
	}

	// In production, this would publish a NATS event to trigger the sync worker.
	if h.EventBus != nil {
		h.EventBus.PublishJSON(r.Context(), "amityvox.integration.calendar_sync", "CALENDAR_SYNC", map[string]string{
			"connection_id":  connectionID,
			"integration_id": integrationID,
		})
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "sync_triggered"})
}

// ============================================================
// Email Gateway Management
// ============================================================

// HandleGetEmailGateway retrieves the email gateway config for an integration.
// GET /api/v1/guilds/{guildID}/integrations/{integrationID}/email/gateway
func (h *Handler) HandleGetEmailGateway(w http.ResponseWriter, r *http.Request) {
	integrationID := chi.URLParam(r, "integrationID")

	var gw EmailGateway
	err := h.Pool.QueryRow(r.Context(),
		`SELECT id, integration_id, email_address, allowed_senders,
		        strip_signatures, max_attachment_size_mb, created_at
		 FROM email_gateways WHERE integration_id = $1`,
		integrationID,
	).Scan(&gw.ID, &gw.IntegrationID, &gw.EmailAddress, &gw.AllowedSenders,
		&gw.StripSignatures, &gw.MaxAttachmentSizeMB, &gw.CreatedAt)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "not_found", "Email gateway not configured")
		return
	}
	if err != nil {
		h.Logger.Error("failed to get email gateway", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get email gateway")
		return
	}

	writeJSON(w, http.StatusOK, gw)
}

// HandleCreateEmailGateway creates or updates the email gateway for an integration.
// POST /api/v1/guilds/{guildID}/integrations/{integrationID}/email/gateway
func (h *Handler) HandleCreateEmailGateway(w http.ResponseWriter, r *http.Request) {
	integrationID := chi.URLParam(r, "integrationID")

	var req createEmailGatewayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	// Verify integration exists and is email type.
	var intType string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT integration_type FROM guild_integrations WHERE id = $1`,
		integrationID).Scan(&intType)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "not_found", "Integration not found")
		return
	}
	if intType != "email" {
		writeError(w, http.StatusBadRequest, "wrong_type", "Integration is not an email integration")
		return
	}

	stripSig := true
	if req.StripSignatures != nil {
		stripSig = *req.StripSignatures
	}
	maxSize := 10
	if req.MaxAttachmentSizeMB != nil && *req.MaxAttachmentSizeMB > 0 {
		maxSize = *req.MaxAttachmentSizeMB
	}
	if maxSize > 25 {
		maxSize = 25
	}

	// Generate a unique email address based on integration ID.
	emailAddr := "channel-" + integrationID[:8] + "@amityvox.chat"

	gwID := models.NewULID().String()
	var gw EmailGateway
	err = h.Pool.QueryRow(r.Context(),
		`INSERT INTO email_gateways (id, integration_id, email_address, allowed_senders, strip_signatures, max_attachment_size_mb, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, NOW())
		 ON CONFLICT (email_address) DO UPDATE SET
		     allowed_senders = EXCLUDED.allowed_senders,
		     strip_signatures = EXCLUDED.strip_signatures,
		     max_attachment_size_mb = EXCLUDED.max_attachment_size_mb
		 RETURNING id, integration_id, email_address, allowed_senders,
		           strip_signatures, max_attachment_size_mb, created_at`,
		gwID, integrationID, emailAddr, req.AllowedSenders, stripSig, maxSize,
	).Scan(&gw.ID, &gw.IntegrationID, &gw.EmailAddress, &gw.AllowedSenders,
		&gw.StripSignatures, &gw.MaxAttachmentSizeMB, &gw.CreatedAt)
	if err != nil {
		h.Logger.Error("failed to create email gateway", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create email gateway")
		return
	}

	writeJSON(w, http.StatusCreated, gw)
}

// HandleDeleteEmailGateway deletes the email gateway for an integration.
// DELETE /api/v1/guilds/{guildID}/integrations/{integrationID}/email/gateway
func (h *Handler) HandleDeleteEmailGateway(w http.ResponseWriter, r *http.Request) {
	integrationID := chi.URLParam(r, "integrationID")

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM email_gateways WHERE integration_id = $1`, integrationID)
	if err != nil {
		h.Logger.Error("failed to delete email gateway", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete email gateway")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Email gateway not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ============================================================
// SMS Bridge Management
// ============================================================

// HandleGetSMSBridge retrieves the SMS bridge config for an integration.
// GET /api/v1/guilds/{guildID}/integrations/{integrationID}/sms/bridge
func (h *Handler) HandleGetSMSBridge(w http.ResponseWriter, r *http.Request) {
	integrationID := chi.URLParam(r, "integrationID")

	var sb SMSBridge
	err := h.Pool.QueryRow(r.Context(),
		`SELECT id, integration_id, provider, phone_number, allowed_numbers, created_at
		 FROM sms_bridges WHERE integration_id = $1`,
		integrationID,
	).Scan(&sb.ID, &sb.IntegrationID, &sb.Provider, &sb.PhoneNumber,
		&sb.AllowedNumbers, &sb.CreatedAt)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "not_found", "SMS bridge not configured")
		return
	}
	if err != nil {
		h.Logger.Error("failed to get SMS bridge", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get SMS bridge")
		return
	}

	writeJSON(w, http.StatusOK, sb)
}

// HandleCreateSMSBridge creates the SMS bridge for an integration.
// POST /api/v1/guilds/{guildID}/integrations/{integrationID}/sms/bridge
func (h *Handler) HandleCreateSMSBridge(w http.ResponseWriter, r *http.Request) {
	integrationID := chi.URLParam(r, "integrationID")

	var req createSMSBridgeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.PhoneNumber == "" {
		writeError(w, http.StatusBadRequest, "missing_phone", "Phone number is required in E.164 format")
		return
	}
	if !strings.HasPrefix(req.PhoneNumber, "+") {
		writeError(w, http.StatusBadRequest, "invalid_phone", "Phone number must be in E.164 format (e.g., +15551234567)")
		return
	}

	validProviders := map[string]bool{"twilio": true, "vonage": true}
	if !validProviders[req.Provider] {
		req.Provider = "twilio"
	}

	if req.APIKey == "" || req.APISecret == "" {
		writeError(w, http.StatusBadRequest, "missing_credentials", "API key and secret are required")
		return
	}

	// Verify integration exists and is sms type.
	var intType string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT integration_type FROM guild_integrations WHERE id = $1`, integrationID).Scan(&intType)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "not_found", "Integration not found")
		return
	}
	if intType != "sms" {
		writeError(w, http.StatusBadRequest, "wrong_type", "Integration is not an SMS integration")
		return
	}

	bridgeID := models.NewULID().String()
	var sb SMSBridge
	err = h.Pool.QueryRow(r.Context(),
		`INSERT INTO sms_bridges (id, integration_id, provider, phone_number,
		                          api_key_encrypted, api_secret_encrypted, account_sid,
		                          allowed_numbers, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
		 RETURNING id, integration_id, provider, phone_number, allowed_numbers, created_at`,
		bridgeID, integrationID, req.Provider, req.PhoneNumber,
		req.APIKey, req.APISecret, req.AccountSID, req.AllowedNumbers,
	).Scan(&sb.ID, &sb.IntegrationID, &sb.Provider, &sb.PhoneNumber,
		&sb.AllowedNumbers, &sb.CreatedAt)
	if err != nil {
		h.Logger.Error("failed to create SMS bridge", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create SMS bridge")
		return
	}

	writeJSON(w, http.StatusCreated, sb)
}

// HandleDeleteSMSBridge deletes the SMS bridge for an integration.
// DELETE /api/v1/guilds/{guildID}/integrations/{integrationID}/sms/bridge
func (h *Handler) HandleDeleteSMSBridge(w http.ResponseWriter, r *http.Request) {
	integrationID := chi.URLParam(r, "integrationID")

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM sms_bridges WHERE integration_id = $1`, integrationID)
	if err != nil {
		h.Logger.Error("failed to delete SMS bridge", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete SMS bridge")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "SMS bridge not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ============================================================
// Bridge Connections (Telegram, Slack, IRC)
// ============================================================

// HandleListBridgeConnections lists all bridge connections for a guild.
// GET /api/v1/guilds/{guildID}/bridge-connections
func (h *Handler) HandleListBridgeConnections(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")
	typeFilter := r.URL.Query().Get("type")

	query := `SELECT id, guild_id, bridge_type, channel_id, remote_id, enabled,
	                  config, status, last_error, created_by, created_at, updated_at
	           FROM bridge_connections WHERE guild_id = $1`
	args := []any{guildID}

	if typeFilter != "" {
		query += " AND bridge_type = $2"
		args = append(args, typeFilter)
	}
	query += " ORDER BY created_at DESC"

	rows, err := h.Pool.Query(r.Context(), query, args...)
	if err != nil {
		h.Logger.Error("failed to list bridge connections", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list bridge connections")
		return
	}
	defer rows.Close()

	connections := []BridgeConnection{}
	for rows.Next() {
		var bc BridgeConnection
		var configJSON []byte
		if err := rows.Scan(&bc.ID, &bc.GuildID, &bc.BridgeType, &bc.ChannelID,
			&bc.RemoteID, &bc.Enabled, &configJSON, &bc.Status, &bc.LastError,
			&bc.CreatedBy, &bc.CreatedAt, &bc.UpdatedAt); err != nil {
			continue
		}
		json.Unmarshal(configJSON, &bc.Config)
		connections = append(connections, bc)
	}

	writeJSON(w, http.StatusOK, connections)
}

// HandleCreateBridgeConnection creates a new bridge connection (Telegram, Slack, or IRC).
// POST /api/v1/guilds/{guildID}/bridge-connections
func (h *Handler) HandleCreateBridgeConnection(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	var req createBridgeConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	validTypes := map[string]bool{"telegram": true, "slack": true, "irc": true}
	if !validTypes[req.BridgeType] {
		writeError(w, http.StatusBadRequest, "invalid_type", "Bridge type must be one of: telegram, slack, irc")
		return
	}

	if req.ChannelID == "" {
		writeError(w, http.StatusBadRequest, "missing_channel", "Channel ID is required")
		return
	}
	if req.RemoteID == "" {
		writeError(w, http.StatusBadRequest, "missing_remote_id", "Remote ID is required (Telegram chat ID, Slack channel, or IRC channel)")
		return
	}

	// Verify the channel belongs to this guild.
	var channelGuildID *string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT guild_id FROM channels WHERE id = $1`, req.ChannelID).Scan(&channelGuildID)
	if err != nil || channelGuildID == nil || *channelGuildID != guildID {
		writeError(w, http.StatusBadRequest, "invalid_channel", "Channel does not belong to this guild")
		return
	}

	connID := models.NewULID().String()
	configJSON, _ := json.Marshal(req.Config)

	var bc BridgeConnection
	err = h.Pool.QueryRow(r.Context(),
		`INSERT INTO bridge_connections (id, guild_id, bridge_type, channel_id, remote_id,
		                                 enabled, config, status, created_by, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, true, $6, 'disconnected', $7, NOW(), NOW())
		 RETURNING id, guild_id, bridge_type, channel_id, remote_id, enabled,
		           config, status, last_error, created_by, created_at, updated_at`,
		connID, guildID, req.BridgeType, req.ChannelID, req.RemoteID, configJSON, userID,
	).Scan(&bc.ID, &bc.GuildID, &bc.BridgeType, &bc.ChannelID, &bc.RemoteID,
		&bc.Enabled, &configJSON, &bc.Status, &bc.LastError, &bc.CreatedBy,
		&bc.CreatedAt, &bc.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			writeError(w, http.StatusConflict, "already_exists", "A bridge connection for this remote channel already exists")
			return
		}
		h.Logger.Error("failed to create bridge connection", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create bridge connection")
		return
	}
	json.Unmarshal(configJSON, &bc.Config)

	h.Logger.Info("bridge connection created",
		slog.String("guild_id", guildID),
		slog.String("bridge_type", req.BridgeType),
		slog.String("remote_id", req.RemoteID),
	)

	writeJSON(w, http.StatusCreated, bc)
}

// HandleUpdateBridgeConnection updates a bridge connection.
// PATCH /api/v1/guilds/{guildID}/bridge-connections/{connectionID}
func (h *Handler) HandleUpdateBridgeConnection(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")
	connectionID := chi.URLParam(r, "connectionID")

	var req updateBridgeConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	setClauses := []string{"updated_at = NOW()"}
	args := []any{connectionID, guildID}
	argIdx := 3

	if req.Enabled != nil {
		setClauses = append(setClauses, "enabled = $"+itoa(argIdx))
		args = append(args, *req.Enabled)
		argIdx++
	}
	if req.RemoteID != nil {
		setClauses = append(setClauses, "remote_id = $"+itoa(argIdx))
		args = append(args, *req.RemoteID)
		argIdx++
	}
	if req.Config != nil {
		configJSON, _ := json.Marshal(req.Config)
		setClauses = append(setClauses, "config = $"+itoa(argIdx))
		args = append(args, configJSON)
		argIdx++
	}

	query := "UPDATE bridge_connections SET " + strings.Join(setClauses, ", ") +
		" WHERE id = $1 AND guild_id = $2" +
		" RETURNING id, guild_id, bridge_type, channel_id, remote_id, enabled, config, status, last_error, created_by, created_at, updated_at"

	var bc BridgeConnection
	var configJSON []byte
	err := h.Pool.QueryRow(r.Context(), query, args...).Scan(
		&bc.ID, &bc.GuildID, &bc.BridgeType, &bc.ChannelID, &bc.RemoteID,
		&bc.Enabled, &configJSON, &bc.Status, &bc.LastError, &bc.CreatedBy,
		&bc.CreatedAt, &bc.UpdatedAt)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "not_found", "Bridge connection not found")
		return
	}
	if err != nil {
		h.Logger.Error("failed to update bridge connection", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update bridge connection")
		return
	}
	json.Unmarshal(configJSON, &bc.Config)

	writeJSON(w, http.StatusOK, bc)
}

// HandleDeleteBridgeConnection deletes a bridge connection.
// DELETE /api/v1/guilds/{guildID}/bridge-connections/{connectionID}
func (h *Handler) HandleDeleteBridgeConnection(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")
	connectionID := chi.URLParam(r, "connectionID")

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM bridge_connections WHERE id = $1 AND guild_id = $2`,
		connectionID, guildID)
	if err != nil {
		h.Logger.Error("failed to delete bridge connection", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete bridge connection")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Bridge connection not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ============================================================
// Integration Message Log
// ============================================================

// HandleGetIntegrationLog retrieves the message log for an integration or guild.
// GET /api/v1/guilds/{guildID}/integrations/log
func (h *Handler) HandleGetIntegrationLog(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")
	limit := 50

	rows, err := h.Pool.Query(r.Context(),
		`SELECT l.id, l.integration_id, l.bridge_connection_id, l.direction,
		        l.source_id, l.amityvox_message_id, l.channel_id, l.status,
		        l.error_message, l.created_at
		 FROM integration_message_log l
		 JOIN channels c ON c.id = l.channel_id
		 WHERE c.guild_id = $1
		 ORDER BY l.created_at DESC
		 LIMIT $2`,
		guildID, limit)
	if err != nil {
		h.Logger.Error("failed to get integration log", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get integration log")
		return
	}
	defer rows.Close()

	type LogEntry struct {
		ID                 string     `json:"id"`
		IntegrationID      *string    `json:"integration_id,omitempty"`
		BridgeConnectionID *string    `json:"bridge_connection_id,omitempty"`
		Direction          string     `json:"direction"`
		SourceID           *string    `json:"source_id,omitempty"`
		AmityVoxMessageID  *string    `json:"amityvox_message_id,omitempty"`
		ChannelID          string     `json:"channel_id"`
		Status             string     `json:"status"`
		ErrorMessage       *string    `json:"error_message,omitempty"`
		CreatedAt          time.Time  `json:"created_at"`
	}

	entries := []LogEntry{}
	for rows.Next() {
		var e LogEntry
		if err := rows.Scan(&e.ID, &e.IntegrationID, &e.BridgeConnectionID, &e.Direction,
			&e.SourceID, &e.AmityVoxMessageID, &e.ChannelID, &e.Status,
			&e.ErrorMessage, &e.CreatedAt); err != nil {
			continue
		}
		entries = append(entries, e)
	}

	writeJSON(w, http.StatusOK, entries)
}

// ============================================================
// Helpers
// ============================================================

func (h *Handler) requireGuildMember(r *http.Request, guildID, userID string) error {
	var exists bool
	err := h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM guild_members WHERE guild_id = $1 AND user_id = $2)`,
		guildID, userID).Scan(&exists)
	if err != nil || !exists {
		return pgx.ErrNoRows
	}
	return nil
}

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

// itoa converts an integer to its string representation for building
// parameterized SQL queries (e.g., "$3", "$4").
func itoa(n int) string {
	return strconv.Itoa(n)
}
