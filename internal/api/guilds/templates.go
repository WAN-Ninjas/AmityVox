// Package guilds — guild template export/import handlers. Templates capture a
// snapshot of a guild's structure (roles, channels, categories, permissions,
// settings) that can be saved, shared, and applied to create new guilds or
// restructure existing ones.
package guilds

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/models"
	"github.com/amityvox/amityvox/internal/permissions"
)

// --- Template data types ---

// guildTemplate represents a saved guild structure template.
type guildTemplate struct {
	ID           string          `json:"id"`
	GuildID      string          `json:"guild_id"`
	Name         string          `json:"name"`
	Description  *string         `json:"description,omitempty"`
	TemplateData json.RawMessage `json:"template_data"`
	CreatorID    string          `json:"creator_id"`
	CreatedAt    time.Time       `json:"created_at"`
}

// templateData is the structured content stored in template_data JSONB.
type templateData struct {
	GuildSettings templateGuildSettings `json:"guild_settings"`
	Roles         []templateRole        `json:"roles"`
	Categories    []templateCategory    `json:"categories"`
	Channels      []templateChannel     `json:"channels"`
}

type templateGuildSettings struct {
	Name               string  `json:"name"`
	Description        *string `json:"description,omitempty"`
	DefaultPermissions int64   `json:"default_permissions"`
	NSFW               bool    `json:"nsfw"`
	VerificationLevel  int     `json:"verification_level"`
	AFKTimeout         int     `json:"afk_timeout"`
}

type templateRole struct {
	Name             string  `json:"name"`
	Color            *string `json:"color,omitempty"`
	Hoist            bool    `json:"hoist"`
	Mentionable      bool    `json:"mentionable"`
	Position         int     `json:"position"`
	PermissionsAllow int64   `json:"permissions_allow"`
	PermissionsDeny  int64   `json:"permissions_deny"`
}

type templateCategory struct {
	Name     string `json:"name"`
	Position int    `json:"position"`
}

type templateChannel struct {
	Name            string `json:"name"`
	ChannelType     string `json:"channel_type"`
	CategoryName    string `json:"category_name,omitempty"` // Linked by name rather than ID.
	Topic           string `json:"topic,omitempty"`
	Position        int    `json:"position"`
	NSFW            bool   `json:"nsfw"`
	SlowmodeSeconds int    `json:"slowmode_seconds"`
	UserLimit       int    `json:"user_limit,omitempty"`
	Bitrate         int    `json:"bitrate,omitempty"`
}

// --- Request types ---

type createTemplateRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

type applyTemplateRequest struct {
	TemplateID string  `json:"template_id"`
	GuildName  *string `json:"guild_name"` // Used when creating a new guild from template.
}

// --- Handlers ---

// HandleCreateGuildTemplate captures the current guild's structure as a template.
// Requires MANAGE_GUILD permission. Creates a snapshot of roles, channels,
// categories, permissions, and settings.
// POST /api/v1/guilds/{guildID}/templates
func (h *Handler) HandleCreateGuildTemplate(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageGuild) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_GUILD permission")
		return
	}

	var req createTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Name == "" || len(req.Name) > 100 {
		writeError(w, http.StatusBadRequest, "invalid_name", "Template name must be 1-100 characters")
		return
	}
	if req.Description != nil && len(*req.Description) > 500 {
		writeError(w, http.StatusBadRequest, "invalid_description", "Description must be at most 500 characters")
		return
	}

	// Limit templates per guild.
	var templateCount int
	h.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM guild_templates WHERE guild_id = $1`, guildID,
	).Scan(&templateCount)
	if templateCount >= 10 {
		writeError(w, http.StatusBadRequest, "template_limit", "A guild can have at most 10 templates")
		return
	}

	ctx := r.Context()

	// Capture the guild's current structure.
	data, err := h.captureGuildStructure(ctx, guildID)
	if err != nil {
		h.Logger.Error("failed to capture guild structure", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create template")
		return
	}

	dataJSON, err := json.Marshal(data)
	if err != nil {
		h.Logger.Error("failed to marshal template data", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create template")
		return
	}

	templateID := models.NewULID().String()
	now := time.Now()

	_, err = h.Pool.Exec(ctx,
		`INSERT INTO guild_templates (id, guild_id, name, description, template_data, creator_id, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		templateID, guildID, req.Name, req.Description, dataJSON, userID, now,
	)
	if err != nil {
		h.Logger.Error("failed to insert template", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create template")
		return
	}

	tmpl := guildTemplate{
		ID:           templateID,
		GuildID:      guildID,
		Name:         req.Name,
		Description:  req.Description,
		TemplateData: dataJSON,
		CreatorID:    userID,
		CreatedAt:    now,
	}

	h.logAudit(ctx, guildID, userID, "template_create", "template", templateID, nil)

	writeJSON(w, http.StatusCreated, tmpl)
}

