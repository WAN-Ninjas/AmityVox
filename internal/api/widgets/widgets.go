// Package widgets implements REST API handlers for guild server widgets and
// channel interactive widgets. Server widgets allow embedding guild info on
// external websites. Channel widgets enable interactive embeds (notes, YouTube,
// countdown timers) within channels. Also handles the plugin marketplace,
// guild plugin management, and key backup/recovery for E2E encryption.
// Mounted under /api/v1/.
package widgets

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/amityvox/amityvox/internal/api/apiutil"
	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/models"
)

// Handler implements widget-related REST API endpoints.
type Handler struct {
	Pool       *pgxpool.Pool
	EventBus   *events.Bus
	InstanceID string
	Logger     *slog.Logger
}

// --- Types ---

// GuildWidget represents the embeddable server widget configuration.
type GuildWidget struct {
	GuildID         string  `json:"guild_id"`
	Enabled         bool    `json:"enabled"`
	InviteChannelID *string `json:"invite_channel_id,omitempty"`
	Style           string  `json:"style"`
	UpdatedAt       string  `json:"updated_at"`
}

// GuildWidgetEmbed is the public widget data served to external websites.
type GuildWidgetEmbed struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Description   *string         `json:"description,omitempty"`
	IconID        *string         `json:"icon_id,omitempty"`
	OnlineCount   int             `json:"online_count"`
	MemberCount   int             `json:"member_count"`
	InviteCode    *string         `json:"invite_code,omitempty"`
	Channels      []WidgetChannel `json:"channels"`
	OnlineMembers []WidgetMember  `json:"online_members"`
}

// WidgetChannel is a minimal channel representation for the widget embed.
type WidgetChannel struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Position int    `json:"position"`
}

// WidgetMember is a minimal member representation for the widget embed.
type WidgetMember struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName *string `json:"display_name,omitempty"`
	AvatarID    *string `json:"avatar_id,omitempty"`
	Status      string  `json:"status"`
}

