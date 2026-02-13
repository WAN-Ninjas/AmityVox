// Package guilds implements REST API handlers for guild operations including
// creating, updating, and deleting guilds, managing members, roles, bans,
// invites, emoji, and the audit log. Mounted under /api/v1/guilds.
package guilds

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/models"
	"github.com/amityvox/amityvox/internal/permissions"
)

// Handler implements guild-related REST API endpoints.
type Handler struct {
	Pool       *pgxpool.Pool
	EventBus   *events.Bus
	InstanceID string
	Logger     *slog.Logger
}

type createGuildRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

type updateGuildRequest struct {
	Name              *string  `json:"name"`
	Description       *string  `json:"description"`
	IconID            *string  `json:"icon_id"`
	BannerID          *string  `json:"banner_id"`
	NSFW              *bool    `json:"nsfw"`
	Discoverable      *bool    `json:"discoverable"`
	VerificationLevel *int     `json:"verification_level"`
	AFKChannelID      *string  `json:"afk_channel_id"`
	AFKTimeout        *int     `json:"afk_timeout"`
	Tags              []string `json:"tags"`
}

type createChannelRequest struct {
	Name        string  `json:"name"`
	ChannelType string  `json:"channel_type"`
	CategoryID  *string `json:"category_id"`
	Topic       *string `json:"topic"`
	Position    *int    `json:"position"`
	NSFW        *bool   `json:"nsfw"`
}

type updateMemberRequest struct {
	Nickname     *string    `json:"nickname"`
	Deaf         *bool      `json:"deaf"`
	Mute         *bool      `json:"mute"`
	TimeoutUntil *time.Time `json:"timeout_until"`
	Roles        []string   `json:"roles"`
}

type createRoleRequest struct {
	Name             string  `json:"name"`
	Color            *string `json:"color"`
	Hoist            *bool   `json:"hoist"`
	Mentionable      *bool   `json:"mentionable"`
	Position         *int    `json:"position"`
	PermissionsAllow *int64  `json:"permissions_allow"`
	PermissionsDeny  *int64  `json:"permissions_deny"`
}

type updateRoleRequest struct {
	Name             *string `json:"name"`
	Color            *string `json:"color"`
	Hoist            *bool   `json:"hoist"`
	Mentionable      *bool   `json:"mentionable"`
	Position         *int    `json:"position"`
	PermissionsAllow *int64  `json:"permissions_allow"`
	PermissionsDeny  *int64  `json:"permissions_deny"`
}

type banRequest struct {
	Reason *string `json:"reason"`
}

// HandleCreateGuild creates a new guild owned by the authenticated user.
// POST /api/v1/guilds
func (h *Handler) HandleCreateGuild(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var req createGuildRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Name == "" || len(req.Name) > 100 {
		writeError(w, http.StatusBadRequest, "invalid_name", "Guild name must be 1-100 characters")
		return
	}

	guildID := models.NewULID().String()
	defaultPerms := int64(permissions.ViewChannel | permissions.ReadHistory |
		permissions.SendMessages | permissions.AddReactions |
		permissions.Connect | permissions.Speak |
		permissions.ChangeNickname | permissions.CreateInvites)

	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create guild")
		return
	}
	defer tx.Rollback(r.Context())

	// Create the guild.
	var guild models.Guild
	err = tx.QueryRow(r.Context(),
		`INSERT INTO guilds (id, instance_id, owner_id, name, description, default_permissions, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, now())
		 RETURNING id, instance_id, owner_id, name, description, icon_id, banner_id,
		           default_permissions, flags, nsfw, discoverable, preferred_locale, max_members,
		           verification_level, afk_channel_id, afk_timeout, created_at`,
		guildID, h.InstanceID, userID, req.Name, req.Description, defaultPerms,
	).Scan(
		&guild.ID, &guild.InstanceID, &guild.OwnerID, &guild.Name, &guild.Description,
		&guild.IconID, &guild.BannerID, &guild.DefaultPermissions, &guild.Flags,
		&guild.NSFW, &guild.Discoverable, &guild.PreferredLocale, &guild.MaxMembers,
		&guild.VerificationLevel, &guild.AFKChannelID, &guild.AFKTimeout, &guild.CreatedAt,
	)
	if err != nil {
		h.Logger.Error("failed to create guild", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create guild")
		return
	}

	// Add owner as member.
	_, err = tx.Exec(r.Context(),
		`INSERT INTO guild_members (guild_id, user_id, joined_at) VALUES ($1, $2, now())`,
		guildID, userID,
	)
	if err != nil {
		h.Logger.Error("failed to add owner as member", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create guild")
		return
	}

	// Create default "general" text channel.
	channelID := models.NewULID().String()
	_, err = tx.Exec(r.Context(),
		`INSERT INTO channels (id, guild_id, channel_type, name, position, created_at)
		 VALUES ($1, $2, 'text', 'general', 0, now())`,
		channelID, guildID,
	)
	if err != nil {
		h.Logger.Error("failed to create default channel", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create guild")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create guild")
		return
	}

	h.EventBus.PublishJSON(r.Context(), events.SubjectGuildCreate, "GUILD_CREATE", guild)

	writeJSON(w, http.StatusCreated, guild)
}

// HandleGetGuild returns a guild's details.
// GET /api/v1/guilds/{guildID}
func (h *Handler) HandleGetGuild(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isMember(r.Context(), guildID, userID) {
		writeError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	guild, err := h.getGuild(r.Context(), guildID)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "guild_not_found", "Guild not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get guild")
		return
	}

	writeJSON(w, http.StatusOK, guild)
}

// HandleUpdateGuild updates a guild's settings. Requires MANAGE_GUILD or owner.
// PATCH /api/v1/guilds/{guildID}
func (h *Handler) HandleUpdateGuild(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageGuild) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_GUILD permission")
		return
	}

	var req updateGuildRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	// If tags were provided, update them; otherwise keep existing.
	var tagsArg interface{} = nil
	if req.Tags != nil {
		tagsArg = req.Tags
	}

	var guild models.Guild
	err := h.Pool.QueryRow(r.Context(),
		`UPDATE guilds SET
			name = COALESCE($2, name),
			description = COALESCE($3, description),
			icon_id = COALESCE($4, icon_id),
			banner_id = COALESCE($5, banner_id),
			nsfw = COALESCE($6, nsfw),
			discoverable = COALESCE($7, discoverable),
			verification_level = COALESCE($8, verification_level),
			afk_channel_id = COALESCE($9, afk_channel_id),
			afk_timeout = COALESCE($10, afk_timeout),
			tags = COALESCE($11, tags)
		 WHERE id = $1
		 RETURNING id, instance_id, owner_id, name, description, icon_id, banner_id,
		           default_permissions, flags, nsfw, discoverable, preferred_locale, max_members,
		           vanity_url, verification_level, afk_channel_id, afk_timeout,
		           tags, member_count, created_at`,
		guildID, req.Name, req.Description, req.IconID, req.BannerID, req.NSFW, req.Discoverable, req.VerificationLevel, req.AFKChannelID, req.AFKTimeout, tagsArg,
	).Scan(
		&guild.ID, &guild.InstanceID, &guild.OwnerID, &guild.Name, &guild.Description,
		&guild.IconID, &guild.BannerID, &guild.DefaultPermissions, &guild.Flags,
		&guild.NSFW, &guild.Discoverable, &guild.PreferredLocale, &guild.MaxMembers,
		&guild.VanityURL, &guild.VerificationLevel, &guild.AFKChannelID, &guild.AFKTimeout,
		&guild.Tags, &guild.MemberCount, &guild.CreatedAt,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update guild")
		return
	}

	h.logAudit(r.Context(), guildID, userID, "guild_update", "guild", guildID, nil)
	h.EventBus.PublishJSON(r.Context(), events.SubjectGuildUpdate, "GUILD_UPDATE", guild)

	writeJSON(w, http.StatusOK, guild)
}

// HandleDeleteGuild deletes a guild. Only the owner can do this.
// DELETE /api/v1/guilds/{guildID}
func (h *Handler) HandleDeleteGuild(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	var ownerID string
	err := h.Pool.QueryRow(r.Context(), `SELECT owner_id FROM guilds WHERE id = $1`, guildID).Scan(&ownerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "guild_not_found", "Guild not found")
		return
	}
	if ownerID != userID {
		writeError(w, http.StatusForbidden, "not_owner", "Only the guild owner can delete the guild")
		return
	}

	_, err = h.Pool.Exec(r.Context(), `DELETE FROM guilds WHERE id = $1`, guildID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete guild")
		return
	}

	h.EventBus.PublishJSON(r.Context(), events.SubjectGuildDelete, "GUILD_DELETE", map[string]string{"id": guildID})

	w.WriteHeader(http.StatusNoContent)
}

// HandleLeaveGuild allows a member to leave a guild.
// POST /api/v1/guilds/{guildID}/leave
func (h *Handler) HandleLeaveGuild(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	// Check if user is the owner — owners cannot leave (must transfer or delete).
	var ownerID string
	if err := h.Pool.QueryRow(r.Context(), `SELECT owner_id FROM guilds WHERE id = $1`, guildID).Scan(&ownerID); err != nil {
		writeError(w, http.StatusNotFound, "guild_not_found", "Guild not found")
		return
	}
	if ownerID == userID {
		writeError(w, http.StatusBadRequest, "owner_cannot_leave", "Guild owner cannot leave. Transfer ownership or delete the guild.")
		return
	}

	result, err := h.Pool.Exec(r.Context(),
		`DELETE FROM guild_members WHERE guild_id = $1 AND user_id = $2`, guildID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to leave guild")
		return
	}
	if result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_member", "You are not a member of this guild")
		return
	}

	h.Pool.Exec(r.Context(),
		`DELETE FROM member_roles WHERE guild_id = $1 AND user_id = $2`, guildID, userID)

	h.EventBus.PublishJSON(r.Context(), events.SubjectGuildMemberRemove, "GUILD_MEMBER_REMOVE", map[string]string{
		"guild_id": guildID, "user_id": userID,
	})

	w.WriteHeader(http.StatusNoContent)
}

