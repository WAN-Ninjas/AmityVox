// Package integrations implements REST API handlers for external service
// integrations including ActivityPub (Mastodon/Lemmy) and bridge connections
// (Telegram, Slack, IRC). Mounted under /api/v1/guilds/{guildID}/integrations.
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
		"activitypub": true,
	}
	if !validTypes[req.IntegrationType] {
		writeError(w, http.StatusBadRequest, "invalid_type", "Integration type must be: activitypub")
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