// ChannelWidget represents an interactive widget embedded in a channel.
type ChannelWidget struct {
	ID         string          `json:"id"`
	ChannelID  string          `json:"channel_id"`
	GuildID    string          `json:"guild_id"`
	WidgetType string          `json:"widget_type"`
	Title      string          `json:"title"`
	Config     json.RawMessage `json:"config"`
	CreatorID  string          `json:"creator_id"`
	Position   int             `json:"position"`
	Active     bool            `json:"active"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

type updateGuildWidgetRequest struct {
	Enabled         *bool   `json:"enabled"`
	InviteChannelID *string `json:"invite_channel_id"`
	Style           *string `json:"style"`
}

type createChannelWidgetRequest struct {
	WidgetType string          `json:"widget_type"`
	Title      string          `json:"title"`
	Config     json.RawMessage `json:"config"`
}

type updateChannelWidgetRequest struct {
	Title    *string          `json:"title"`
	Config   *json.RawMessage `json:"config"`
	Position *int             `json:"position"`
	Active   *bool            `json:"active"`
}

// Valid widget types for channel widgets.
var validWidgetTypes = map[string]bool{
	"notes":         true,
	"youtube":       true,
	"countdown":     true,
	"custom_iframe": true,
}

// Valid styles for server widgets.
var validStyles = map[string]bool{
	"banner_1": true,
	"banner_2": true,
	"shield":   true,
}

// ============================================================
// Server Widget Endpoints
// ============================================================

// HandleGetGuildWidget returns the guild widget configuration.
// GET /api/v1/guilds/{guildID}/widget
func (h *Handler) HandleGetGuildWidget(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")

	var gw GuildWidget
	err := h.Pool.QueryRow(r.Context(),
		`SELECT guild_id, enabled, invite_channel_id, style, updated_at
		 FROM guild_widgets WHERE guild_id = $1`, guildID,
	).Scan(&gw.GuildID, &gw.Enabled, &gw.InviteChannelID, &gw.Style, &gw.UpdatedAt)

	if err == pgx.ErrNoRows {
		// Return default disabled widget.
		apiutil.WriteJSON(w, http.StatusOK, GuildWidget{
			GuildID: guildID,
			Enabled: false,
			Style:   "banner_1",
		})
		return
	}
	if err != nil {
		h.Logger.Error("failed to get guild widget", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get guild widget")
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, gw)
}

// HandleUpdateGuildWidget updates the guild widget configuration.
// PATCH /api/v1/guilds/{guildID}/widget
func (h *Handler) HandleUpdateGuildWidget(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	// Only guild owner or admin can update widget settings.
	if !h.isGuildAdmin(r, guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Only the guild owner or an admin can manage the widget")
		return
	}

	var req updateGuildWidgetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Style != nil && !validStyles[*req.Style] {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_style", "Valid styles are: banner_1, banner_2, shield")
		return
	}

	var gw GuildWidget
	err := h.Pool.QueryRow(r.Context(),
		`INSERT INTO guild_widgets (guild_id, enabled, invite_channel_id, style, updated_at)
		 VALUES ($1, COALESCE($2, false), $3, COALESCE($4, 'banner_1'), NOW())
		 ON CONFLICT (guild_id) DO UPDATE SET
		   enabled = COALESCE($2, guild_widgets.enabled),
		   invite_channel_id = COALESCE($3, guild_widgets.invite_channel_id),
		   style = COALESCE($4, guild_widgets.style),
		   updated_at = NOW()
		 RETURNING guild_id, enabled, invite_channel_id, style, updated_at`,
		guildID, req.Enabled, req.InviteChannelID, req.Style,
	).Scan(&gw.GuildID, &gw.Enabled, &gw.InviteChannelID, &gw.Style, &gw.UpdatedAt)

	if err != nil {
		h.Logger.Error("failed to update guild widget", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update guild widget")
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, gw)
}

// HandleGetGuildWidgetEmbed returns the public embeddable widget data (no auth required).
// GET /api/v1/guilds/{guildID}/widget.json
func (h *Handler) HandleGetGuildWidgetEmbed(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")

	// Check if widget is enabled.
	var enabled bool
	var inviteChannelID *string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT enabled, invite_channel_id FROM guild_widgets WHERE guild_id = $1`, guildID,
	).Scan(&enabled, &inviteChannelID)

	if err == pgx.ErrNoRows || !enabled {
		apiutil.WriteError(w, http.StatusForbidden, "widget_disabled", "The widget is not enabled for this guild")
		return
	}
	if err != nil {
		h.Logger.Error("failed to check widget status", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to load widget")
		return
	}

	// Fetch guild info.
	var embed GuildWidgetEmbed
	err = h.Pool.QueryRow(r.Context(),
		`SELECT id, name, description, icon_id,
		  (SELECT COUNT(*) FROM guild_members WHERE guild_id = $1)
		 FROM guilds WHERE id = $1`, guildID,
	).Scan(&embed.ID, &embed.Name, &embed.Description, &embed.IconID, &embed.MemberCount)
	if err != nil {
		h.Logger.Error("failed to fetch guild for widget", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusNotFound, "guild_not_found", "Guild not found")
		return
	}

	// Fetch public text channels (limited to 25).
	channelRows, err := h.Pool.Query(r.Context(),
		`SELECT id, name, position FROM channels
		 WHERE guild_id = $1 AND channel_type = 'text' AND NOT nsfw
		 ORDER BY position ASC LIMIT 25`, guildID,
	)
	if err == nil {
		defer channelRows.Close()
		channels := make([]WidgetChannel, 0)
		for channelRows.Next() {
			var ch WidgetChannel
			if err := channelRows.Scan(&ch.ID, &ch.Name, &ch.Position); err == nil {
				channels = append(channels, ch)
			}
		}
		embed.Channels = channels
	}

	// Fetch online members (limited to 100).
	memberRows, err := h.Pool.Query(r.Context(),
		`SELECT u.id, u.username, u.display_name, u.avatar_id, u.status_presence
		 FROM guild_members gm
		 JOIN users u ON u.id = gm.user_id
		 WHERE gm.guild_id = $1 AND u.status_presence IN ('online', 'idle', 'dnd')
		 LIMIT 100`, guildID,
	)
	if err == nil {
		defer memberRows.Close()
		members := make([]WidgetMember, 0)
		onlineCount := 0
		for memberRows.Next() {
			var m WidgetMember
			if err := memberRows.Scan(&m.ID, &m.Username, &m.DisplayName, &m.AvatarID, &m.Status); err == nil {
				members = append(members, m)
				onlineCount++
			}
		}
		embed.OnlineMembers = members
		embed.OnlineCount = onlineCount
	}

	// Generate an invite if an invite channel is configured.
	if inviteChannelID != nil {
		var code string
		err = h.Pool.QueryRow(r.Context(),
			`SELECT code FROM invites
			 WHERE guild_id = $1 AND channel_id = $2
			   AND (expires_at IS NULL OR expires_at > NOW())
			   AND (max_uses IS NULL OR uses < max_uses)
			 ORDER BY created_at DESC LIMIT 1`,
			guildID, *inviteChannelID,
		).Scan(&code)
		if err == nil {
			embed.InviteCode = &code
		}
	}

	apiutil.WriteJSON(w, http.StatusOK, embed)
}