// HandleTransferGuildOwnership transfers guild ownership to another member.
// POST /api/v1/guilds/{guildID}/transfer
func (h *Handler) HandleTransferGuildOwnership(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	// Only the owner can transfer ownership.
	var ownerID string
	if err := h.Pool.QueryRow(r.Context(), `SELECT owner_id FROM guilds WHERE id = $1`, guildID).Scan(&ownerID); err != nil {
		writeError(w, http.StatusNotFound, "guild_not_found", "Guild not found")
		return
	}
	if ownerID != userID {
		writeError(w, http.StatusForbidden, "not_owner", "Only the guild owner can transfer ownership")
		return
	}

	var req struct {
		NewOwnerID string `json:"new_owner_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if req.NewOwnerID == "" {
		writeError(w, http.StatusBadRequest, "missing_new_owner", "new_owner_id is required")
		return
	}
	if req.NewOwnerID == userID {
		writeError(w, http.StatusBadRequest, "already_owner", "You are already the owner")
		return
	}

	// Verify new owner is a member.
	if !h.isMember(r.Context(), guildID, req.NewOwnerID) {
		writeError(w, http.StatusBadRequest, "not_member", "New owner must be a member of the guild")
		return
	}

	var guild models.Guild
	err := h.Pool.QueryRow(r.Context(),
		`UPDATE guilds SET owner_id = $2
		 WHERE id = $1
		 RETURNING id, instance_id, owner_id, name, description, icon_id, banner_id,
		           default_permissions, flags, nsfw, discoverable, preferred_locale, max_members,
		           verification_level, created_at`,
		guildID, req.NewOwnerID,
	).Scan(
		&guild.ID, &guild.InstanceID, &guild.OwnerID, &guild.Name, &guild.Description,
		&guild.IconID, &guild.BannerID, &guild.DefaultPermissions, &guild.Flags,
		&guild.NSFW, &guild.Discoverable, &guild.PreferredLocale, &guild.MaxMembers,
		&guild.VerificationLevel, &guild.CreatedAt,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to transfer ownership")
		return
	}

	h.logAudit(r.Context(), guildID, userID, "guild_transfer", "guild", guildID, nil)
	h.EventBus.PublishJSON(r.Context(), events.SubjectGuildUpdate, "GUILD_UPDATE", guild)

	writeJSON(w, http.StatusOK, guild)
}

// HandleGetGuildChannels lists all channels in a guild.
// GET /api/v1/guilds/{guildID}/channels
func (h *Handler) HandleGetGuildChannels(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isMember(r.Context(), guildID, userID) {
		writeError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, guild_id, category_id, channel_type, name, topic, position,
		        slowmode_seconds, nsfw, encrypted, last_message_id, owner_id,
		        default_permissions, user_limit, bitrate, locked, locked_by, locked_at, archived, created_at
		 FROM channels WHERE guild_id = $1
		 ORDER BY position, created_at`,
		guildID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get channels")
		return
	}
	defer rows.Close()

	channels := make([]models.Channel, 0)
	for rows.Next() {
		var c models.Channel
		if err := rows.Scan(
			&c.ID, &c.GuildID, &c.CategoryID, &c.ChannelType, &c.Name, &c.Topic,
			&c.Position, &c.SlowmodeSeconds, &c.NSFW, &c.Encrypted, &c.LastMessageID,
			&c.OwnerID, &c.DefaultPermissions, &c.UserLimit, &c.Bitrate,
			&c.Locked, &c.LockedBy, &c.LockedAt, &c.Archived, &c.CreatedAt,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read channels")
			return
		}
		channels = append(channels, c)
	}

	writeJSON(w, http.StatusOK, channels)
}

// HandleCreateGuildChannel creates a new channel in a guild.
// POST /api/v1/guilds/{guildID}/channels
func (h *Handler) HandleCreateGuildChannel(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageChannels) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_CHANNELS permission")
		return
	}

	var req createChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Name == "" || len(req.Name) > 100 {
		writeError(w, http.StatusBadRequest, "invalid_name", "Channel name must be 1-100 characters")
		return
	}

	validTypes := map[string]bool{
		"text": true, "voice": true, "announcement": true, "forum": true, "stage": true,
	}
	if !validTypes[req.ChannelType] {
		writeError(w, http.StatusBadRequest, "invalid_type", "Invalid channel type")
		return
	}

	channelID := models.NewULID().String()
	nsfw := false
	if req.NSFW != nil {
		nsfw = *req.NSFW
	}
	position := 0
	if req.Position != nil {
		position = *req.Position
	}

	var channel models.Channel
	err := h.Pool.QueryRow(r.Context(),
		`INSERT INTO channels (id, guild_id, category_id, channel_type, name, topic, position, nsfw, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
		 RETURNING id, guild_id, category_id, channel_type, name, topic, position,
		           slowmode_seconds, nsfw, encrypted, last_message_id, owner_id,
		           default_permissions, user_limit, bitrate, locked, locked_by, locked_at, archived, created_at`,
		channelID, guildID, req.CategoryID, req.ChannelType, req.Name, req.Topic, position, nsfw,
	).Scan(
		&channel.ID, &channel.GuildID, &channel.CategoryID, &channel.ChannelType, &channel.Name,
		&channel.Topic, &channel.Position, &channel.SlowmodeSeconds, &channel.NSFW, &channel.Encrypted,
		&channel.LastMessageID, &channel.OwnerID, &channel.DefaultPermissions,
		&channel.UserLimit, &channel.Bitrate,
		&channel.Locked, &channel.LockedBy, &channel.LockedAt, &channel.Archived, &channel.CreatedAt,
	)
	if err != nil {
		h.Logger.Error("failed to create channel", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create channel")
		return
	}

	h.logAudit(r.Context(), guildID, userID, "channel_create", "channel", channelID, nil)
	channelData, err := json.Marshal(channel)
	if err != nil {
		h.Logger.Error("failed to marshal channel for event", slog.String("error", err.Error()))
	} else {
		h.EventBus.Publish(r.Context(), events.SubjectChannelCreate, events.Event{
			Type:    "CHANNEL_CREATE",
			GuildID: guildID,
			Data:    channelData,
		})
	}

	writeJSON(w, http.StatusCreated, channel)
}

// HandleGetGuildMembers lists members of a guild.
// GET /api/v1/guilds/{guildID}/members
func (h *Handler) HandleGetGuildMembers(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isMember(r.Context(), guildID, userID) {
		writeError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT gm.guild_id, gm.user_id, gm.nickname, gm.avatar_id, gm.joined_at,
		        gm.timeout_until, gm.deaf, gm.mute,
		        u.id, u.instance_id, u.username, u.display_name, u.avatar_id,
		        u.status_text, u.status_emoji, u.status_presence, u.status_expires_at,
		        u.bio, u.banner_id, u.accent_color, u.pronouns, u.flags, u.created_at
		 FROM guild_members gm
		 JOIN users u ON u.id = gm.user_id
		 WHERE gm.guild_id = $1
		 ORDER BY gm.joined_at
		 LIMIT 1000`,
		guildID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get members")
		return
	}
	defer rows.Close()

	members := make([]models.GuildMember, 0)
	for rows.Next() {
		var m models.GuildMember
		var u models.User
		if err := rows.Scan(
			&m.GuildID, &m.UserID, &m.Nickname, &m.AvatarID, &m.JoinedAt,
			&m.TimeoutUntil, &m.Deaf, &m.Mute,
			&u.ID, &u.InstanceID, &u.Username, &u.DisplayName, &u.AvatarID,
			&u.StatusText, &u.StatusEmoji, &u.StatusPresence, &u.StatusExpiresAt,
			&u.Bio, &u.BannerID, &u.AccentColor, &u.Pronouns, &u.Flags, &u.CreatedAt,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read members")
			return
		}
		m.User = &u
		members = append(members, m)
	}

	writeJSON(w, http.StatusOK, members)
}

// HandleGetGuildMember returns a single guild member.
// GET /api/v1/guilds/{guildID}/members/{memberID}
func (h *Handler) HandleGetGuildMember(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	memberID := chi.URLParam(r, "memberID")

	if !h.isMember(r.Context(), guildID, userID) {
		writeError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	var m models.GuildMember
	err := h.Pool.QueryRow(r.Context(),
		`SELECT guild_id, user_id, nickname, avatar_id, joined_at, timeout_until, deaf, mute
		 FROM guild_members WHERE guild_id = $1 AND user_id = $2`,
		guildID, memberID,
	).Scan(&m.GuildID, &m.UserID, &m.Nickname, &m.AvatarID, &m.JoinedAt, &m.TimeoutUntil, &m.Deaf, &m.Mute)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "member_not_found", "Member not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get member")
		return
	}

	writeJSON(w, http.StatusOK, m)
}

// HandleSearchGuildMembers searches for guild members by username or nickname.
// GET /api/v1/guilds/{guildID}/members/search?q=<query>
func (h *Handler) HandleSearchGuildMembers(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isMember(r.Context(), guildID, userID) {
		writeError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		writeError(w, http.StatusBadRequest, "missing_query", "Query parameter 'q' is required")
		return
	}
	if len(query) > 100 {
		query = query[:100]
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT gm.guild_id, gm.user_id, gm.nickname, gm.avatar_id, gm.joined_at, gm.timeout_until, gm.deaf, gm.mute
		 FROM guild_members gm
		 JOIN users u ON u.id = gm.user_id
		 WHERE gm.guild_id = $1
		   AND (u.username ILIKE '%' || $2 || '%' OR gm.nickname ILIKE '%' || $2 || '%')
		 ORDER BY u.username
		 LIMIT 25`,
		guildID, query,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to search members")
		return
	}
	defer rows.Close()

	members := make([]models.GuildMember, 0)
	for rows.Next() {
		var m models.GuildMember
		if err := rows.Scan(&m.GuildID, &m.UserID, &m.Nickname, &m.AvatarID, &m.JoinedAt, &m.TimeoutUntil, &m.Deaf, &m.Mute); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read members")
			return
		}
		members = append(members, m)
	}

	writeJSON(w, http.StatusOK, members)
}