// HandleGetGuildTemplates lists all templates for a guild.
// Requires guild membership.
// GET /api/v1/guilds/{guildID}/templates
func (h *Handler) HandleGetGuildTemplates(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isMember(r.Context(), guildID, userID) {
		writeError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, guild_id, name, description, template_data, creator_id, created_at
		 FROM guild_templates
		 WHERE guild_id = $1
		 ORDER BY created_at DESC`,
		guildID,
	)
	if err != nil {
		h.Logger.Error("failed to get templates", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get templates")
		return
	}
	defer rows.Close()

	templates := make([]guildTemplate, 0)
	for rows.Next() {
		var t guildTemplate
		if err := rows.Scan(&t.ID, &t.GuildID, &t.Name, &t.Description, &t.TemplateData,
			&t.CreatorID, &t.CreatedAt); err != nil {
			h.Logger.Error("failed to scan template", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read templates")
			return
		}
		templates = append(templates, t)
	}

	writeJSON(w, http.StatusOK, templates)
}

// HandleGetGuildTemplate returns a single template by ID.
// GET /api/v1/guilds/{guildID}/templates/{templateID}
func (h *Handler) HandleGetGuildTemplate(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	templateID := chi.URLParam(r, "templateID")

	if !h.isMember(r.Context(), guildID, userID) {
		writeError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	var t guildTemplate
	err := h.Pool.QueryRow(r.Context(),
		`SELECT id, guild_id, name, description, template_data, creator_id, created_at
		 FROM guild_templates
		 WHERE id = $1 AND guild_id = $2`,
		templateID, guildID,
	).Scan(&t.ID, &t.GuildID, &t.Name, &t.Description, &t.TemplateData, &t.CreatorID, &t.CreatedAt)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "template_not_found", "Template not found")
		return
	}
	if err != nil {
		h.Logger.Error("failed to get template", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get template")
		return
	}

	writeJSON(w, http.StatusOK, t)
}

// HandleDeleteGuildTemplate deletes a guild template.
// Requires MANAGE_GUILD permission.
// DELETE /api/v1/guilds/{guildID}/templates/{templateID}
func (h *Handler) HandleDeleteGuildTemplate(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	templateID := chi.URLParam(r, "templateID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageGuild) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_GUILD permission")
		return
	}

	result, err := h.Pool.Exec(r.Context(),
		`DELETE FROM guild_templates WHERE id = $1 AND guild_id = $2`,
		templateID, guildID,
	)
	if err != nil {
		h.Logger.Error("failed to delete template", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete template")
		return
	}
	if result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "template_not_found", "Template not found")
		return
	}

	h.logAudit(r.Context(), guildID, userID, "template_delete", "template", templateID, nil)

	w.WriteHeader(http.StatusNoContent)
}

// HandleApplyGuildTemplate creates a new guild from a template or applies a
// template's structure to an existing guild.
//
// If guild_name is provided: creates a new guild with that name and applies
// the template structure.
// If guild_name is omitted: applies the template to the guild that owns it
// (adds missing roles and channels without removing existing ones).
//
// POST /api/v1/guilds/{guildID}/templates/{templateID}/apply
func (h *Handler) HandleApplyGuildTemplate(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	sourceGuildID := chi.URLParam(r, "guildID")
	templateID := chi.URLParam(r, "templateID")

	// User must be a member of the source guild to access its templates.
	if !h.isMember(r.Context(), sourceGuildID, userID) {
		writeError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	var req applyTemplateRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}
	}

	ctx := r.Context()

	// Fetch the template.
	var tmplData json.RawMessage
	err := h.Pool.QueryRow(ctx,
		`SELECT template_data FROM guild_templates WHERE id = $1 AND guild_id = $2`,
		templateID, sourceGuildID,
	).Scan(&tmplData)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "template_not_found", "Template not found")
		return
	}
	if err != nil {
		h.Logger.Error("failed to get template for apply", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get template")
		return
	}

	var data templateData
	if err := json.Unmarshal(tmplData, &data); err != nil {
		h.Logger.Error("failed to unmarshal template data", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Invalid template data")
		return
	}

	if req.GuildName != nil && *req.GuildName != "" {
		// Create a new guild from the template.
		guild, err := h.createGuildFromTemplate(ctx, userID, *req.GuildName, data)
		if err != nil {
			h.Logger.Error("failed to create guild from template", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create guild from template")
			return
		}
		writeJSON(w, http.StatusCreated, guild)
	} else {
		// Apply template to the source guild (additive).
		if !h.hasGuildPermission(ctx, sourceGuildID, userID, permissions.ManageGuild) {
			writeError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_GUILD permission to apply templates")
			return
		}
		if err := h.applyTemplateToGuild(ctx, sourceGuildID, data); err != nil {
			h.Logger.Error("failed to apply template to guild", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to apply template")
			return
		}
		h.logAudit(ctx, sourceGuildID, userID, "template_apply", "template", templateID, nil)

		guild, err := h.getGuild(ctx, sourceGuildID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get guild")
			return
		}
		writeJSON(w, http.StatusOK, guild)
	}
}

// --- Internal helpers ---

// captureGuildStructure takes a snapshot of a guild's roles, channels,
// categories, and settings for template storage.
func (h *Handler) captureGuildStructure(ctx context.Context, guildID string) (templateData, error) {
	var data templateData

	// Capture guild settings.
	var gs templateGuildSettings
	err := h.Pool.QueryRow(ctx,
		`SELECT name, description, default_permissions, nsfw, verification_level, afk_timeout
		 FROM guilds WHERE id = $1`, guildID,
	).Scan(&gs.Name, &gs.Description, &gs.DefaultPermissions, &gs.NSFW, &gs.VerificationLevel, &gs.AFKTimeout)
	if err != nil {
		return data, err
	}
	data.GuildSettings = gs

	// Capture roles (excluding @everyone which is implicit).
	rRows, err := h.Pool.Query(ctx,
		`SELECT name, color, hoist, mentionable, position, permissions_allow, permissions_deny
		 FROM roles WHERE guild_id = $1
		 ORDER BY position ASC`,
		guildID,
	)
	if err != nil {
		return data, err
	}
	defer rRows.Close()

	data.Roles = make([]templateRole, 0)
	for rRows.Next() {
		var r templateRole
		if err := rRows.Scan(&r.Name, &r.Color, &r.Hoist, &r.Mentionable,
			&r.Position, &r.PermissionsAllow, &r.PermissionsDeny); err != nil {
			return data, err
		}
		data.Roles = append(data.Roles, r)
	}

	// Capture categories and build a map from category ID to name for channel linking.
	catIDToName := make(map[string]string)
	data.Categories = make([]templateCategory, 0)
	catRows, err := h.Pool.Query(ctx,
		`SELECT id, name, position FROM guild_categories WHERE guild_id = $1 ORDER BY position`,
		guildID,
	)
	if err != nil {
		return data, err
	}
	defer catRows.Close()
	for catRows.Next() {
		var id, name string
		var position int
		if err := catRows.Scan(&id, &name, &position); err != nil {
			return data, err
		}
		catIDToName[id] = name
		data.Categories = append(data.Categories, templateCategory{Name: name, Position: position})
	}

	// Capture channels.
	chRows, err := h.Pool.Query(ctx,
		`SELECT name, channel_type, category_id, topic, position, nsfw, slowmode_seconds,
		        user_limit, bitrate
		 FROM channels WHERE guild_id = $1
		 ORDER BY position`,
		guildID,
	)
	if err != nil {
		return data, err
	}
	defer chRows.Close()

	data.Channels = make([]templateChannel, 0)
	for chRows.Next() {
		var ch templateChannel
		var categoryID *string
		var name *string
		var topic *string
		if err := chRows.Scan(&name, &ch.ChannelType, &categoryID, &topic,
			&ch.Position, &ch.NSFW, &ch.SlowmodeSeconds, &ch.UserLimit, &ch.Bitrate); err != nil {
			return data, err
		}
		if name != nil {
			ch.Name = *name
		}
		if topic != nil {
			ch.Topic = *topic
		}
		if categoryID != nil {
			if catName, ok := catIDToName[*categoryID]; ok {
				ch.CategoryName = catName
			}
		}
		data.Channels = append(data.Channels, ch)
	}

	return data, nil
}

// createGuildFromTemplate creates a new guild and populates it using template data.
func (h *Handler) createGuildFromTemplate(ctx context.Context, userID, guildName string, data templateData) (*models.Guild, error) {
	guildID := models.NewULID().String()
	now := time.Now()

	defaultPerms := data.GuildSettings.DefaultPermissions
	if defaultPerms == 0 {
		defaultPerms = int64(permissions.ViewChannel | permissions.ReadHistory |
			permissions.SendMessages | permissions.AddReactions |
			permissions.Connect | permissions.Speak |
			permissions.ChangeNickname | permissions.CreateInvites)
	}

	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Create the guild.
	var guild models.Guild
	err = tx.QueryRow(ctx,
		`INSERT INTO guilds (id, instance_id, owner_id, name, description, default_permissions,
		                     nsfw, verification_level, afk_timeout, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 RETURNING id, instance_id, owner_id, name, description, icon_id, banner_id,
		           default_permissions, flags, nsfw, discoverable, preferred_locale, max_members,
		           verification_level, afk_channel_id, afk_timeout, created_at`,
		guildID, h.InstanceID, userID, guildName, data.GuildSettings.Description,
		defaultPerms, data.GuildSettings.NSFW, data.GuildSettings.VerificationLevel,
		data.GuildSettings.AFKTimeout, now,
	).Scan(
		&guild.ID, &guild.InstanceID, &guild.OwnerID, &guild.Name, &guild.Description,
		&guild.IconID, &guild.BannerID, &guild.DefaultPermissions, &guild.Flags,
		&guild.NSFW, &guild.Discoverable, &guild.PreferredLocale, &guild.MaxMembers,
		&guild.VerificationLevel, &guild.AFKChannelID, &guild.AFKTimeout, &guild.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Add the creating user as the owner member.
	_, err = tx.Exec(ctx,
		`INSERT INTO guild_members (guild_id, user_id, joined_at) VALUES ($1, $2, $3)`,
		guildID, userID, now,
	)
	if err != nil {
		return nil, err
	}

	// Create roles from template.
	for _, role := range data.Roles {
		roleID := models.NewULID().String()
		_, err = tx.Exec(ctx,
			`INSERT INTO roles (id, guild_id, name, color, hoist, mentionable, position,
			                    permissions_allow, permissions_deny, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			roleID, guildID, role.Name, role.Color, role.Hoist, role.Mentionable,
			role.Position, role.PermissionsAllow, role.PermissionsDeny, now,
		)
		if err != nil {
			return nil, err
		}
	}

	// Create categories from template and build a name-to-ID map.
	catNameToID := make(map[string]string)
	for _, cat := range data.Categories {
		catID := models.NewULID().String()
		_, err = tx.Exec(ctx,
			`INSERT INTO guild_categories (id, guild_id, name, position, created_at)
			 VALUES ($1, $2, $3, $4, $5)`,
			catID, guildID, cat.Name, cat.Position, now,
		)
		if err != nil {
			return nil, err
		}
		catNameToID[cat.Name] = catID
	}

	// Create channels from template.
	hasGeneral := false
	for _, ch := range data.Channels {
		channelID := models.NewULID().String()
		var categoryID *string
		if ch.CategoryName != "" {
			if id, ok := catNameToID[ch.CategoryName]; ok {
				categoryID = &id
			}
		}
		var topicPtr *string
		if ch.Topic != "" {
			topicPtr = &ch.Topic
		}
		var namePtr *string
		if ch.Name != "" {
			namePtr = &ch.Name
			if ch.Name == "general" && ch.ChannelType == "text" {
				hasGeneral = true
			}
		}
		_, err = tx.Exec(ctx,
			`INSERT INTO channels (id, guild_id, category_id, channel_type, name, topic, position,
			                       nsfw, slowmode_seconds, user_limit, bitrate, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			channelID, guildID, categoryID, ch.ChannelType, namePtr, topicPtr,
			ch.Position, ch.NSFW, ch.SlowmodeSeconds, ch.UserLimit, ch.Bitrate, now,
		)
		if err != nil {
			return nil, err
		}
	}

	// Ensure at least one text channel exists.
	if !hasGeneral && len(data.Channels) == 0 {
		channelID := models.NewULID().String()
		_, err = tx.Exec(ctx,
			`INSERT INTO channels (id, guild_id, channel_type, name, position, created_at)
			 VALUES ($1, $2, 'text', 'general', 0, $3)`,
			channelID, guildID, now,
		)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &guild, nil
}

// applyTemplateToGuild applies a template's structure to an existing guild.
// This is additive: it creates roles and channels that don't already exist
// by name, but does not remove existing ones.
func (h *Handler) applyTemplateToGuild(ctx context.Context, guildID string, data templateData) error {
	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	now := time.Now()

	// Get existing role names to avoid duplicates.
	existingRoles := make(map[string]bool)
	rRows, err := tx.Query(ctx, `SELECT name FROM roles WHERE guild_id = $1`, guildID)
	if err != nil {
		return err
	}
	defer rRows.Close()
	for rRows.Next() {
		var name string
		rRows.Scan(&name)
		existingRoles[name] = true
	}

	// Create missing roles.
	for _, role := range data.Roles {
		if existingRoles[role.Name] {
			continue
		}
		roleID := models.NewULID().String()
		_, err = tx.Exec(ctx,
			`INSERT INTO roles (id, guild_id, name, color, hoist, mentionable, position,
			                    permissions_allow, permissions_deny, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			roleID, guildID, role.Name, role.Color, role.Hoist, role.Mentionable,
			role.Position, role.PermissionsAllow, role.PermissionsDeny, now,
		)
		if err != nil {
			return err
		}
	}

	// Get existing category names.
	existingCats := make(map[string]string) // name -> id
	cRows, err := tx.Query(ctx, `SELECT id, name FROM guild_categories WHERE guild_id = $1`, guildID)
	if err != nil {
		return err
	}
	defer cRows.Close()
	for cRows.Next() {
		var id, name string
		cRows.Scan(&id, &name)
		existingCats[name] = id
	}

	// Create missing categories.
	for _, cat := range data.Categories {
		if _, ok := existingCats[cat.Name]; ok {
			continue
		}
		catID := models.NewULID().String()
		_, err = tx.Exec(ctx,
			`INSERT INTO guild_categories (id, guild_id, name, position, created_at)
			 VALUES ($1, $2, $3, $4, $5)`,
			catID, guildID, cat.Name, cat.Position, now,
		)
		if err != nil {
			return err
		}
		existingCats[cat.Name] = catID
	}

	// Get existing channel names to avoid duplicates.
	existingChannels := make(map[string]bool)
	chRows, err := tx.Query(ctx, `SELECT name FROM channels WHERE guild_id = $1`, guildID)
	if err != nil {
		return err
	}
	defer chRows.Close()
	for chRows.Next() {
		var name *string
		chRows.Scan(&name)
		if name != nil {
			existingChannels[*name] = true
		}
	}

	// Create missing channels.
	for _, ch := range data.Channels {
		if existingChannels[ch.Name] {
			continue
		}
		channelID := models.NewULID().String()
		var categoryID *string
		if ch.CategoryName != "" {
			if id, ok := existingCats[ch.CategoryName]; ok {
				categoryID = &id
			}
		}
		var topicPtr *string
		if ch.Topic != "" {
			topicPtr = &ch.Topic
		}
		var namePtr *string
		if ch.Name != "" {
			namePtr = &ch.Name
		}
		_, err = tx.Exec(ctx,
			`INSERT INTO channels (id, guild_id, category_id, channel_type, name, topic, position,
			                       nsfw, slowmode_seconds, user_limit, bitrate, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			channelID, guildID, categoryID, ch.ChannelType, namePtr, topicPtr,
			ch.Position, ch.NSFW, ch.SlowmodeSeconds, ch.UserLimit, ch.Bitrate, now,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}