// ============================================================
// Channel Widget Endpoints
// ============================================================

// HandleGetChannelWidgets returns all widgets for a channel.
// GET /api/v1/channels/{channelID}/widgets
func (h *Handler) HandleGetChannelWidgets(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, channel_id, guild_id, widget_type, title, config, creator_id,
		        position, active, created_at, updated_at
		 FROM channel_widgets
		 WHERE channel_id = $1
		 ORDER BY position ASC`, channelID,
	)
	if err != nil {
		h.Logger.Error("failed to get channel widgets", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get widgets")
		return
	}
	defer rows.Close()

	widgets := make([]ChannelWidget, 0)
	for rows.Next() {
		var cw ChannelWidget
		if err := rows.Scan(&cw.ID, &cw.ChannelID, &cw.GuildID, &cw.WidgetType,
			&cw.Title, &cw.Config, &cw.CreatorID, &cw.Position, &cw.Active,
			&cw.CreatedAt, &cw.UpdatedAt); err != nil {
			h.Logger.Error("failed to scan channel widget", slog.String("error", err.Error()))
			continue
		}
		widgets = append(widgets, cw)
	}

	apiutil.WriteJSON(w, http.StatusOK, widgets)
}

// HandleCreateChannelWidget creates a new widget in a channel.
// POST /api/v1/channels/{channelID}/widgets
func (h *Handler) HandleCreateChannelWidget(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	var req createChannelWidgetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Title == "" {
		apiutil.WriteError(w, http.StatusBadRequest, "missing_title", "Title is required")
		return
	}
	if !validWidgetTypes[req.WidgetType] {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_widget_type",
			"Valid types: notes, youtube, countdown, custom_iframe")
		return
	}
	if req.Config == nil {
		req.Config = json.RawMessage(`{}`)
	}

	// Get guild_id from channel.
	var guildID *string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT guild_id FROM channels WHERE id = $1`, channelID,
	).Scan(&guildID)
	if err != nil {
		apiutil.WriteError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
		return
	}
	if guildID == nil {
		apiutil.WriteError(w, http.StatusBadRequest, "not_guild_channel", "Widgets can only be added to guild channels")
		return
	}

	// Check if user has permission to add widgets.
	if !h.hasWidgetPermission(r, *guildID, userID, "add") {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "You do not have permission to add widgets")
		return
	}

	// Get the next position.
	var maxPos int
	h.Pool.QueryRow(r.Context(),
		`SELECT COALESCE(MAX(position), -1) FROM channel_widgets WHERE channel_id = $1`,
		channelID,
	).Scan(&maxPos)

	widgetID := models.NewULID().String()

	var cw ChannelWidget
	err = h.Pool.QueryRow(r.Context(),
		`INSERT INTO channel_widgets (id, channel_id, guild_id, widget_type, title, config, creator_id, position, active, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, true, NOW(), NOW())
		 RETURNING id, channel_id, guild_id, widget_type, title, config, creator_id, position, active, created_at, updated_at`,
		widgetID, channelID, *guildID, req.WidgetType, req.Title, req.Config, userID, maxPos+1,
	).Scan(&cw.ID, &cw.ChannelID, &cw.GuildID, &cw.WidgetType, &cw.Title, &cw.Config,
		&cw.CreatorID, &cw.Position, &cw.Active, &cw.CreatedAt, &cw.UpdatedAt)

	if err != nil {
		h.Logger.Error("failed to create channel widget", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create widget")
		return
	}

	h.EventBus.PublishGuildEvent(r.Context(), events.SubjectChannelUpdate, "CHANNEL_WIDGET_CREATE", cw.GuildID, cw)

	apiutil.WriteJSON(w, http.StatusCreated, cw)
}