// HandleUpdateGuildMember updates a guild member's properties.
// PATCH /api/v1/guilds/{guildID}/members/{memberID}
func (h *Handler) HandleUpdateGuildMember(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	memberID := chi.URLParam(r, "memberID")

	var req updateMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	// Permission check: manage nicknames for nickname, manage roles for roles, etc.
	if req.Nickname != nil && userID != memberID {
		if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageNicknames) {
			writeError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_NICKNAMES permission")
			return
		}
	}
	if req.Deaf != nil || req.Mute != nil || req.TimeoutUntil != nil {
		if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.TimeoutMembers) {
			writeError(w, http.StatusForbidden, "missing_permission", "You need TIMEOUT_MEMBERS permission")
			return
		}
	}

	var m models.GuildMember
	err := h.Pool.QueryRow(r.Context(),
		`UPDATE guild_members SET
			nickname = COALESCE($3, nickname),
			deaf = COALESCE($4, deaf),
			mute = COALESCE($5, mute),
			timeout_until = COALESCE($6, timeout_until)
		 WHERE guild_id = $1 AND user_id = $2
		 RETURNING guild_id, user_id, nickname, avatar_id, joined_at, timeout_until, deaf, mute`,
		guildID, memberID, req.Nickname, req.Deaf, req.Mute, req.TimeoutUntil,
	).Scan(&m.GuildID, &m.UserID, &m.Nickname, &m.AvatarID, &m.JoinedAt, &m.TimeoutUntil, &m.Deaf, &m.Mute)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "member_not_found", "Member not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update member")
		return
	}

	// Handle role assignment.
	if req.Roles != nil {
		if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.AssignRoles) {
			writeError(w, http.StatusForbidden, "missing_permission", "You need ASSIGN_ROLES permission")
			return
		}
		h.Pool.Exec(r.Context(), `DELETE FROM member_roles WHERE guild_id = $1 AND user_id = $2`, guildID, memberID)
		for _, roleID := range req.Roles {
			h.Pool.Exec(r.Context(),
				`INSERT INTO member_roles (guild_id, user_id, role_id) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
				guildID, memberID, roleID)
		}
	}

	h.logAudit(r.Context(), guildID, userID, "member_update", "user", memberID, nil)
	h.EventBus.PublishJSON(r.Context(), events.SubjectGuildMemberUpdate, "GUILD_MEMBER_UPDATE", m)

	writeJSON(w, http.StatusOK, m)
}

// HandleRemoveGuildMember kicks a member from the guild.
// DELETE /api/v1/guilds/{guildID}/members/{memberID}
func (h *Handler) HandleRemoveGuildMember(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	memberID := chi.URLParam(r, "memberID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.KickMembers) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need KICK_MEMBERS permission")
		return
	}

	// Can't kick the owner.
	var ownerID string
	_ = h.Pool.QueryRow(r.Context(), `SELECT owner_id FROM guilds WHERE id = $1`, guildID).Scan(&ownerID)
	if memberID == ownerID {
		writeError(w, http.StatusForbidden, "cannot_kick_owner", "Cannot kick the guild owner")
		return
	}

	_, err := h.Pool.Exec(r.Context(),
		`DELETE FROM guild_members WHERE guild_id = $1 AND user_id = $2`, guildID, memberID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to remove member")
		return
	}

	h.logAudit(r.Context(), guildID, userID, "member_kick", "user", memberID, nil)
	h.EventBus.PublishJSON(r.Context(), events.SubjectGuildMemberRemove, "GUILD_MEMBER_REMOVE", map[string]string{
		"guild_id": guildID, "user_id": memberID,
	})

	w.WriteHeader(http.StatusNoContent)
}

// HandleGetGuildBans lists all bans in a guild.
// GET /api/v1/guilds/{guildID}/bans
func (h *Handler) HandleGetGuildBans(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.BanMembers) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need BAN_MEMBERS permission")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT guild_id, user_id, reason, banned_by, created_at
		 FROM guild_bans WHERE guild_id = $1
		 ORDER BY created_at DESC`,
		guildID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get bans")
		return
	}
	defer rows.Close()

	bans := make([]models.GuildBan, 0)
	for rows.Next() {
		var b models.GuildBan
		if err := rows.Scan(&b.GuildID, &b.UserID, &b.Reason, &b.BannedBy, &b.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read bans")
			return
		}
		bans = append(bans, b)
	}

	writeJSON(w, http.StatusOK, bans)
}

// HandleCreateGuildBan bans a user from the guild.
// PUT /api/v1/guilds/{guildID}/bans/{userID}
func (h *Handler) HandleCreateGuildBan(w http.ResponseWriter, r *http.Request) {
	actorID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	targetID := chi.URLParam(r, "userID")

	if !h.hasGuildPermission(r.Context(), guildID, actorID, permissions.BanMembers) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need BAN_MEMBERS permission")
		return
	}

	// Can't ban the owner.
	var ownerID string
	_ = h.Pool.QueryRow(r.Context(), `SELECT owner_id FROM guilds WHERE id = $1`, guildID).Scan(&ownerID)
	if targetID == ownerID {
		writeError(w, http.StatusForbidden, "cannot_ban_owner", "Cannot ban the guild owner")
		return
	}

	var req banRequest
	json.NewDecoder(r.Body).Decode(&req)

	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to ban user")
		return
	}
	defer tx.Rollback(r.Context())

	// Remove from members.
	tx.Exec(r.Context(), `DELETE FROM guild_members WHERE guild_id = $1 AND user_id = $2`, guildID, targetID)

	// Insert ban.
	tx.Exec(r.Context(),
		`INSERT INTO guild_bans (guild_id, user_id, reason, banned_by, created_at)
		 VALUES ($1, $2, $3, $4, now())
		 ON CONFLICT (guild_id, user_id) DO UPDATE SET reason = $3, banned_by = $4`,
		guildID, targetID, req.Reason, actorID)

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to ban user")
		return
	}

	h.logAudit(r.Context(), guildID, actorID, "member_ban", "user", targetID, req.Reason)
	h.EventBus.PublishJSON(r.Context(), events.SubjectGuildBanAdd, "GUILD_BAN_ADD", map[string]string{
		"guild_id": guildID, "user_id": targetID,
	})

	w.WriteHeader(http.StatusNoContent)
}

// HandleRemoveGuildBan unbans a user from the guild.
// DELETE /api/v1/guilds/{guildID}/bans/{userID}
func (h *Handler) HandleRemoveGuildBan(w http.ResponseWriter, r *http.Request) {
	actorID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	targetID := chi.URLParam(r, "userID")

	if !h.hasGuildPermission(r.Context(), guildID, actorID, permissions.BanMembers) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need BAN_MEMBERS permission")
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM guild_bans WHERE guild_id = $1 AND user_id = $2`, guildID, targetID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to unban user")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "ban_not_found", "User is not banned")
		return
	}

	h.logAudit(r.Context(), guildID, actorID, "member_unban", "user", targetID, nil)
	h.EventBus.PublishJSON(r.Context(), events.SubjectGuildBanRemove, "GUILD_BAN_REMOVE", map[string]string{
		"guild_id": guildID, "user_id": targetID,
	})

	w.WriteHeader(http.StatusNoContent)
}

// HandleGetGuildRoles lists all roles in a guild.
// GET /api/v1/guilds/{guildID}/roles
func (h *Handler) HandleGetGuildRoles(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isMember(r.Context(), guildID, userID) {
		writeError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, guild_id, name, color, hoist, mentionable, position,
		        permissions_allow, permissions_deny, created_at
		 FROM roles WHERE guild_id = $1
		 ORDER BY position`,
		guildID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get roles")
		return
	}
	defer rows.Close()

	roles := make([]models.Role, 0)
	for rows.Next() {
		var r models.Role
		if err := rows.Scan(
			&r.ID, &r.GuildID, &r.Name, &r.Color, &r.Hoist, &r.Mentionable,
			&r.Position, &r.PermissionsAllow, &r.PermissionsDeny, &r.CreatedAt,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read roles")
			return
		}
		roles = append(roles, r)
	}

	writeJSON(w, http.StatusOK, roles)
}

// HandleCreateGuildRole creates a new role in a guild.
// POST /api/v1/guilds/{guildID}/roles
func (h *Handler) HandleCreateGuildRole(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageRoles) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_ROLES permission")
		return
	}

	var req createRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Name == "" || len(req.Name) > 100 {
		writeError(w, http.StatusBadRequest, "invalid_name", "Role name must be 1-100 characters")
		return
	}

	roleID := models.NewULID().String()
	hoist := false
	if req.Hoist != nil {
		hoist = *req.Hoist
	}
	mentionable := false
	if req.Mentionable != nil {
		mentionable = *req.Mentionable
	}
	position := 0
	if req.Position != nil {
		position = *req.Position
	}
	var permAllow, permDeny int64
	if req.PermissionsAllow != nil {
		permAllow = *req.PermissionsAllow
	}
	if req.PermissionsDeny != nil {
		permDeny = *req.PermissionsDeny
	}

	var role models.Role
	err := h.Pool.QueryRow(r.Context(),
		`INSERT INTO roles (id, guild_id, name, color, hoist, mentionable, position, permissions_allow, permissions_deny, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
		 RETURNING id, guild_id, name, color, hoist, mentionable, position, permissions_allow, permissions_deny, created_at`,
		roleID, guildID, req.Name, req.Color, hoist, mentionable, position, permAllow, permDeny,
	).Scan(
		&role.ID, &role.GuildID, &role.Name, &role.Color, &role.Hoist, &role.Mentionable,
		&role.Position, &role.PermissionsAllow, &role.PermissionsDeny, &role.CreatedAt,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create role")
		return
	}

	h.logAudit(r.Context(), guildID, userID, "role_create", "role", roleID, nil)
	h.EventBus.PublishJSON(r.Context(), events.SubjectGuildRoleCreate, "GUILD_ROLE_CREATE", role)

	writeJSON(w, http.StatusCreated, role)
}

// HandleUpdateGuildRole updates a role's properties.
// PATCH /api/v1/guilds/{guildID}/roles/{roleID}
func (h *Handler) HandleUpdateGuildRole(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	roleID := chi.URLParam(r, "roleID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageRoles) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_ROLES permission")
		return
	}

	var req updateRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	var role models.Role
	err := h.Pool.QueryRow(r.Context(),
		`UPDATE roles SET
			name = COALESCE($3, name),
			color = COALESCE($4, color),
			hoist = COALESCE($5, hoist),
			mentionable = COALESCE($6, mentionable),
			position = COALESCE($7, position),
			permissions_allow = COALESCE($8, permissions_allow),
			permissions_deny = COALESCE($9, permissions_deny)
		 WHERE id = $1 AND guild_id = $2
		 RETURNING id, guild_id, name, color, hoist, mentionable, position, permissions_allow, permissions_deny, created_at`,
		roleID, guildID, req.Name, req.Color, req.Hoist, req.Mentionable, req.Position,
		req.PermissionsAllow, req.PermissionsDeny,
	).Scan(
		&role.ID, &role.GuildID, &role.Name, &role.Color, &role.Hoist, &role.Mentionable,
		&role.Position, &role.PermissionsAllow, &role.PermissionsDeny, &role.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "role_not_found", "Role not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update role")
		return
	}

	h.logAudit(r.Context(), guildID, userID, "role_update", "role", roleID, nil)
	h.EventBus.PublishJSON(r.Context(), events.SubjectGuildRoleUpdate, "GUILD_ROLE_UPDATE", role)

	writeJSON(w, http.StatusOK, role)
}

// HandleDeleteGuildRole deletes a role from a guild.
// DELETE /api/v1/guilds/{guildID}/roles/{roleID}
func (h *Handler) HandleDeleteGuildRole(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	roleID := chi.URLParam(r, "roleID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageRoles) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_ROLES permission")
		return
	}

	tag, err := h.Pool.Exec(r.Context(), `DELETE FROM roles WHERE id = $1 AND guild_id = $2`, roleID, guildID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete role")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "role_not_found", "Role not found")
		return
	}

	h.logAudit(r.Context(), guildID, userID, "role_delete", "role", roleID, nil)
	h.EventBus.PublishJSON(r.Context(), events.SubjectGuildRoleDelete, "GUILD_ROLE_DELETE", map[string]string{
		"guild_id": guildID, "role_id": roleID,
	})

	w.WriteHeader(http.StatusNoContent)
}

// HandleReorderGuildRoles updates the positions of multiple roles at once.
// PATCH /api/v1/guilds/{guildID}/roles
func (h *Handler) HandleReorderGuildRoles(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageRoles) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_ROLES permission")
		return
	}

	var req []struct {
		ID       string `json:"id"`
		Position int    `json:"position"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Expected array of {id, position} objects")
		return
	}

	if len(req) == 0 {
		writeError(w, http.StatusBadRequest, "empty_array", "At least one role position is required")
		return
	}

	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to reorder roles")
		return
	}
	defer tx.Rollback(r.Context())

	for _, item := range req {
		_, err := tx.Exec(r.Context(),
			`UPDATE roles SET position = $3 WHERE id = $1 AND guild_id = $2`,
			item.ID, guildID, item.Position)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to reorder roles")
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to reorder roles")
		return
	}

	// Return updated role list.
	h.HandleGetGuildRoles(w, r)
}

// HandleReorderGuildChannels updates the positions of multiple channels at once.
// PATCH /api/v1/guilds/{guildID}/channels
func (h *Handler) HandleReorderGuildChannels(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageChannels) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_CHANNELS permission")
		return
	}

	var req []struct {
		ID       string `json:"id"`
		Position int    `json:"position"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Expected array of {id, position} objects")
		return
	}

	if len(req) == 0 {
		writeError(w, http.StatusBadRequest, "empty_array", "At least one channel position is required")
		return
	}

	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to reorder channels")
		return
	}
	defer tx.Rollback(r.Context())

	for _, item := range req {
		_, err := tx.Exec(r.Context(),
			`UPDATE channels SET position = $3 WHERE id = $1 AND guild_id = $2`,
			item.ID, guildID, item.Position)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to reorder channels")
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to reorder channels")
		return
	}

	// Return updated channel list.
	h.HandleGetGuildChannels(w, r)
}

// HandleGetGuildInvites lists all invites for a guild.
// GET /api/v1/guilds/{guildID}/invites
func (h *Handler) HandleGetGuildInvites(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageGuild) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_GUILD permission")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT code, guild_id, channel_id, creator_id, max_uses, uses,
		        max_age_seconds, temporary, created_at, expires_at
		 FROM invites WHERE guild_id = $1
		 ORDER BY created_at DESC`,
		guildID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get invites")
		return
	}
	defer rows.Close()

	invites := make([]models.Invite, 0)
	for rows.Next() {
		var inv models.Invite
		if err := rows.Scan(
			&inv.Code, &inv.GuildID, &inv.ChannelID, &inv.CreatorID, &inv.MaxUses,
			&inv.Uses, &inv.MaxAgeSeconds, &inv.Temporary, &inv.CreatedAt, &inv.ExpiresAt,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read invites")
			return
		}
		invites = append(invites, inv)
	}

	writeJSON(w, http.StatusOK, invites)
}

// HandleCreateGuildInvite creates a new invite for a guild.
// POST /api/v1/guilds/{guildID}/invites
func (h *Handler) HandleCreateGuildInvite(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.CreateInvites) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need CREATE_INVITES permission")
		return
	}

	var req struct {
		ChannelID     *string `json:"channel_id"`
		MaxUses       *int    `json:"max_uses"`
		MaxAgeSeconds *int    `json:"max_age_seconds"`
		Temporary     bool    `json:"temporary"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Empty body is fine — use defaults.
		req = struct {
			ChannelID     *string `json:"channel_id"`
			MaxUses       *int    `json:"max_uses"`
			MaxAgeSeconds *int    `json:"max_age_seconds"`
			Temporary     bool    `json:"temporary"`
		}{}
	}

	code := generateInviteCode()

	// Expiry: 0 means never, nil defaults to 24 hours, positive value is seconds.
	var expiresAt *time.Time
	if req.MaxAgeSeconds != nil && *req.MaxAgeSeconds == 0 {
		// Explicitly "never" — leave expiresAt nil.
	} else if req.MaxAgeSeconds != nil && *req.MaxAgeSeconds > 0 {
		t := time.Now().Add(time.Duration(*req.MaxAgeSeconds) * time.Second)
		expiresAt = &t
	} else {
		t := time.Now().Add(24 * time.Hour)
		expiresAt = &t
	}

	var inv models.Invite
	err := h.Pool.QueryRow(r.Context(),
		`INSERT INTO invites (code, guild_id, channel_id, creator_id, max_uses, max_age_seconds, temporary, created_at, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, now(), $8)
		 RETURNING code, guild_id, channel_id, creator_id, max_uses, uses, max_age_seconds, temporary, created_at, expires_at`,
		code, guildID, req.ChannelID, userID, req.MaxUses, req.MaxAgeSeconds, req.Temporary, expiresAt,
	).Scan(
		&inv.Code, &inv.GuildID, &inv.ChannelID, &inv.CreatorID, &inv.MaxUses,
		&inv.Uses, &inv.MaxAgeSeconds, &inv.Temporary, &inv.CreatedAt, &inv.ExpiresAt,
	)
	if err != nil {
		h.Logger.Error("failed to create invite", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create invite")
		return
	}

	writeJSON(w, http.StatusCreated, inv)
}

// HandleGetGuildAuditLog returns the audit log for a guild.
// GET /api/v1/guilds/{guildID}/audit-log
func (h *Handler) HandleGetGuildAuditLog(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ViewAuditLog) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need VIEW_AUDIT_LOG permission")
		return
	}

	// Build filtered query for audit log.
	baseSQL := `SELECT id, guild_id, actor_id, action, target_type, target_id, reason, changes, created_at
		 FROM audit_log WHERE guild_id = $1`
	args := []interface{}{guildID}
	argIdx := 2

	if action := r.URL.Query().Get("action"); action != "" {
		baseSQL += fmt.Sprintf(` AND action = $%d`, argIdx)
		args = append(args, action)
		argIdx++
	}
	if actorID := r.URL.Query().Get("actor_id"); actorID != "" {
		baseSQL += fmt.Sprintf(` AND actor_id = $%d`, argIdx)
		args = append(args, actorID)
		argIdx++
	}
	if before := r.URL.Query().Get("before"); before != "" {
		baseSQL += fmt.Sprintf(` AND id < $%d`, argIdx)
		args = append(args, before)
		argIdx++
	}

	baseSQL += ` ORDER BY created_at DESC LIMIT 100`

	rows, err := h.Pool.Query(r.Context(), baseSQL, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get audit log")
		return
	}
	defer rows.Close()

	entries := make([]models.AuditLogEntry, 0)
	for rows.Next() {
		var e models.AuditLogEntry
		if err := rows.Scan(
			&e.ID, &e.GuildID, &e.ActorID, &e.Action, &e.TargetType,
			&e.TargetID, &e.Reason, &e.Changes, &e.CreatedAt,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read audit log")
			return
		}
		entries = append(entries, e)
	}

	writeJSON(w, http.StatusOK, entries)
}

// HandleGetGuildEmoji lists custom emoji for a guild.
// GET /api/v1/guilds/{guildID}/emoji
func (h *Handler) HandleGetGuildEmoji(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isMember(r.Context(), guildID, userID) {
		writeError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, guild_id, name, creator_id, animated, s3_key, created_at
		 FROM custom_emoji WHERE guild_id = $1
		 ORDER BY name`,
		guildID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get emoji")
		return
	}
	defer rows.Close()

	emoji := make([]models.CustomEmoji, 0)
	for rows.Next() {
		var e models.CustomEmoji
		if err := rows.Scan(&e.ID, &e.GuildID, &e.Name, &e.CreatorID, &e.Animated, &e.S3Key, &e.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read emoji")
			return
		}
		emoji = append(emoji, e)
	}

	writeJSON(w, http.StatusOK, emoji)
}

// HandleCreateGuildEmoji creates a custom emoji (metadata only; file upload is separate).
// POST /api/v1/guilds/{guildID}/emoji
func (h *Handler) HandleCreateGuildEmoji(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageEmoji) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_EMOJI permission")
		return
	}

	var req struct {
		Name     string `json:"name"`
		S3Key    string `json:"s3_key"`
		Animated bool   `json:"animated"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Name == "" || len(req.Name) > 32 {
		writeError(w, http.StatusBadRequest, "invalid_name", "Emoji name must be 1-32 characters")
		return
	}

	emojiID := models.NewULID().String()
	var emoji models.CustomEmoji
	err := h.Pool.QueryRow(r.Context(),
		`INSERT INTO custom_emoji (id, guild_id, name, creator_id, animated, s3_key, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, now())
		 RETURNING id, guild_id, name, creator_id, animated, s3_key, created_at`,
		emojiID, guildID, req.Name, userID, req.Animated, req.S3Key,
	).Scan(&emoji.ID, &emoji.GuildID, &emoji.Name, &emoji.CreatorID, &emoji.Animated, &emoji.S3Key, &emoji.CreatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create emoji")
		return
	}

	h.EventBus.PublishJSON(r.Context(), events.SubjectGuildEmojiUpdate, "GUILD_EMOJI_UPDATE", emoji)

	writeJSON(w, http.StatusCreated, emoji)
}

// HandleUpdateGuildEmoji updates a custom emoji's name.
// PATCH /api/v1/guilds/{guildID}/emoji/{emojiID}
func (h *Handler) HandleUpdateGuildEmoji(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	emojiID := chi.URLParam(r, "emojiID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageEmoji) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_EMOJI permission")
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if req.Name == "" || len(req.Name) > 32 {
		writeError(w, http.StatusBadRequest, "invalid_name", "Emoji name must be 1-32 characters")
		return
	}

	var emoji models.CustomEmoji
	err := h.Pool.QueryRow(r.Context(),
		`UPDATE custom_emoji SET name = $1
		 WHERE id = $2 AND guild_id = $3
		 RETURNING id, guild_id, name, creator_id, animated, s3_key, created_at`,
		req.Name, emojiID, guildID,
	).Scan(&emoji.ID, &emoji.GuildID, &emoji.Name, &emoji.CreatorID, &emoji.Animated, &emoji.S3Key, &emoji.CreatedAt)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "emoji_not_found", "Emoji not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update emoji")
		return
	}

	h.EventBus.PublishJSON(r.Context(), events.SubjectGuildEmojiUpdate, "GUILD_EMOJI_UPDATE", emoji)

	writeJSON(w, http.StatusOK, emoji)
}