// HandleUpdateChannelWidget updates a channel widget.
// PATCH /api/v1/channels/{channelID}/widgets/{widgetID}
func (h *Handler) HandleUpdateChannelWidget(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	widgetID := chi.URLParam(r, "widgetID")

	var req updateChannelWidgetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	// Get the widget to check permissions.
	var guildID, creatorID string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT guild_id, creator_id FROM channel_widgets WHERE id = $1`, widgetID,
	).Scan(&guildID, &creatorID)
	if err == pgx.ErrNoRows {
		apiutil.WriteError(w, http.StatusNotFound, "widget_not_found", "Widget not found")
		return
	}
	if err != nil {
		h.Logger.Error("failed to get widget", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get widget")
		return
	}

	// Creator or guild admin/owner can update.
	if creatorID != userID && !h.hasWidgetPermission(r, guildID, userID, "configure") {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "You do not have permission to update this widget")
		return
	}

	var cw ChannelWidget
	err = h.Pool.QueryRow(r.Context(),
		`UPDATE channel_widgets SET
		   title = COALESCE($2, title),
		   config = COALESCE($3, config),
		   position = COALESCE($4, position),
		   active = COALESCE($5, active),
		   updated_at = NOW()
		 WHERE id = $1
		 RETURNING id, channel_id, guild_id, widget_type, title, config, creator_id, position, active, created_at, updated_at`,
		widgetID, req.Title, req.Config, req.Position, req.Active,
	).Scan(&cw.ID, &cw.ChannelID, &cw.GuildID, &cw.WidgetType, &cw.Title, &cw.Config,
		&cw.CreatorID, &cw.Position, &cw.Active, &cw.CreatedAt, &cw.UpdatedAt)

	if err != nil {
		h.Logger.Error("failed to update widget", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update widget")
		return
	}

	h.EventBus.PublishGuildEvent(r.Context(), events.SubjectChannelUpdate, "CHANNEL_WIDGET_UPDATE", cw.GuildID, cw)

	apiutil.WriteJSON(w, http.StatusOK, cw)
}

// HandleDeleteChannelWidget removes a channel widget.
// DELETE /api/v1/channels/{channelID}/widgets/{widgetID}
func (h *Handler) HandleDeleteChannelWidget(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	widgetID := chi.URLParam(r, "widgetID")

	// Get widget details for permission check.
	var guildID, creatorID string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT guild_id, creator_id FROM channel_widgets WHERE id = $1`, widgetID,
	).Scan(&guildID, &creatorID)
	if err == pgx.ErrNoRows {
		apiutil.WriteError(w, http.StatusNotFound, "widget_not_found", "Widget not found")
		return
	}
	if err != nil {
		h.Logger.Error("failed to get widget", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get widget")
		return
	}

	// Creator or guild admin/owner can delete.
	if creatorID != userID && !h.hasWidgetPermission(r, guildID, userID, "remove") {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "You do not have permission to remove this widget")
		return
	}

	tag, err := h.Pool.Exec(r.Context(), `DELETE FROM channel_widgets WHERE id = $1`, widgetID)
	if err != nil {
		h.Logger.Error("failed to delete widget", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete widget")
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "widget_not_found", "Widget not found")
		return
	}

	h.EventBus.PublishGuildEvent(r.Context(), events.SubjectChannelUpdate, "CHANNEL_WIDGET_DELETE", guildID, map[string]string{
		"widget_id": widgetID,
		"guild_id":  guildID,
	})

	w.WriteHeader(http.StatusNoContent)
}

// ============================================================
// Plugin Marketplace Endpoints
// ============================================================

// HandleListPlugins returns available plugins from the marketplace.
// GET /api/v1/plugins
func (h *Handler) HandleListPlugins(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	search := r.URL.Query().Get("q")
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
		limit = l
	}

	query := `SELECT id, name, description, author, version, homepage_url, icon_url,
	                 category, public, verified, install_count, created_at, updated_at
	          FROM plugins WHERE public = true`
	args := make([]interface{}, 0)
	argIdx := 1

	if category != "" {
		query += ` AND category = $` + strconv.Itoa(argIdx)
		args = append(args, category)
		argIdx++
	}
	if search != "" {
		query += ` AND (name ILIKE $` + strconv.Itoa(argIdx) + ` OR description ILIKE $` + strconv.Itoa(argIdx) + `)`
		args = append(args, "%"+search+"%")
		argIdx++
	}

	query += ` ORDER BY install_count DESC, created_at DESC LIMIT $` + strconv.Itoa(argIdx)
	args = append(args, limit)

	rows, err := h.Pool.Query(r.Context(), query, args...)
	if err != nil {
		h.Logger.Error("failed to list plugins", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list plugins")
		return
	}
	defer rows.Close()

	type PluginListing struct {
		ID           string    `json:"id"`
		Name         string    `json:"name"`
		Description  *string   `json:"description,omitempty"`
		Author       string    `json:"author"`
		Version      string    `json:"version"`
		HomepageURL  *string   `json:"homepage_url,omitempty"`
		IconURL      *string   `json:"icon_url,omitempty"`
		Category     string    `json:"category"`
		Public       bool      `json:"public"`
		Verified     bool      `json:"verified"`
		InstallCount int       `json:"install_count"`
		CreatedAt    time.Time `json:"created_at"`
		UpdatedAt    time.Time `json:"updated_at"`
	}

	plugins := make([]PluginListing, 0)
	for rows.Next() {
		var p PluginListing
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Author, &p.Version,
			&p.HomepageURL, &p.IconURL, &p.Category, &p.Public, &p.Verified,
			&p.InstallCount, &p.CreatedAt, &p.UpdatedAt); err != nil {
			h.Logger.Error("failed to scan plugin", slog.String("error", err.Error()))
			continue
		}
		plugins = append(plugins, p)
	}

	apiutil.WriteJSON(w, http.StatusOK, plugins)
}