// HandleDeleteGuildEmoji deletes a custom emoji.
// DELETE /api/v1/guilds/{guildID}/emoji/{emojiID}
func (h *Handler) HandleDeleteGuildEmoji(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	emojiID := chi.URLParam(r, "emojiID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageEmoji) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_EMOJI permission")
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM custom_emoji WHERE id = $1 AND guild_id = $2`, emojiID, guildID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete emoji")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "emoji_not_found", "Emoji not found")
		return
	}

	h.EventBus.PublishJSON(r.Context(), events.SubjectGuildEmojiUpdate, "GUILD_EMOJI_DELETE",
		map[string]string{"id": emojiID, "guild_id": guildID})

	w.WriteHeader(http.StatusNoContent)
}

// HandleGetGuildCategories lists channel categories for a guild.
// GET /api/v1/guilds/{guildID}/categories
func (h *Handler) HandleGetGuildCategories(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")
	userID := auth.UserIDFromContext(r.Context())

	if !h.isMember(r.Context(), guildID, userID) {
		writeError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, guild_id, name, position, created_at
		 FROM guild_categories WHERE guild_id = $1
		 ORDER BY position`,
		guildID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get categories")
		return
	}
	defer rows.Close()

	categories := make([]models.GuildCategory, 0)
	for rows.Next() {
		var c models.GuildCategory
		if err := rows.Scan(&c.ID, &c.GuildID, &c.Name, &c.Position, &c.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read categories")
			return
		}
		categories = append(categories, c)
	}

	writeJSON(w, http.StatusOK, categories)
}

// HandleCreateGuildCategory creates a new channel category.
// POST /api/v1/guilds/{guildID}/categories
func (h *Handler) HandleCreateGuildCategory(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")
	userID := auth.UserIDFromContext(r.Context())

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageChannels) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_CHANNELS permission")
		return
	}

	var req struct {
		Name     string `json:"name"`
		Position *int   `json:"position"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if req.Name == "" || len(req.Name) > 100 {
		writeError(w, http.StatusBadRequest, "invalid_name", "Category name must be 1-100 characters")
		return
	}

	catID := models.NewULID().String()
	position := 0
	if req.Position != nil {
		position = *req.Position
	}

	var cat models.GuildCategory
	err := h.Pool.QueryRow(r.Context(),
		`INSERT INTO guild_categories (id, guild_id, name, position, created_at)
		 VALUES ($1, $2, $3, $4, now())
		 RETURNING id, guild_id, name, position, created_at`,
		catID, guildID, req.Name, position,
	).Scan(&cat.ID, &cat.GuildID, &cat.Name, &cat.Position, &cat.CreatedAt)
	if err != nil {
		h.Logger.Error("failed to create category", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create category")
		return
	}

	writeJSON(w, http.StatusCreated, cat)
}

// HandleUpdateGuildCategory updates a channel category's name or position.
// PATCH /api/v1/guilds/{guildID}/categories/{categoryID}
func (h *Handler) HandleUpdateGuildCategory(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")
	categoryID := chi.URLParam(r, "categoryID")
	userID := auth.UserIDFromContext(r.Context())

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageChannels) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_CHANNELS permission")
		return
	}

	var req struct {
		Name     *string `json:"name"`
		Position *int    `json:"position"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	var cat models.GuildCategory
	err := h.Pool.QueryRow(r.Context(),
		`UPDATE guild_categories SET
			name = COALESCE($3, name),
			position = COALESCE($4, position)
		 WHERE id = $1 AND guild_id = $2
		 RETURNING id, guild_id, name, position, created_at`,
		categoryID, guildID, req.Name, req.Position,
	).Scan(&cat.ID, &cat.GuildID, &cat.Name, &cat.Position, &cat.CreatedAt)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "category_not_found", "Category not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update category")
		return
	}

	writeJSON(w, http.StatusOK, cat)
}

// HandleDeleteGuildCategory deletes a channel category. Channels in this
// category will have their category_id set to NULL.
// DELETE /api/v1/guilds/{guildID}/categories/{categoryID}
func (h *Handler) HandleDeleteGuildCategory(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")
	categoryID := chi.URLParam(r, "categoryID")
	userID := auth.UserIDFromContext(r.Context())

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageChannels) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_CHANNELS permission")
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM guild_categories WHERE id = $1 AND guild_id = $2`, categoryID, guildID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete category")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "category_not_found", "Category not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Internal helpers ---

// HandleGetGuildWebhooks lists all webhooks for a guild.
// GET /api/v1/guilds/{guildID}/webhooks
func (h *Handler) HandleGetGuildWebhooks(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageWebhooks) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_WEBHOOKS permission")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, guild_id, channel_id, creator_id, name, avatar_id, token,
		        webhook_type, outgoing_url, created_at
		 FROM webhooks WHERE guild_id = $1
		 ORDER BY created_at DESC`,
		guildID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get webhooks")
		return
	}
	defer rows.Close()

	webhooks := make([]models.Webhook, 0)
	for rows.Next() {
		var wh models.Webhook
		if err := rows.Scan(
			&wh.ID, &wh.GuildID, &wh.ChannelID, &wh.CreatorID, &wh.Name,
			&wh.AvatarID, &wh.Token, &wh.WebhookType, &wh.OutgoingURL, &wh.CreatedAt,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read webhooks")
			return
		}
		webhooks = append(webhooks, wh)
	}

	writeJSON(w, http.StatusOK, webhooks)
}

// HandleCreateGuildWebhook creates a new webhook for a guild channel.
// POST /api/v1/guilds/{guildID}/webhooks
func (h *Handler) HandleCreateGuildWebhook(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageWebhooks) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_WEBHOOKS permission")
		return
	}

	var req struct {
		Name      string  `json:"name"`
		ChannelID string  `json:"channel_id"`
		AvatarID  *string `json:"avatar_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Name == "" || len(req.Name) > 80 || req.ChannelID == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "Name (1-80 chars) and channel_id are required")
		return
	}

	// Verify the channel belongs to this guild.
	var channelGuildID *string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT guild_id FROM channels WHERE id = $1`, req.ChannelID).Scan(&channelGuildID)
	if err != nil || channelGuildID == nil || *channelGuildID != guildID {
		writeError(w, http.StatusBadRequest, "invalid_channel", "Channel not found in this guild")
		return
	}

	webhookID := models.NewULID().String()
	token := generateInviteCode() + generateInviteCode() + generateInviteCode() // 36 char token

	var wh models.Webhook
	err = h.Pool.QueryRow(r.Context(),
		`INSERT INTO webhooks (id, guild_id, channel_id, creator_id, name, avatar_id, token, webhook_type, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, 'incoming', now())
		 RETURNING id, guild_id, channel_id, creator_id, name, avatar_id, token, webhook_type, outgoing_url, created_at`,
		webhookID, guildID, req.ChannelID, userID, req.Name, req.AvatarID, token,
	).Scan(
		&wh.ID, &wh.GuildID, &wh.ChannelID, &wh.CreatorID, &wh.Name,
		&wh.AvatarID, &wh.Token, &wh.WebhookType, &wh.OutgoingURL, &wh.CreatedAt,
	)
	if err != nil {
		h.Logger.Error("failed to create webhook", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create webhook")
		return
	}

	h.logAudit(r.Context(), guildID, userID, "WEBHOOK_CREATE", "webhook", webhookID, nil)
	writeJSON(w, http.StatusCreated, wh)
}

// HandleUpdateGuildWebhook updates an existing webhook.
// PATCH /api/v1/guilds/{guildID}/webhooks/{webhookID}
func (h *Handler) HandleUpdateGuildWebhook(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	webhookID := chi.URLParam(r, "webhookID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageWebhooks) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_WEBHOOKS permission")
		return
	}

	var req struct {
		Name      *string `json:"name"`
		ChannelID *string `json:"channel_id"`
		AvatarID  *string `json:"avatar_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	var wh models.Webhook
	err := h.Pool.QueryRow(r.Context(),
		`UPDATE webhooks SET
			name = COALESCE($1, name),
			channel_id = COALESCE($2, channel_id),
			avatar_id = COALESCE($3, avatar_id)
		 WHERE id = $4 AND guild_id = $5
		 RETURNING id, guild_id, channel_id, creator_id, name, avatar_id, token, webhook_type, outgoing_url, created_at`,
		req.Name, req.ChannelID, req.AvatarID, webhookID, guildID,
	).Scan(
		&wh.ID, &wh.GuildID, &wh.ChannelID, &wh.CreatorID, &wh.Name,
		&wh.AvatarID, &wh.Token, &wh.WebhookType, &wh.OutgoingURL, &wh.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "webhook_not_found", "Webhook not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update webhook")
		return
	}

	h.logAudit(r.Context(), guildID, userID, "WEBHOOK_UPDATE", "webhook", webhookID, nil)
	writeJSON(w, http.StatusOK, wh)
}

// HandleDeleteGuildWebhook deletes a webhook.
// DELETE /api/v1/guilds/{guildID}/webhooks/{webhookID}
func (h *Handler) HandleDeleteGuildWebhook(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	webhookID := chi.URLParam(r, "webhookID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageWebhooks) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_WEBHOOKS permission")
		return
	}

	result, err := h.Pool.Exec(r.Context(),
		`DELETE FROM webhooks WHERE id = $1 AND guild_id = $2`, webhookID, guildID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete webhook")
		return
	}

	if result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "webhook_not_found", "Webhook not found")
		return
	}

	h.logAudit(r.Context(), guildID, userID, "WEBHOOK_DELETE", "webhook", webhookID, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) getGuild(ctx context.Context, guildID string) (*models.Guild, error) {
	var g models.Guild
	err := h.Pool.QueryRow(ctx,
		`SELECT g.id, g.instance_id, g.owner_id, g.name, g.description, g.icon_id, g.banner_id,
		        g.default_permissions, g.flags, g.nsfw, g.discoverable, g.preferred_locale,
		        g.max_members, g.vanity_url, g.verification_level, g.afk_channel_id, g.afk_timeout,
		        g.tags, g.member_count, g.created_at
		 FROM guilds g WHERE g.id = $1`,
		guildID,
	).Scan(
		&g.ID, &g.InstanceID, &g.OwnerID, &g.Name, &g.Description, &g.IconID,
		&g.BannerID, &g.DefaultPermissions, &g.Flags, &g.NSFW, &g.Discoverable,
		&g.PreferredLocale, &g.MaxMembers, &g.VanityURL, &g.VerificationLevel, &g.AFKChannelID, &g.AFKTimeout,
		&g.Tags, &g.MemberCount, &g.CreatedAt,
	)
	return &g, err
}

// HandleGetGuildPreview returns a limited preview of a guild for non-members.
// Includes basic info, approximate member count, emoji count, and top channels.
// GET /api/v1/guilds/{guildID}/preview
func (h *Handler) HandleGetGuildPreview(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")

	var g models.Guild
	err := h.Pool.QueryRow(r.Context(),
		`SELECT g.id, g.instance_id, g.owner_id, g.name, g.description, g.icon_id, g.banner_id,
		        g.flags, g.nsfw, g.discoverable, g.preferred_locale,
		        g.verification_level, g.afk_channel_id, g.afk_timeout,
		        g.tags, g.member_count, g.created_at
		 FROM guilds g WHERE g.id = $1`,
		guildID,
	).Scan(
		&g.ID, &g.InstanceID, &g.OwnerID, &g.Name, &g.Description, &g.IconID,
		&g.BannerID, &g.Flags, &g.NSFW, &g.Discoverable, &g.PreferredLocale,
		&g.VerificationLevel, &g.AFKChannelID, &g.AFKTimeout,
		&g.Tags, &g.MemberCount, &g.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "guild_not_found", "Guild not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get guild preview")
		return
	}

	var emojiCount int
	h.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM custom_emoji WHERE guild_id = $1`, guildID).Scan(&emojiCount)

	type channelPreview struct {
		ID          string  `json:"id"`
		ChannelType string  `json:"channel_type"`
		Name        *string `json:"name"`
	}
	channelPreviews := make([]channelPreview, 0)
	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, channel_type, name
		 FROM channels WHERE guild_id = $1
		 ORDER BY position
		 LIMIT 5`,
		guildID,
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var cp channelPreview
			if err := rows.Scan(&cp.ID, &cp.ChannelType, &cp.Name); err == nil {
				channelPreviews = append(channelPreviews, cp)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":           g.ID,
		"name":         g.Name,
		"description":  g.Description,
		"icon_id":      g.IconID,
		"banner_id":    g.BannerID,
		"nsfw":         g.NSFW,
		"discoverable": g.Discoverable,
		"member_count": g.MemberCount,
		"emoji_count":  emojiCount,
		"channels":     channelPreviews,
		"created_at":   g.CreatedAt,
	})
}

// HandleGetGuildVanityURL returns the current vanity URL for a guild.
// GET /api/v1/guilds/{guildID}/vanity-url
func (h *Handler) HandleGetGuildVanityURL(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")

	var vanityURL *string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT vanity_url FROM guilds WHERE id = $1`, guildID).Scan(&vanityURL)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "guild_not_found", "Guild not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get vanity URL")
		return
	}

	code := ""
	if vanityURL != nil {
		code = *vanityURL
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"code": code,
	})
}

// HandleSetGuildVanityURL sets or updates the vanity URL for a guild.
// Only the guild owner can set a vanity URL.
// PATCH /api/v1/guilds/{guildID}/vanity-url
func (h *Handler) HandleSetGuildVanityURL(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")
	userID := auth.UserIDFromContext(r.Context())

	// Only guild owner can set vanity URL.
	var ownerID string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT owner_id FROM guilds WHERE id = $1`, guildID).Scan(&ownerID)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "guild_not_found", "Guild not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get guild")
		return
	}
	if ownerID != userID {
		writeError(w, http.StatusForbidden, "not_owner", "Only the guild owner can set a vanity URL")
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	// Validate: 3-32 characters, alphanumeric and hyphens only.
	if len(req.Code) < 3 || len(req.Code) > 32 {
		writeError(w, http.StatusBadRequest, "invalid_code", "Vanity URL must be 3-32 characters")
		return
	}
	for _, c := range req.Code {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
			writeError(w, http.StatusBadRequest, "invalid_code", "Vanity URL must be lowercase alphanumeric or hyphens")
			return
		}
	}

	_, err = h.Pool.Exec(r.Context(),
		`UPDATE guilds SET vanity_url = $1 WHERE id = $2`, req.Code, guildID)
	if err != nil {
		// Unique constraint violation.
		writeError(w, http.StatusConflict, "vanity_taken", "This vanity URL is already in use")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"code": req.Code,
	})
}

// HandleResolveVanityURL resolves a vanity URL to a guild invite-like object.
// GET /api/v1/guilds/vanity/{code}
func (h *Handler) HandleResolveVanityURL(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	var guildID, guildName string
	var memberCount int
	err := h.Pool.QueryRow(r.Context(),
		`SELECT id, name, member_count FROM guilds WHERE vanity_url = $1`, code).Scan(&guildID, &guildName, &memberCount)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "vanity_not_found", "No guild found with this vanity URL")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to resolve vanity URL")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"guild_id":     guildID,
		"guild_name":   guildName,
		"code":         code,
		"member_count": memberCount,
	})
}

// HandleDiscoverGuilds returns a list of public, discoverable guilds.
// Guilds that have been bumped recently appear higher in the listing.
// GET /api/v1/guilds/discover
func (h *Handler) HandleDiscoverGuilds(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	tag := r.URL.Query().Get("tag")
	sortBy := r.URL.Query().Get("sort")
	limit := 50

	// The bump_score subquery counts bumps in the last 24 hours.
	baseSQL := `SELECT g.id, g.instance_id, g.owner_id, g.name, g.description, g.icon_id,
	            g.banner_id, g.default_permissions, g.flags, g.nsfw, g.discoverable,
	            g.preferred_locale, g.max_members, g.vanity_url, g.verification_level,
	            g.afk_channel_id, g.afk_timeout, g.tags,
	            g.member_count, g.created_at
	     FROM guilds g
	     WHERE g.discoverable = true`

	argN := 1
	var args []interface{}
	if query != "" {
		baseSQL += fmt.Sprintf(` AND g.name ILIKE '%%' || $%d || '%%'`, argN)
		args = append(args, query)
		argN++
	}
	if tag != "" && tag != "All" {
		baseSQL += fmt.Sprintf(` AND $%d = ANY(g.tags)`, argN)
		args = append(args, tag)
		argN++
	}

	// Sort by bumps if requested, otherwise default to member_count.
	if sortBy == "bumps" {
		baseSQL += fmt.Sprintf(` ORDER BY (SELECT COUNT(*) FROM guild_bumps b WHERE b.guild_id = g.id AND b.bumped_at > now() - INTERVAL '24 hours') DESC, g.member_count DESC LIMIT $%d`, argN)
	} else {
		baseSQL += fmt.Sprintf(` ORDER BY g.member_count DESC LIMIT $%d`, argN)
	}
	args = append(args, limit)

	rows, err := h.Pool.Query(r.Context(), baseSQL, args...)
	if err != nil {
		h.Logger.Error("discover guilds query failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to discover guilds")
		return
	}
	defer rows.Close()

	guilds := make([]models.Guild, 0)
	for rows.Next() {
		var g models.Guild
		if err := rows.Scan(
			&g.ID, &g.InstanceID, &g.OwnerID, &g.Name, &g.Description, &g.IconID,
			&g.BannerID, &g.DefaultPermissions, &g.Flags, &g.NSFW, &g.Discoverable,
			&g.PreferredLocale, &g.MaxMembers, &g.VanityURL, &g.VerificationLevel,
			&g.AFKChannelID, &g.AFKTimeout, &g.Tags,
			&g.MemberCount, &g.CreatedAt,
		); err != nil {
			h.Logger.Error("discover guilds scan failed", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read guilds")
			return
		}
		guilds = append(guilds, g)
	}

	writeJSON(w, http.StatusOK, guilds)
}

// HandleJoinDiscoverableGuild allows a user to join a discoverable guild directly.
// POST /api/v1/guilds/{guildID}/join
func (h *Handler) HandleJoinDiscoverableGuild(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	// Check guild exists and is discoverable.
	var discoverable bool
	var guildName string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT discoverable, name FROM guilds WHERE id = $1`, guildID,
	).Scan(&discoverable, &guildName)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Guild not found")
		return
	}
	if !discoverable {
		writeError(w, http.StatusForbidden, "not_discoverable", "This guild is not open for direct joins")
		return
	}

	// Check already a member.
	if h.isMember(r.Context(), guildID, userID) {
		writeError(w, http.StatusConflict, "already_member", "You are already a member of this guild")
		return
	}

	// Check ban.
	var banned bool
	h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM guild_bans WHERE guild_id = $1 AND user_id = $2)`,
		guildID, userID,
	).Scan(&banned)
	if banned {
		writeError(w, http.StatusForbidden, "banned", "You are banned from this guild")
		return
	}

	// Add member.
	_, err = h.Pool.Exec(r.Context(),
		`INSERT INTO guild_members (guild_id, user_id, joined_at) VALUES ($1, $2, now())`,
		guildID, userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to join guild")
		return
	}

	// Increment member count.
	h.Pool.Exec(r.Context(), `UPDATE guilds SET member_count = member_count + 1 WHERE id = $1`, guildID)

	// Publish guild join event.
	if h.EventBus != nil {
		h.EventBus.PublishJSON(r.Context(), events.SubjectGuildMemberAdd, "GUILD_MEMBER_ADD",
			map[string]interface{}{
				"guild_id": guildID,
				"user_id":  userID,
			})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"guild_id": guildID,
		"name":     guildName,
		"joined":   true,
	})
}

func (h *Handler) isMember(ctx context.Context, guildID, userID string) bool {
	var exists bool
	h.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM guild_members WHERE guild_id = $1 AND user_id = $2)`,
		guildID, userID,
	).Scan(&exists)
	return exists
}