// HandleGetPlugin returns a single plugin's details.
// GET /api/v1/plugins/{pluginID}
func (h *Handler) HandleGetPlugin(w http.ResponseWriter, r *http.Request) {
	pluginID := chi.URLParam(r, "pluginID")

	type PluginDetail struct {
		ID           string          `json:"id"`
		Name         string          `json:"name"`
		Description  *string         `json:"description,omitempty"`
		Author       string          `json:"author"`
		Version      string          `json:"version"`
		HomepageURL  *string         `json:"homepage_url,omitempty"`
		IconURL      *string         `json:"icon_url,omitempty"`
		Manifest     json.RawMessage `json:"manifest"`
		Category     string          `json:"category"`
		Public       bool            `json:"public"`
		Verified     bool            `json:"verified"`
		InstallCount int             `json:"install_count"`
		CreatedAt    time.Time       `json:"created_at"`
		UpdatedAt    time.Time       `json:"updated_at"`
	}

	var p PluginDetail
	err := h.Pool.QueryRow(r.Context(),
		`SELECT id, name, description, author, version, homepage_url, icon_url,
		        manifest, category, public, verified, install_count, created_at, updated_at
		 FROM plugins WHERE id = $1`, pluginID,
	).Scan(&p.ID, &p.Name, &p.Description, &p.Author, &p.Version,
		&p.HomepageURL, &p.IconURL, &p.Manifest, &p.Category,
		&p.Public, &p.Verified, &p.InstallCount, &p.CreatedAt, &p.UpdatedAt)

	if err == pgx.ErrNoRows {
		apiutil.WriteError(w, http.StatusNotFound, "plugin_not_found", "Plugin not found")
		return
	}
	if err != nil {
		h.Logger.Error("failed to get plugin", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get plugin")
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, p)
}

// HandleInstallPlugin installs a plugin in a guild.
// POST /api/v1/guilds/{guildID}/plugins
func (h *Handler) HandleInstallPlugin(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isGuildAdmin(r, guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Only guild owners or admins can install plugins")
		return
	}

	var req struct {
		PluginID string          `json:"plugin_id"`
		Config   json.RawMessage `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if req.PluginID == "" {
		apiutil.WriteError(w, http.StatusBadRequest, "missing_plugin_id", "plugin_id is required")
		return
	}
	if req.Config == nil {
		req.Config = json.RawMessage(`{}`)
	}

	// Verify plugin exists.
	var exists bool
	err := h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM plugins WHERE id = $1 AND public = true)`, req.PluginID,
	).Scan(&exists)
	if err != nil || !exists {
		apiutil.WriteError(w, http.StatusNotFound, "plugin_not_found", "Plugin not found")
		return
	}

	installID := models.NewULID().String()

	type InstalledPlugin struct {
		ID          string          `json:"id"`
		GuildID     string          `json:"guild_id"`
		PluginID    string          `json:"plugin_id"`
		Enabled     bool            `json:"enabled"`
		Config      json.RawMessage `json:"config"`
		InstalledBy string          `json:"installed_by"`
		InstalledAt time.Time       `json:"installed_at"`
	}

	var ip InstalledPlugin
	err = h.Pool.QueryRow(r.Context(),
		`INSERT INTO guild_plugins (id, guild_id, plugin_id, enabled, config, installed_by, installed_at, updated_at)
		 VALUES ($1, $2, $3, true, $4, $5, NOW(), NOW())
		 ON CONFLICT (guild_id, plugin_id) DO UPDATE SET
		   enabled = true, config = EXCLUDED.config, updated_at = NOW()
		 RETURNING id, guild_id, plugin_id, enabled, config, installed_by, installed_at`,
		installID, guildID, req.PluginID, req.Config, userID,
	).Scan(&ip.ID, &ip.GuildID, &ip.PluginID, &ip.Enabled, &ip.Config, &ip.InstalledBy, &ip.InstalledAt)

	if err != nil {
		h.Logger.Error("failed to install plugin", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to install plugin")
		return
	}

	// Increment install count.
	h.Pool.Exec(r.Context(),
		`UPDATE plugins SET install_count = install_count + 1 WHERE id = $1`, req.PluginID)

	apiutil.WriteJSON(w, http.StatusCreated, ip)
}

// HandleGetGuildPlugins returns installed plugins for a guild.
// GET /api/v1/guilds/{guildID}/plugins
func (h *Handler) HandleGetGuildPlugins(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")

	rows, err := h.Pool.Query(r.Context(),
		`SELECT gp.id, gp.guild_id, gp.plugin_id, gp.enabled, gp.config, gp.installed_by,
		        gp.installed_at, gp.updated_at,
		        p.name, p.description, p.author, p.version, p.icon_url, p.category
		 FROM guild_plugins gp
		 JOIN plugins p ON p.id = gp.plugin_id
		 WHERE gp.guild_id = $1
		 ORDER BY gp.installed_at DESC`, guildID,
	)
	if err != nil {
		h.Logger.Error("failed to get guild plugins", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get guild plugins")
		return
	}
	defer rows.Close()

	type GuildPluginEntry struct {
		ID          string          `json:"id"`
		GuildID     string          `json:"guild_id"`
		PluginID    string          `json:"plugin_id"`
		Enabled     bool            `json:"enabled"`
		Config      json.RawMessage `json:"config"`
		InstalledBy string          `json:"installed_by"`
		InstalledAt time.Time       `json:"installed_at"`
		UpdatedAt   time.Time       `json:"updated_at"`
		Name        string          `json:"name"`
		Description *string         `json:"description,omitempty"`
		Author      string          `json:"author"`
		Version     string          `json:"version"`
		IconURL     *string         `json:"icon_url,omitempty"`
		Category    string          `json:"category"`
	}

	plugins := make([]GuildPluginEntry, 0)
	for rows.Next() {
		var gp GuildPluginEntry
		if err := rows.Scan(&gp.ID, &gp.GuildID, &gp.PluginID, &gp.Enabled, &gp.Config,
			&gp.InstalledBy, &gp.InstalledAt, &gp.UpdatedAt,
			&gp.Name, &gp.Description, &gp.Author, &gp.Version, &gp.IconURL, &gp.Category); err != nil {
			h.Logger.Error("failed to scan guild plugin", slog.String("error", err.Error()))
			continue
		}
		plugins = append(plugins, gp)
	}

	apiutil.WriteJSON(w, http.StatusOK, plugins)
}

// HandleUpdateGuildPlugin updates a guild plugin's config or enabled state.
// PATCH /api/v1/guilds/{guildID}/plugins/{installID}
func (h *Handler) HandleUpdateGuildPlugin(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	installID := chi.URLParam(r, "installID")

	if !h.isGuildAdmin(r, guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Only guild owners or admins can manage plugins")
		return
	}

	var req struct {
		Enabled *bool            `json:"enabled"`
		Config  *json.RawMessage `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`UPDATE guild_plugins SET
		   enabled = COALESCE($3, enabled),
		   config = COALESCE($4, config),
		   updated_at = NOW()
		 WHERE id = $1 AND guild_id = $2`,
		installID, guildID, req.Enabled, req.Config,
	)
	if err != nil {
		h.Logger.Error("failed to update guild plugin", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update plugin")
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "plugin_not_found", "Plugin installation not found")
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// HandleUninstallPlugin removes a plugin from a guild.
// DELETE /api/v1/guilds/{guildID}/plugins/{installID}
func (h *Handler) HandleUninstallPlugin(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	installID := chi.URLParam(r, "installID")

	if !h.isGuildAdmin(r, guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Only guild owners or admins can uninstall plugins")
		return
	}

	// Get plugin_id for decrementing install count.
	var pluginID string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT plugin_id FROM guild_plugins WHERE id = $1 AND guild_id = $2`,
		installID, guildID,
	).Scan(&pluginID)
	if err == pgx.ErrNoRows {
		apiutil.WriteError(w, http.StatusNotFound, "plugin_not_found", "Plugin installation not found")
		return
	}
	if err != nil {
		h.Logger.Error("failed to get guild plugin", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to uninstall plugin")
		return
	}

	_, err = h.Pool.Exec(r.Context(),
		`DELETE FROM guild_plugins WHERE id = $1 AND guild_id = $2`, installID, guildID)
	if err != nil {
		h.Logger.Error("failed to uninstall plugin", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to uninstall plugin")
		return
	}

	// Decrement install count.
	h.Pool.Exec(r.Context(),
		`UPDATE plugins SET install_count = GREATEST(install_count - 1, 0) WHERE id = $1`, pluginID)

	w.WriteHeader(http.StatusNoContent)
}

// ============================================================
// Key Backup Endpoints
// ============================================================

// HandleCreateKeyBackup creates or replaces a key backup for the current user.
// PUT /api/v1/encryption/key-backup
func (h *Handler) HandleCreateKeyBackup(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var req struct {
		EncryptedData []byte `json:"encrypted_data"`
		Salt          []byte `json:"salt"`
		Nonce         []byte `json:"nonce"`
		KeyCount      int    `json:"key_count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if len(req.EncryptedData) == 0 {
		apiutil.WriteError(w, http.StatusBadRequest, "missing_data", "encrypted_data is required")
		return
	}
	if len(req.Salt) < 16 {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_salt", "Salt must be at least 16 bytes")
		return
	}
	if len(req.Nonce) < 12 {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_nonce", "Nonce must be at least 12 bytes")
		return
	}

	backupID := models.NewULID().String()

	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO key_backups (id, user_id, encrypted_data, salt, nonce, key_count, version, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, 1, NOW(), NOW())
		 ON CONFLICT (user_id) DO UPDATE SET
		   id = EXCLUDED.id,
		   encrypted_data = EXCLUDED.encrypted_data,
		   salt = EXCLUDED.salt,
		   nonce = EXCLUDED.nonce,
		   key_count = EXCLUDED.key_count,
		   version = key_backups.version + 1,
		   updated_at = NOW()`,
		backupID, userID, req.EncryptedData, req.Salt, req.Nonce, req.KeyCount,
	)
	if err != nil {
		h.Logger.Error("failed to create key backup", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create key backup")
		return
	}

	apiutil.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"id":        backupID,
		"key_count": req.KeyCount,
		"version":   1,
	})
}

// HandleGetKeyBackup retrieves the key backup metadata for the current user.
// GET /api/v1/encryption/key-backup
func (h *Handler) HandleGetKeyBackup(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	type KeyBackupMeta struct {
		ID        string    `json:"id"`
		KeyCount  int       `json:"key_count"`
		Version   int       `json:"version"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}

	var meta KeyBackupMeta
	err := h.Pool.QueryRow(r.Context(),
		`SELECT id, key_count, version, created_at, updated_at
		 FROM key_backups WHERE user_id = $1`, userID,
	).Scan(&meta.ID, &meta.KeyCount, &meta.Version, &meta.CreatedAt, &meta.UpdatedAt)

	if err == pgx.ErrNoRows {
		apiutil.WriteJSON(w, http.StatusOK, map[string]interface{}{"exists": false})
		return
	}
	if err != nil {
		h.Logger.Error("failed to get key backup", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get key backup")
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"exists": true,
		"backup": meta,
	})
}

// HandleDownloadKeyBackup returns the encrypted key backup data for recovery.
// POST /api/v1/encryption/key-backup/download
func (h *Handler) HandleDownloadKeyBackup(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	type KeyBackupData struct {
		ID            string    `json:"id"`
		EncryptedData []byte    `json:"encrypted_data"`
		Salt          []byte    `json:"salt"`
		Nonce         []byte    `json:"nonce"`
		KeyCount      int       `json:"key_count"`
		Version       int       `json:"version"`
		CreatedAt     time.Time `json:"created_at"`
	}

	var backup KeyBackupData
	err := h.Pool.QueryRow(r.Context(),
		`SELECT id, encrypted_data, salt, nonce, key_count, version, created_at
		 FROM key_backups WHERE user_id = $1`, userID,
	).Scan(&backup.ID, &backup.EncryptedData, &backup.Salt, &backup.Nonce,
		&backup.KeyCount, &backup.Version, &backup.CreatedAt)

	if err == pgx.ErrNoRows {
		apiutil.WriteError(w, http.StatusNotFound, "no_backup", "No key backup found")
		return
	}
	if err != nil {
		h.Logger.Error("failed to download key backup", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to download key backup")
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, backup)
}

// HandleDeleteKeyBackup deletes the key backup for the current user.
// DELETE /api/v1/encryption/key-backup
func (h *Handler) HandleDeleteKeyBackup(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM key_backups WHERE user_id = $1`, userID)
	if err != nil {
		h.Logger.Error("failed to delete key backup", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete key backup")
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "no_backup", "No key backup found")
		return
	}

	// Also delete recovery codes.
	h.Pool.Exec(r.Context(),
		`DELETE FROM key_backup_recovery_codes WHERE user_id = $1`, userID)

	w.WriteHeader(http.StatusNoContent)
}

// HandleGenerateRecoveryCodes generates new recovery codes for key backup.
// POST /api/v1/encryption/key-backup/recovery-codes
func (h *Handler) HandleGenerateRecoveryCodes(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	// Verify user has a backup.
	var exists bool
	err := h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM key_backups WHERE user_id = $1)`, userID,
	).Scan(&exists)
	if err != nil || !exists {
		apiutil.WriteError(w, http.StatusBadRequest, "no_backup", "Create a key backup first")
		return
	}

	// Delete old codes.
	h.Pool.Exec(r.Context(),
		`DELETE FROM key_backup_recovery_codes WHERE user_id = $1`, userID)

	// Generate 8 recovery codes.
	codes := make([]string, 8)
	for i := range codes {
		codes[i] = generateRecoveryCode()
		codeID := models.NewULID().String()
		hash := hashRecoveryCode(codes[i])
		h.Pool.Exec(r.Context(),
			`INSERT INTO key_backup_recovery_codes (id, user_id, code_hash, used, created_at)
			 VALUES ($1, $2, $3, false, NOW())`,
			codeID, userID, hash,
		)
	}

	apiutil.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"codes": codes,
	})
}

// ============================================================
// Helpers
// ============================================================

// isGuildAdmin checks if the user is the guild owner or an instance admin.
func (h *Handler) isGuildAdmin(r *http.Request, guildID, userID string) bool {
	var ownerID string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT owner_id FROM guilds WHERE id = $1`, guildID,
	).Scan(&ownerID)
	if err != nil {
		return false
	}
	if ownerID == userID {
		return true
	}

	var flags int
	err = h.Pool.QueryRow(r.Context(),
		`SELECT flags FROM users WHERE id = $1`, userID,
	).Scan(&flags)
	if err != nil {
		return false
	}
	return flags&models.UserFlagAdmin != 0
}

// hasWidgetPermission checks if a user has a specific widget permission.
func (h *Handler) hasWidgetPermission(r *http.Request, guildID, userID, action string) bool {
	if h.isGuildAdmin(r, guildID, userID) {
		return true
	}

	var canAdd, canRemove, canConfigure bool
	err := h.Pool.QueryRow(r.Context(),
		`SELECT can_add, can_remove, can_configure
		 FROM widget_permissions
		 WHERE guild_id = $1 AND user_id = $2`, guildID, userID,
	).Scan(&canAdd, &canRemove, &canConfigure)
	if err == nil {
		switch action {
		case "add":
			return canAdd
		case "remove":
			return canRemove
		case "configure":
			return canConfigure
		}
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT wp.can_add, wp.can_remove, wp.can_configure
		 FROM widget_permissions wp
		 JOIN member_roles mr ON mr.role_id = wp.role_id
		 WHERE wp.guild_id = $1 AND mr.user_id = $2 AND mr.guild_id = $1`,
		guildID, userID,
	)
	if err != nil {
		return false
	}
	defer rows.Close()

	for rows.Next() {
		if err := rows.Scan(&canAdd, &canRemove, &canConfigure); err == nil {
			switch action {
			case "add":
				if canAdd {
					return true
				}
			case "remove":
				if canRemove {
					return true
				}
			case "configure":
				if canConfigure {
					return true
				}
			}
		}
	}

	return false
}

// generateRecoveryCode creates a random recovery code in XXXX-XXXX-XXXX-XXXX format.
func generateRecoveryCode() string {
	const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 16)
	randBytes := make([]byte, 16)
	_, _ = rand.Read(randBytes)
	for i := range b {
		b[i] = chars[int(randBytes[i])%len(chars)]
	}
	return string(b[:4]) + "-" + string(b[4:8]) + "-" + string(b[8:12]) + "-" + string(b[12:16])
}

// hashRecoveryCode returns the SHA-256 hex digest of a recovery code.
func hashRecoveryCode(code string) string {
	h := sha256.Sum256([]byte(code))
	return hex.EncodeToString(h[:])
}