func (h *Handler) hasGuildPermission(ctx context.Context, guildID, userID string, perm uint64) bool {
	// Owner has all permissions.
	var ownerID string
	if err := h.Pool.QueryRow(ctx, `SELECT owner_id FROM guilds WHERE id = $1`, guildID).Scan(&ownerID); err != nil {
		return false
	}
	if userID == ownerID {
		return true
	}

	// Check admin flag on user.
	var userFlags int
	h.Pool.QueryRow(ctx, `SELECT flags FROM users WHERE id = $1`, userID).Scan(&userFlags)
	if userFlags&models.UserFlagAdmin != 0 {
		return true
	}

	// Get guild default permissions.
	var defaultPerms int64
	h.Pool.QueryRow(ctx, `SELECT default_permissions FROM guilds WHERE id = $1`, guildID).Scan(&defaultPerms)
	computedPerms := uint64(defaultPerms)

	// Apply member's role permissions.
	rows, _ := h.Pool.Query(ctx,
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
			computedPerms |= uint64(allow)
			computedPerms &^= uint64(deny)
		}
	}

	if computedPerms&permissions.Administrator != 0 {
		return true
	}

	return computedPerms&perm != 0
}

// HandleAddMemberRole adds a single role to a guild member.
// PUT /api/v1/guilds/{guildID}/members/{memberID}/roles/{roleID}
func (h *Handler) HandleAddMemberRole(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	memberID := chi.URLParam(r, "memberID")
	roleID := chi.URLParam(r, "roleID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.AssignRoles) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need ASSIGN_ROLES permission")
		return
	}

	// Verify the role belongs to this guild.
	var exists bool
	err := h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM roles WHERE id = $1 AND guild_id = $2)`, roleID, guildID).Scan(&exists)
	if err != nil || !exists {
		writeError(w, http.StatusNotFound, "role_not_found", "Role not found in this guild")
		return
	}

	// Verify the member exists.
	err = h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM guild_members WHERE guild_id = $1 AND user_id = $2)`, guildID, memberID).Scan(&exists)
	if err != nil || !exists {
		writeError(w, http.StatusNotFound, "member_not_found", "Member not found in this guild")
		return
	}

	_, err = h.Pool.Exec(r.Context(),
		`INSERT INTO member_roles (guild_id, user_id, role_id) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
		guildID, memberID, roleID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to add role")
		return
	}

	h.logAudit(r.Context(), guildID, userID, "member_role_add", "role", roleID, nil)
	h.EventBus.PublishJSON(r.Context(), events.SubjectGuildMemberUpdate, "GUILD_MEMBER_UPDATE", map[string]string{
		"guild_id": guildID, "user_id": memberID, "role_id": roleID, "action": "role_add",
	})

	w.WriteHeader(http.StatusNoContent)
}

// HandleRemoveMemberRole removes a single role from a guild member.
// DELETE /api/v1/guilds/{guildID}/members/{memberID}/roles/{roleID}
func (h *Handler) HandleRemoveMemberRole(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	memberID := chi.URLParam(r, "memberID")
	roleID := chi.URLParam(r, "roleID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.AssignRoles) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need ASSIGN_ROLES permission")
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM member_roles WHERE guild_id = $1 AND user_id = $2 AND role_id = $3`,
		guildID, memberID, roleID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to remove role")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "role_not_assigned", "Member does not have this role")
		return
	}

	h.logAudit(r.Context(), guildID, userID, "member_role_remove", "role", roleID, nil)
	h.EventBus.PublishJSON(r.Context(), events.SubjectGuildMemberUpdate, "GUILD_MEMBER_UPDATE", map[string]string{
		"guild_id": guildID, "user_id": memberID, "role_id": roleID, "action": "role_remove",
	})

	w.WriteHeader(http.StatusNoContent)
}

// HandleGetMemberRoles returns the roles assigned to a guild member.
// GET /api/v1/guilds/{guildID}/members/{memberID}/roles
func (h *Handler) HandleGetMemberRoles(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")
	memberID := chi.URLParam(r, "memberID")

	rows, err := h.Pool.Query(r.Context(),
		`SELECT r.id, r.guild_id, r.name, r.color, r.hoist, r.mentionable, r.position,
		        r.permissions_allow, r.permissions_deny, r.created_at
		 FROM roles r
		 JOIN member_roles mr ON r.id = mr.role_id
		 WHERE mr.guild_id = $1 AND mr.user_id = $2
		 ORDER BY r.position DESC`,
		guildID, memberID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get member roles")
		return
	}
	defer rows.Close()

	roles := make([]models.Role, 0)
	for rows.Next() {
		var role models.Role
		if err := rows.Scan(
			&role.ID, &role.GuildID, &role.Name, &role.Color, &role.Hoist, &role.Mentionable,
			&role.Position, &role.PermissionsAllow, &role.PermissionsDeny, &role.CreatedAt,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read roles")
			return
		}
		roles = append(roles, role)
	}

	writeJSON(w, http.StatusOK, roles)
}

func (h *Handler) logAudit(ctx context.Context, guildID, actorID, action, targetType, targetID string, reason *string) {
	id := models.NewULID().String()
	h.Pool.Exec(ctx,
		`INSERT INTO audit_log (id, guild_id, actor_id, action, target_type, target_id, reason, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, now())`,
		id, guildID, actorID, action, targetType, targetID, reason,
	)
}

func generateInviteCode() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// HandleGetGuildPruneCount previews how many members would be pruned.
// GET /api/v1/guilds/{guildID}/prune?days=7
func (h *Handler) HandleGetGuildPruneCount(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.KickMembers) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need KICK_MEMBERS permission")
		return
	}

	days := 7
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, err := fmt.Sscanf(d, "%d", &days); err != nil || parsed != 1 || days < 1 || days > 30 {
			days = 7
		}
	}

	cutoff := time.Now().AddDate(0, 0, -days)
	var count int
	h.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM guild_members gm
		 JOIN users u ON u.id = gm.user_id
		 WHERE gm.guild_id = $1
		   AND gm.user_id != (SELECT owner_id FROM guilds WHERE id = $1)
		   AND NOT EXISTS (
		       SELECT 1 FROM messages m
		       JOIN channels c ON c.id = m.channel_id
		       WHERE m.author_id = gm.user_id AND c.guild_id = $1 AND m.created_at > $2
		   )`,
		guildID, cutoff,
	).Scan(&count)

	writeJSON(w, http.StatusOK, map[string]int{"pruned": count})
}

// HandleGuildPrune removes inactive members from a guild.
// POST /api/v1/guilds/{guildID}/prune?days=7
func (h *Handler) HandleGuildPrune(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.KickMembers) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need KICK_MEMBERS permission")
		return
	}

	days := 7
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, err := fmt.Sscanf(d, "%d", &days); err != nil || parsed != 1 || days < 1 || days > 30 {
			days = 7
		}
	}

	cutoff := time.Now().AddDate(0, 0, -days)
	result, err := h.Pool.Exec(r.Context(),
		`DELETE FROM guild_members
		 WHERE guild_id = $1
		   AND user_id != (SELECT owner_id FROM guilds WHERE id = $1)
		   AND NOT EXISTS (
		       SELECT 1 FROM messages m
		       JOIN channels c ON c.id = m.channel_id
		       WHERE m.author_id = guild_members.user_id AND c.guild_id = $1 AND m.created_at > $2
		   )`,
		guildID, cutoff,
	)
	if err != nil {
		h.Logger.Error("guild prune failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to prune members")
		return
	}

	pruned := int(result.RowsAffected())

	h.EventBus.PublishJSON(r.Context(), events.SubjectGuildMemberRemove, "GUILD_MEMBERS_PRUNE", map[string]interface{}{
		"guild_id": guildID,
		"pruned":   pruned,
	})

	writeJSON(w, http.StatusOK, map[string]int{"pruned": pruned})
}

// HandleCloneChannel clones a channel within a guild, copying its settings but
// not its messages. Optionally accepts a new name; defaults to "<original>-copy".
// POST /api/v1/guilds/{guildID}/channels/{channelID}/clone
func (h *Handler) HandleCloneChannel(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	channelID := chi.URLParam(r, "channelID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageChannels) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_CHANNELS permission")
		return
	}

	// Fetch the original channel.
	var orig models.Channel
	err := h.Pool.QueryRow(r.Context(),
		`SELECT id, guild_id, category_id, channel_type, name, topic, position,
		        slowmode_seconds, nsfw, encrypted, last_message_id, owner_id,
		        default_permissions, user_limit, bitrate, locked, locked_by, locked_at, archived, created_at
		 FROM channels WHERE id = $1 AND guild_id = $2`,
		channelID, guildID,
	).Scan(
		&orig.ID, &orig.GuildID, &orig.CategoryID, &orig.ChannelType, &orig.Name,
		&orig.Topic, &orig.Position, &orig.SlowmodeSeconds, &orig.NSFW, &orig.Encrypted,
		&orig.LastMessageID, &orig.OwnerID, &orig.DefaultPermissions,
		&orig.UserLimit, &orig.Bitrate,
		&orig.Locked, &orig.LockedBy, &orig.LockedAt, &orig.Archived, &orig.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "channel_not_found", "Channel not found in this guild")
			return
		}
		h.Logger.Error("failed to fetch channel for clone", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to fetch channel")
		return
	}

	// Parse optional name from request body.
	var req struct {
		Name *string `json:"name"`
	}
	// Body is optional; ignore decode errors for empty body.
	json.NewDecoder(r.Body).Decode(&req)

	newName := ""
	if req.Name != nil && *req.Name != "" {
		newName = *req.Name
	} else if orig.Name != nil {
		newName = *orig.Name + "-copy"
	} else {
		newName = "cloned-channel"
	}

	if len(newName) > 100 {
		newName = newName[:100]
	}

	newID := models.NewULID().String()

	// Copy permission overwrites from the original channel.
	type permOverwrite struct {
		TargetType       string
		TargetID         string
		PermissionsAllow int64
		PermissionsDeny  int64
	}
	var overwrites []permOverwrite
	permRows, permErr := h.Pool.Query(r.Context(),
		`SELECT target_type, target_id, permissions_allow, permissions_deny
		 FROM channel_permissions WHERE channel_id = $1`,
		channelID,
	)
	if permErr == nil {
		for permRows.Next() {
			var po permOverwrite
			if scanErr := permRows.Scan(&po.TargetType, &po.TargetID, &po.PermissionsAllow, &po.PermissionsDeny); scanErr == nil {
				overwrites = append(overwrites, po)
			}
		}
		permRows.Close()
	}

	// Insert the cloned channel.
	var cloned models.Channel
	err = h.Pool.QueryRow(r.Context(),
		`INSERT INTO channels (id, guild_id, category_id, channel_type, name, topic, position,
		                       slowmode_seconds, nsfw, encrypted, user_limit, bitrate, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, false, $10, $11, now())
		 RETURNING id, guild_id, category_id, channel_type, name, topic, position,
		           slowmode_seconds, nsfw, encrypted, last_message_id, owner_id,
		           default_permissions, user_limit, bitrate, locked, locked_by, locked_at, archived, created_at`,
		newID, guildID, orig.CategoryID, orig.ChannelType, newName, orig.Topic,
		orig.Position+1, orig.SlowmodeSeconds, orig.NSFW, orig.UserLimit, orig.Bitrate,
	).Scan(
		&cloned.ID, &cloned.GuildID, &cloned.CategoryID, &cloned.ChannelType, &cloned.Name,
		&cloned.Topic, &cloned.Position, &cloned.SlowmodeSeconds, &cloned.NSFW, &cloned.Encrypted,
		&cloned.LastMessageID, &cloned.OwnerID, &cloned.DefaultPermissions,
		&cloned.UserLimit, &cloned.Bitrate,
		&cloned.Locked, &cloned.LockedBy, &cloned.LockedAt, &cloned.Archived, &cloned.CreatedAt,
	)
	if err != nil {
		h.Logger.Error("failed to clone channel", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to clone channel")
		return
	}

	// Insert permission overwrites for the new channel (after channel exists).
	for _, po := range overwrites {
		h.Pool.Exec(r.Context(),
			`INSERT INTO channel_permissions (channel_id, target_type, target_id, permissions_allow, permissions_deny)
			 VALUES ($1, $2, $3, $4, $5)`,
			newID, po.TargetType, po.TargetID, po.PermissionsAllow, po.PermissionsDeny,
		)
	}

	h.logAudit(r.Context(), guildID, userID, "channel_clone", "channel", newID, nil)
	h.EventBus.PublishJSON(r.Context(), events.SubjectChannelCreate, "CHANNEL_CREATE", cloned)

	writeJSON(w, http.StatusCreated, cloned)
}

// --- Server Guide ---

// GuideStep represents a single step in a guild's server guide.
type GuideStep struct {
	ID        string  `json:"id"`
	GuildID   string  `json:"guild_id"`
	Title     string  `json:"title"`
	Content   string  `json:"content"`
	Position  int     `json:"position"`
	ChannelID *string `json:"channel_id,omitempty"`
	CreatedAt string  `json:"created_at"`
}

type updateServerGuideRequest struct {
	Steps []struct {
		Title     string  `json:"title"`
		Content   string  `json:"content"`
		Position  int     `json:"position"`
		ChannelID *string `json:"channel_id,omitempty"`
	} `json:"steps"`
}

// HandleGetServerGuide returns the server guide steps for a guild.
// GET /api/v1/guilds/{guildID}/guide
func (h *Handler) HandleGetServerGuide(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isMember(r.Context(), guildID, userID) {
		writeError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, guild_id, title, content, position, channel_id, created_at
		 FROM guild_guides
		 WHERE guild_id = $1
		 ORDER BY position ASC, created_at ASC`,
		guildID,
	)
	if err != nil {
		h.Logger.Error("failed to get server guide", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get server guide")
		return
	}
	defer rows.Close()

	steps := make([]GuideStep, 0)
	for rows.Next() {
		var s GuideStep
		if err := rows.Scan(&s.ID, &s.GuildID, &s.Title, &s.Content, &s.Position, &s.ChannelID, &s.CreatedAt); err != nil {
			h.Logger.Error("failed to scan guide step", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read server guide")
			return
		}
		steps = append(steps, s)
	}
	if err := rows.Err(); err != nil {
		h.Logger.Error("error iterating guide steps", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read server guide")
		return
	}

	writeJSON(w, http.StatusOK, steps)
}

// HandleUpdateServerGuide replaces the entire server guide for a guild.
// Requires MANAGE_GUILD permission.
// PUT /api/v1/guilds/{guildID}/guide
func (h *Handler) HandleUpdateServerGuide(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageGuild) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_GUILD permission")
		return
	}

	var req updateServerGuideRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if len(req.Steps) > 20 {
		writeError(w, http.StatusBadRequest, "too_many_steps", "Server guide can have at most 20 steps")
		return
	}

	for i, step := range req.Steps {
		if step.Title == "" || len(step.Title) > 100 {
			writeError(w, http.StatusBadRequest, "invalid_title", fmt.Sprintf("Step %d title must be 1-100 characters", i+1))
			return
		}
		if step.Content == "" || len(step.Content) > 4000 {
			writeError(w, http.StatusBadRequest, "invalid_content", fmt.Sprintf("Step %d content must be 1-4000 characters", i+1))
			return
		}
	}

	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update server guide")
		return
	}
	defer tx.Rollback(r.Context())

	// Delete all existing guide steps for this guild.
	_, err = tx.Exec(r.Context(), `DELETE FROM guild_guides WHERE guild_id = $1`, guildID)
	if err != nil {
		h.Logger.Error("failed to clear server guide", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update server guide")
		return
	}

	// Insert new guide steps.
	steps := make([]GuideStep, 0, len(req.Steps))
	for i, step := range req.Steps {
		stepID := models.NewULID().String()
		var s GuideStep
		err := tx.QueryRow(r.Context(),
			`INSERT INTO guild_guides (id, guild_id, title, content, position, channel_id, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, now())
			 RETURNING id, guild_id, title, content, position, channel_id, created_at`,
			stepID, guildID, step.Title, step.Content, i, step.ChannelID,
		).Scan(&s.ID, &s.GuildID, &s.Title, &s.Content, &s.Position, &s.ChannelID, &s.CreatedAt)
		if err != nil {
			h.Logger.Error("failed to insert guide step", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update server guide")
			return
		}
		steps = append(steps, s)
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update server guide")
		return
	}

	h.logAudit(r.Context(), guildID, userID, "guide_update", "guild", guildID, nil)

	writeJSON(w, http.StatusOK, steps)
}

// --- Bump System ---

// GuildBump represents a single bump action for a guild.
type GuildBump struct {
	ID       string `json:"id"`
	GuildID  string `json:"guild_id"`
	BumpedBy string `json:"bumped_by"`
	BumpedAt string `json:"bumped_at"`
}

// BumpResponse is returned after a successful bump.
type BumpResponse struct {
	Success     bool   `json:"success"`
	NextBumpAt  string `json:"next_bump_at"`
	BumpMessage string `json:"bump_message"`
}

// HandleBumpGuild bumps a guild in the discovery listing.
// Rate limited to once per 2 hours per guild.
// POST /api/v1/guilds/{guildID}/bump
func (h *Handler) HandleBumpGuild(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isMember(r.Context(), guildID, userID) {
		writeError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	// Check if the guild is discoverable; only discoverable guilds can be bumped.
	var discoverable bool
	err := h.Pool.QueryRow(r.Context(),
		`SELECT discoverable FROM guilds WHERE id = $1`, guildID,
	).Scan(&discoverable)
	if err != nil {
		writeError(w, http.StatusNotFound, "guild_not_found", "Guild not found")
		return
	}
	if !discoverable {
		writeError(w, http.StatusBadRequest, "not_discoverable", "Only discoverable guilds can be bumped")
		return
	}

	// Rate limit: check if the guild was bumped in the last 2 hours.
	var lastBump *time.Time
	err = h.Pool.QueryRow(r.Context(),
		`SELECT MAX(bumped_at) FROM guild_bumps WHERE guild_id = $1`,
		guildID,
	).Scan(&lastBump)
	if err != nil && err != pgx.ErrNoRows {
		h.Logger.Error("failed to check bump cooldown", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to bump guild")
		return
	}

	cooldown := 2 * time.Hour
	if lastBump != nil && time.Since(*lastBump) < cooldown {
		nextBump := lastBump.Add(cooldown)
		remaining := time.Until(nextBump)
		minutes := int(remaining.Minutes())
		writeError(w, http.StatusTooManyRequests, "bump_cooldown",
			fmt.Sprintf("Guild was recently bumped. Try again in %d minutes.", minutes))
		return
	}

	// Record the bump.
	bumpID := models.NewULID().String()
	now := time.Now()
	_, err = h.Pool.Exec(r.Context(),
		`INSERT INTO guild_bumps (id, guild_id, bumped_by, bumped_at) VALUES ($1, $2, $3, $4)`,
		bumpID, guildID, userID, now,
	)
	if err != nil {
		h.Logger.Error("failed to record guild bump", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to bump guild")
		return
	}

	nextBumpAt := now.Add(cooldown)

	writeJSON(w, http.StatusOK, BumpResponse{
		Success:     true,
		NextBumpAt:  nextBumpAt.Format(time.RFC3339),
		BumpMessage: "Guild bumped! It will appear higher in discovery.",
	})
}

// HandleGetBumpStatus returns the current bump cooldown status for a guild.
// GET /api/v1/guilds/{guildID}/bump
func (h *Handler) HandleGetBumpStatus(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isMember(r.Context(), guildID, userID) {
		writeError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	var lastBump *time.Time
	err := h.Pool.QueryRow(r.Context(),
		`SELECT MAX(bumped_at) FROM guild_bumps WHERE guild_id = $1`,
		guildID,
	).Scan(&lastBump)
	if err != nil && err != pgx.ErrNoRows {
		h.Logger.Error("failed to get bump status", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get bump status")
		return
	}

	// Count total bumps in last 24 hours for display.
	var bumpCount int
	h.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM guild_bumps WHERE guild_id = $1 AND bumped_at > now() - INTERVAL '24 hours'`,
		guildID,
	).Scan(&bumpCount)

	cooldown := 2 * time.Hour
	canBump := true
	var nextBumpAt *string

	if lastBump != nil && time.Since(*lastBump) < cooldown {
		canBump = false
		t := lastBump.Add(cooldown).Format(time.RFC3339)
		nextBumpAt = &t
	}

	type bumpStatus struct {
		CanBump    bool    `json:"can_bump"`
		NextBumpAt *string `json:"next_bump_at,omitempty"`
		LastBump   *string `json:"last_bump,omitempty"`
		BumpCount  int     `json:"bump_count_24h"`
	}

	var lastBumpStr *string
	if lastBump != nil {
		s := lastBump.Format(time.RFC3339)
		lastBumpStr = &s
	}

	writeJSON(w, http.StatusOK, bumpStatus{
		CanBump:    canBump,
		NextBumpAt: nextBumpAt,
		LastBump:   lastBumpStr,
		BumpCount:  bumpCount,
	})
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
