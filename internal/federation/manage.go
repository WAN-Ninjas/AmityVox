package federation

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/models"
	"github.com/amityvox/amityvox/internal/permissions"
)

// flexInt64 unmarshals from both JSON numbers and strings, so JavaScript
// clients can safely send int64 permission values (bit 63 overflows Number).
type flexInt64 struct {
	Value int64
	Set   bool
}

func (f *flexInt64) UnmarshalJSON(data []byte) error {
	f.Set = true
	s := string(data)
	if s == "null" {
		f.Set = false
		return nil
	}
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	if len(s) > 0 && s[0] == '-' {
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid permission value: %s", string(data))
		}
		f.Value = v
		return nil
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid permission value: %s", string(data))
	}
	f.Value = int64(v)
	return nil
}

func (f *flexInt64) Int64Ptr() *int64 {
	if !f.Set {
		return nil
	}
	return &f.Value
}

// manageRequest is the inner payload of a signed management request.
type manageRequest struct {
	Action string          `json:"action"`  // e.g. "guild_update", "channel_create", etc.
	UserID string          `json:"user_id"` // The remote user performing the action
	Data   json.RawMessage `json:"data"`    // Action-specific payload
}

// manageResponse is returned to the requesting instance.
type manageResponse struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

// HandleManage processes remote guild management requests.
// POST /federation/v1/guilds/{guildID}/manage
func (ss *SyncService) HandleManage(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")
	if guildID == "" {
		http.Error(w, "Missing guild ID", http.StatusBadRequest)
		return
	}

	// 1. Verify signed request (reuse verifyFederationRequest from dm.go).
	signed, senderID, ok := ss.verifyFederationRequest(w, r)
	if !ok {
		return
	}

	// 2. Verify this guild actually belongs to this instance (not federated).
	var ownerInstanceID *string
	err := ss.fed.pool.QueryRow(r.Context(),
		`SELECT instance_id FROM guilds WHERE id = $1`, guildID,
	).Scan(&ownerInstanceID)
	if err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, "Guild not found", http.StatusNotFound)
		} else {
			ss.logger.Error("manage: guild lookup failed", slog.String("error", err.Error()))
			http.Error(w, "Internal error", http.StatusInternalServerError)
		}
		return
	}
	// Only process if guild is local (instance_id IS NULL).
	if ownerInstanceID != nil {
		http.Error(w, "Guild is not owned by this instance", http.StatusForbidden)
		return
	}

	// 3. Parse the management request from signed payload.
	var req manageRequest
	if err := json.Unmarshal(signed.Payload, &req); err != nil {
		http.Error(w, "Invalid management request", http.StatusBadRequest)
		return
	}

	ss.logger.Info("federation manage request",
		slog.String("sender", senderID),
		slog.String("guild_id", guildID),
		slog.String("action", req.Action),
		slog.String("user_id", req.UserID))

	// 4. Verify the user belongs to the sender instance.
	// Skip for member_join — the user stub doesn't exist yet on the home instance.
	if req.Action != "member_join" {
		if !ss.validateSenderUser(r.Context(), w, senderID, req.UserID) {
			return
		}
	}

	// 5. Execute the action.
	ctx := r.Context()
	switch req.Action {
	case "guild_update":
		ss.manageGuildUpdate(ctx, w, guildID, req.UserID, req.Data)
	case "guild_delete":
		ss.manageGuildDelete(ctx, w, guildID, req.UserID)
	case "channel_create":
		ss.manageChannelCreate(ctx, w, guildID, req.UserID, req.Data)
	case "channel_update":
		ss.manageChannelUpdate(ctx, w, guildID, req.UserID, req.Data)
	case "channel_delete":
		ss.manageChannelDelete(ctx, w, guildID, req.UserID, req.Data)
	case "role_create":
		ss.manageRoleCreate(ctx, w, guildID, req.UserID, req.Data)
	case "role_update":
		ss.manageRoleUpdate(ctx, w, guildID, req.UserID, req.Data)
	case "role_delete":
		ss.manageRoleDelete(ctx, w, guildID, req.UserID, req.Data)
	case "member_update":
		ss.manageMemberUpdate(ctx, w, guildID, req.UserID, req.Data)
	case "member_remove":
		ss.manageMemberRemove(ctx, w, guildID, req.UserID, req.Data)
	case "member_ban":
		ss.manageMemberBan(ctx, w, guildID, req.UserID, req.Data)
	case "member_unban":
		ss.manageMemberUnban(ctx, w, guildID, req.UserID, req.Data)
	case "category_create":
		ss.manageCategoryCreate(ctx, w, guildID, req.UserID, req.Data)
	case "category_update":
		ss.manageCategoryUpdate(ctx, w, guildID, req.UserID, req.Data)
	case "category_delete":
		ss.manageCategoryDelete(ctx, w, guildID, req.UserID, req.Data)
	case "message_delete":
		ss.manageMessageDelete(ctx, w, guildID, req.UserID, req.Data)
	case "message_pin":
		ss.manageMessagePin(ctx, w, guildID, req.UserID, req.Data)
	case "message_unpin":
		ss.manageMessageUnpin(ctx, w, guildID, req.UserID, req.Data)
	case "member_role_remove":
		ss.manageMemberRoleRemove(ctx, w, guildID, req.UserID, req.Data)
	case "member_join":
		ss.manageMemberJoin(ctx, w, guildID, senderID, req.UserID, req.Data)
	default:
		writeManageError(w, http.StatusBadRequest, "Unknown action: "+req.Action)
	}
}

// --- Permission helpers ---

// hasManageGuildPermission checks if a remote user has the specified permission
// in a guild, mirroring the logic in the guilds handler.
func (ss *SyncService) hasManageGuildPermission(ctx context.Context, guildID, userID string, perm uint64) bool {
	// Owner has all permissions.
	var ownerID string
	if err := ss.fed.pool.QueryRow(ctx, `SELECT owner_id FROM guilds WHERE id = $1`, guildID).Scan(&ownerID); err != nil {
		return false
	}
	if userID == ownerID {
		return true
	}

	// Get guild default permissions.
	var defaultPerms int64
	ss.fed.pool.QueryRow(ctx, `SELECT default_permissions FROM guilds WHERE id = $1`, guildID).Scan(&defaultPerms)
	computedPerms := uint64(defaultPerms)

	// Apply member's role permissions.
	rows, _ := ss.fed.pool.Query(ctx,
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

	return permissions.HasPermission(computedPerms, perm)
}

// --- Response helpers ---

func writeManageOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	var raw json.RawMessage
	if data != nil {
		b, err := json.Marshal(data)
		if err == nil {
			raw = b
		}
	}
	json.NewEncoder(w).Encode(manageResponse{OK: true, Data: raw})
}

func writeManageError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(manageResponse{OK: false, Error: msg})
}

// --- Action implementations ---

func (ss *SyncService) manageGuildUpdate(ctx context.Context, w http.ResponseWriter, guildID, userID string, data json.RawMessage) {
	if !ss.hasManageGuildPermission(ctx, guildID, userID, permissions.ManageGuild) {
		writeManageError(w, http.StatusForbidden, "Missing MANAGE_GUILD permission")
		return
	}

	var req struct {
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
	if err := json.Unmarshal(data, &req); err != nil {
		writeManageError(w, http.StatusBadRequest, "Invalid guild_update data")
		return
	}

	var tagsArg interface{} = nil
	if req.Tags != nil {
		tagsArg = req.Tags
	}

	var guild models.Guild
	err := ss.fed.pool.QueryRow(ctx,
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
		guildID, req.Name, req.Description, req.IconID, req.BannerID, req.NSFW,
		req.Discoverable, req.VerificationLevel, req.AFKChannelID, req.AFKTimeout, tagsArg,
	).Scan(
		&guild.ID, &guild.InstanceID, &guild.OwnerID, &guild.Name, &guild.Description,
		&guild.IconID, &guild.BannerID, &guild.DefaultPermissions, &guild.Flags,
		&guild.NSFW, &guild.Discoverable, &guild.PreferredLocale, &guild.MaxMembers,
		&guild.VanityURL, &guild.VerificationLevel, &guild.AFKChannelID, &guild.AFKTimeout,
		&guild.Tags, &guild.MemberCount, &guild.CreatedAt,
	)
	if err != nil {
		ss.logger.Error("manage guild_update: DB error", slog.String("error", err.Error()))
		writeManageError(w, http.StatusInternalServerError, "Failed to update guild")
		return
	}

	ss.bus.PublishGuildEvent(ctx, events.SubjectGuildUpdate, "GUILD_UPDATE", guildID, guild)
	writeManageOK(w, guild)
}

func (ss *SyncService) manageGuildDelete(ctx context.Context, w http.ResponseWriter, guildID, userID string) {
	// Only the owner can delete a guild.
	var ownerID string
	if err := ss.fed.pool.QueryRow(ctx, `SELECT owner_id FROM guilds WHERE id = $1`, guildID).Scan(&ownerID); err != nil {
		writeManageError(w, http.StatusNotFound, "Guild not found")
		return
	}
	if ownerID != userID {
		writeManageError(w, http.StatusForbidden, "Only the guild owner can delete the guild")
		return
	}

	if _, err := ss.fed.pool.Exec(ctx, `DELETE FROM guilds WHERE id = $1`, guildID); err != nil {
		ss.logger.Error("manage guild_delete: DB error", slog.String("error", err.Error()))
		writeManageError(w, http.StatusInternalServerError, "Failed to delete guild")
		return
	}

	ss.bus.PublishGuildEvent(ctx, events.SubjectGuildDelete, "GUILD_DELETE", guildID, map[string]string{"id": guildID})
	w.WriteHeader(http.StatusNoContent)
}

func (ss *SyncService) manageChannelCreate(ctx context.Context, w http.ResponseWriter, guildID, userID string, data json.RawMessage) {
	if !ss.hasManageGuildPermission(ctx, guildID, userID, permissions.ManageChannels) {
		writeManageError(w, http.StatusForbidden, "Missing MANAGE_CHANNELS permission")
		return
	}

	var req struct {
		Name        string  `json:"name"`
		ChannelType string  `json:"channel_type"`
		CategoryID  *string `json:"category_id"`
		Topic       *string `json:"topic"`
		Position    *int    `json:"position"`
		NSFW        *bool   `json:"nsfw"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		writeManageError(w, http.StatusBadRequest, "Invalid channel_create data")
		return
	}

	if req.Name == "" || len(req.Name) > 100 {
		writeManageError(w, http.StatusBadRequest, "Channel name must be 1-100 characters")
		return
	}

	validTypes := map[string]bool{
		"text": true, "voice": true, "announcement": true, "forum": true, "gallery": true, "stage": true,
	}
	if !validTypes[req.ChannelType] {
		writeManageError(w, http.StatusBadRequest, "Invalid channel type")
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
	err := ss.fed.pool.QueryRow(ctx,
		`INSERT INTO channels (id, guild_id, category_id, channel_type, name, topic, position, nsfw, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
		 RETURNING id, guild_id, category_id, channel_type, name, topic, position,
		           slowmode_seconds, nsfw, encrypted, last_message_id, owner_id,
		           default_permissions, user_limit, bitrate, locked, locked_by, locked_at,
		           archived, parent_channel_id, last_activity_at, created_at`,
		channelID, guildID, req.CategoryID, req.ChannelType, req.Name, req.Topic, position, nsfw,
	).Scan(
		&channel.ID, &channel.GuildID, &channel.CategoryID, &channel.ChannelType, &channel.Name,
		&channel.Topic, &channel.Position, &channel.SlowmodeSeconds, &channel.NSFW, &channel.Encrypted,
		&channel.LastMessageID, &channel.OwnerID, &channel.DefaultPermissions,
		&channel.UserLimit, &channel.Bitrate,
		&channel.Locked, &channel.LockedBy, &channel.LockedAt, &channel.Archived,
		&channel.ParentChannelID, &channel.LastActivityAt, &channel.CreatedAt,
	)
	if err != nil {
		ss.logger.Error("manage channel_create: DB error", slog.String("error", err.Error()))
		writeManageError(w, http.StatusInternalServerError, "Failed to create channel")
		return
	}

	channelData, _ := json.Marshal(channel)
	ss.bus.Publish(ctx, events.SubjectChannelCreate, events.Event{
		Type:    "CHANNEL_CREATE",
		GuildID: guildID,
		Data:    channelData,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(manageResponse{OK: true, Data: channelData})
}

func (ss *SyncService) manageChannelUpdate(ctx context.Context, w http.ResponseWriter, guildID, userID string, data json.RawMessage) {
	if !ss.hasManageGuildPermission(ctx, guildID, userID, permissions.ManageChannels) {
		writeManageError(w, http.StatusForbidden, "Missing MANAGE_CHANNELS permission")
		return
	}

	var req struct {
		ChannelID                  string   `json:"channel_id"`
		Name                       *string  `json:"name"`
		Topic                      *string  `json:"topic"`
		Position                   *int     `json:"position"`
		NSFW                       *bool    `json:"nsfw"`
		SlowmodeSeconds            *int     `json:"slowmode_seconds"`
		UserLimit                  *int     `json:"user_limit"`
		Bitrate                    *int     `json:"bitrate"`
		Archived                   *bool    `json:"archived"`
		Encrypted                  *bool    `json:"encrypted"`
		ReadOnly                   *bool    `json:"read_only"`
		ReadOnlyRoleIDs            []string `json:"read_only_role_ids"`
		DefaultAutoArchiveDuration *int     `json:"default_auto_archive_duration"`
		ForumDefaultSort           *string  `json:"forum_default_sort"`
		ForumPostGuidelines        *string  `json:"forum_post_guidelines"`
		ForumRequireTags           *bool    `json:"forum_require_tags"`
		GalleryDefaultSort         *string  `json:"gallery_default_sort"`
		GalleryPostGuidelines      *string  `json:"gallery_post_guidelines"`
		GalleryRequireTags         *bool    `json:"gallery_require_tags"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		writeManageError(w, http.StatusBadRequest, "Invalid channel_update data")
		return
	}

	// Use channel_id from data, or fall back to "id" field.
	channelID := req.ChannelID
	if channelID == "" {
		var idFallback struct{ ID string `json:"id"` }
		json.Unmarshal(data, &idFallback)
		channelID = idFallback.ID
	}
	if channelID == "" {
		writeManageError(w, http.StatusBadRequest, "Missing channel_id")
		return
	}

	// Verify channel belongs to this guild.
	var chGuildID *string
	if err := ss.fed.pool.QueryRow(ctx, `SELECT guild_id FROM channels WHERE id = $1`, channelID).Scan(&chGuildID); err != nil {
		writeManageError(w, http.StatusNotFound, "Channel not found")
		return
	}
	if chGuildID == nil || *chGuildID != guildID {
		writeManageError(w, http.StatusForbidden, "Channel does not belong to this guild")
		return
	}

	var channel models.Channel
	err := ss.fed.pool.QueryRow(ctx,
		`UPDATE channels SET
			name = COALESCE($2, name),
			topic = COALESCE($3, topic),
			position = COALESCE($4, position),
			nsfw = COALESCE($5, nsfw),
			slowmode_seconds = COALESCE($6, slowmode_seconds),
			user_limit = COALESCE($7, user_limit),
			bitrate = COALESCE($8, bitrate),
			archived = COALESCE($9, archived),
			encrypted = COALESCE($10, encrypted),
			read_only = COALESCE($11, read_only),
			read_only_role_ids = COALESCE($12, read_only_role_ids),
			default_auto_archive_duration = COALESCE($13, default_auto_archive_duration),
			forum_default_sort = COALESCE($14, forum_default_sort),
			forum_post_guidelines = COALESCE($15, forum_post_guidelines),
			forum_require_tags = COALESCE($16, forum_require_tags),
			gallery_default_sort = COALESCE($17, gallery_default_sort),
			gallery_post_guidelines = COALESCE($18, gallery_post_guidelines),
			gallery_require_tags = COALESCE($19, gallery_require_tags)
		 WHERE id = $1
		 RETURNING id, guild_id, category_id, channel_type, name, topic, position,
		           slowmode_seconds, nsfw, encrypted, last_message_id, owner_id,
		           default_permissions, user_limit, bitrate, locked, locked_by, locked_at,
		           archived, read_only, read_only_role_ids, default_auto_archive_duration,
		           forum_default_sort, forum_post_guidelines, forum_require_tags,
		           gallery_default_sort, gallery_post_guidelines, gallery_require_tags,
		           parent_channel_id, last_activity_at, created_at`,
		channelID, req.Name, req.Topic, req.Position, req.NSFW, req.SlowmodeSeconds,
		req.UserLimit, req.Bitrate, req.Archived, req.Encrypted, req.ReadOnly, req.ReadOnlyRoleIDs,
		req.DefaultAutoArchiveDuration,
		req.ForumDefaultSort, req.ForumPostGuidelines, req.ForumRequireTags,
		req.GalleryDefaultSort, req.GalleryPostGuidelines, req.GalleryRequireTags,
	).Scan(
		&channel.ID, &channel.GuildID, &channel.CategoryID, &channel.ChannelType, &channel.Name,
		&channel.Topic, &channel.Position, &channel.SlowmodeSeconds, &channel.NSFW, &channel.Encrypted,
		&channel.LastMessageID, &channel.OwnerID, &channel.DefaultPermissions,
		&channel.UserLimit, &channel.Bitrate,
		&channel.Locked, &channel.LockedBy, &channel.LockedAt,
		&channel.Archived, &channel.ReadOnly, &channel.ReadOnlyRoleIDs,
		&channel.DefaultAutoArchiveDuration,
		&channel.ForumDefaultSort, &channel.ForumPostGuidelines, &channel.ForumRequireTags,
		&channel.GalleryDefaultSort, &channel.GalleryPostGuidelines, &channel.GalleryRequireTags,
		&channel.ParentChannelID, &channel.LastActivityAt, &channel.CreatedAt,
	)
	if err != nil {
		ss.logger.Error("manage channel_update: DB error", slog.String("error", err.Error()))
		writeManageError(w, http.StatusInternalServerError, "Failed to update channel")
		return
	}

	channelData, _ := json.Marshal(channel)
	ss.bus.Publish(ctx, events.SubjectChannelUpdate, events.Event{
		Type:      "CHANNEL_UPDATE",
		GuildID:   guildID,
		ChannelID: channelID,
		Data:      channelData,
	})
	writeManageOK(w, channel)
}

func (ss *SyncService) manageChannelDelete(ctx context.Context, w http.ResponseWriter, guildID, userID string, data json.RawMessage) {
	if !ss.hasManageGuildPermission(ctx, guildID, userID, permissions.ManageChannels) {
		writeManageError(w, http.StatusForbidden, "Missing MANAGE_CHANNELS permission")
		return
	}

	var req struct {
		ChannelID string `json:"channel_id"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		writeManageError(w, http.StatusBadRequest, "Invalid channel_delete data")
		return
	}
	if req.ChannelID == "" {
		var idFallback struct{ ID string `json:"id"` }
		json.Unmarshal(data, &idFallback)
		req.ChannelID = idFallback.ID
	}
	if req.ChannelID == "" {
		writeManageError(w, http.StatusBadRequest, "Missing channel_id")
		return
	}

	// Verify channel belongs to this guild.
	var chGuildID *string
	if err := ss.fed.pool.QueryRow(ctx, `SELECT guild_id FROM channels WHERE id = $1`, req.ChannelID).Scan(&chGuildID); err != nil {
		writeManageError(w, http.StatusNotFound, "Channel not found")
		return
	}
	if chGuildID == nil || *chGuildID != guildID {
		writeManageError(w, http.StatusForbidden, "Channel does not belong to this guild")
		return
	}

	tag, err := ss.fed.pool.Exec(ctx, `DELETE FROM channels WHERE id = $1 AND guild_id = $2`, req.ChannelID, guildID)
	if err != nil {
		ss.logger.Error("manage channel_delete: DB error", slog.String("error", err.Error()))
		writeManageError(w, http.StatusInternalServerError, "Failed to delete channel")
		return
	}
	if tag.RowsAffected() == 0 {
		writeManageError(w, http.StatusNotFound, "Channel not found")
		return
	}

	ss.bus.Publish(ctx, events.SubjectChannelDelete, events.Event{
		Type:    "CHANNEL_DELETE",
		GuildID: guildID,
		Data:    mustMarshalManage(map[string]string{"id": req.ChannelID, "guild_id": guildID}),
	})
	w.WriteHeader(http.StatusNoContent)
}

func (ss *SyncService) manageRoleCreate(ctx context.Context, w http.ResponseWriter, guildID, userID string, data json.RawMessage) {
	if !ss.hasManageGuildPermission(ctx, guildID, userID, permissions.ManageRoles) {
		writeManageError(w, http.StatusForbidden, "Missing MANAGE_ROLES permission")
		return
	}

	var req struct {
		Name             string    `json:"name"`
		Color            *string   `json:"color"`
		Hoist            *bool     `json:"hoist"`
		Mentionable      *bool     `json:"mentionable"`
		Position         *int      `json:"position"`
		PermissionsAllow flexInt64 `json:"permissions_allow"`
		PermissionsDeny  flexInt64 `json:"permissions_deny"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		writeManageError(w, http.StatusBadRequest, "Invalid role_create data")
		return
	}

	if req.Name == "" || len(req.Name) > 100 {
		writeManageError(w, http.StatusBadRequest, "Role name must be 1-100 characters")
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
	} else {
		var maxPos int
		ss.fed.pool.QueryRow(ctx,
			`SELECT COALESCE(MAX(position), 0) FROM roles WHERE guild_id = $1`, guildID,
		).Scan(&maxPos)
		position = maxPos + 1
	}

	var permAllow, permDeny int64
	if req.PermissionsAllow.Set {
		permAllow = req.PermissionsAllow.Value
	}
	if req.PermissionsDeny.Set {
		permDeny = req.PermissionsDeny.Value
	}

	var role models.Role
	err := ss.fed.pool.QueryRow(ctx,
		`INSERT INTO roles (id, guild_id, name, color, hoist, mentionable, position, permissions_allow, permissions_deny, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
		 RETURNING id, guild_id, name, color, hoist, mentionable, position, permissions_allow, permissions_deny, created_at`,
		roleID, guildID, req.Name, req.Color, hoist, mentionable, position, permAllow, permDeny,
	).Scan(
		&role.ID, &role.GuildID, &role.Name, &role.Color, &role.Hoist, &role.Mentionable,
		&role.Position, &role.PermissionsAllow, &role.PermissionsDeny, &role.CreatedAt,
	)
	if err != nil {
		ss.logger.Error("manage role_create: DB error", slog.String("error", err.Error()))
		writeManageError(w, http.StatusInternalServerError, "Failed to create role")
		return
	}

	ss.bus.PublishGuildEvent(ctx, events.SubjectGuildRoleCreate, "GUILD_ROLE_CREATE", guildID, role)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	roleData, _ := json.Marshal(role)
	json.NewEncoder(w).Encode(manageResponse{OK: true, Data: roleData})
}

func (ss *SyncService) manageRoleUpdate(ctx context.Context, w http.ResponseWriter, guildID, userID string, data json.RawMessage) {
	if !ss.hasManageGuildPermission(ctx, guildID, userID, permissions.ManageRoles) {
		writeManageError(w, http.StatusForbidden, "Missing MANAGE_ROLES permission")
		return
	}

	var req struct {
		RoleID           string    `json:"role_id"`
		Name             *string   `json:"name"`
		Color            *string   `json:"color"`
		Hoist            *bool     `json:"hoist"`
		Mentionable      *bool     `json:"mentionable"`
		Position         *int      `json:"position"`
		PermissionsAllow flexInt64 `json:"permissions_allow"`
		PermissionsDeny  flexInt64 `json:"permissions_deny"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		writeManageError(w, http.StatusBadRequest, "Invalid role_update data")
		return
	}
	if req.RoleID == "" {
		var idFallback struct{ ID string `json:"id"` }
		json.Unmarshal(data, &idFallback)
		req.RoleID = idFallback.ID
	}
	if req.RoleID == "" {
		writeManageError(w, http.StatusBadRequest, "Missing role_id")
		return
	}

	var role models.Role
	err := ss.fed.pool.QueryRow(ctx,
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
		req.RoleID, guildID, req.Name, req.Color, req.Hoist, req.Mentionable, req.Position,
		req.PermissionsAllow.Int64Ptr(), req.PermissionsDeny.Int64Ptr(),
	).Scan(
		&role.ID, &role.GuildID, &role.Name, &role.Color, &role.Hoist, &role.Mentionable,
		&role.Position, &role.PermissionsAllow, &role.PermissionsDeny, &role.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeManageError(w, http.StatusNotFound, "Role not found")
			return
		}
		ss.logger.Error("manage role_update: DB error", slog.String("error", err.Error()))
		writeManageError(w, http.StatusInternalServerError, "Failed to update role")
		return
	}

	ss.bus.PublishGuildEvent(ctx, events.SubjectGuildRoleUpdate, "GUILD_ROLE_UPDATE", guildID, role)
	writeManageOK(w, role)
}

func (ss *SyncService) manageRoleDelete(ctx context.Context, w http.ResponseWriter, guildID, userID string, data json.RawMessage) {
	if !ss.hasManageGuildPermission(ctx, guildID, userID, permissions.ManageRoles) {
		writeManageError(w, http.StatusForbidden, "Missing MANAGE_ROLES permission")
		return
	}

	var req struct {
		RoleID string `json:"role_id"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		writeManageError(w, http.StatusBadRequest, "Invalid role_delete data")
		return
	}
	if req.RoleID == "" {
		writeManageError(w, http.StatusBadRequest, "Missing role_id")
		return
	}

	// Block deletion of @everyone role.
	var roleName string
	var rolePos int
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT name, position FROM roles WHERE id = $1 AND guild_id = $2`, req.RoleID, guildID,
	).Scan(&roleName, &rolePos); err != nil {
		writeManageError(w, http.StatusNotFound, "Role not found")
		return
	}
	if roleName == "@everyone" && rolePos == 0 {
		writeManageError(w, http.StatusForbidden, "The @everyone role cannot be deleted")
		return
	}

	tag, err := ss.fed.pool.Exec(ctx, `DELETE FROM roles WHERE id = $1 AND guild_id = $2`, req.RoleID, guildID)
	if err != nil {
		ss.logger.Error("manage role_delete: DB error", slog.String("error", err.Error()))
		writeManageError(w, http.StatusInternalServerError, "Failed to delete role")
		return
	}
	if tag.RowsAffected() == 0 {
		writeManageError(w, http.StatusNotFound, "Role not found")
		return
	}

	ss.bus.PublishGuildEvent(ctx, events.SubjectGuildRoleDelete, "GUILD_ROLE_DELETE", guildID, map[string]string{
		"id": req.RoleID, "guild_id": guildID,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (ss *SyncService) manageMemberUpdate(ctx context.Context, w http.ResponseWriter, guildID, userID string, data json.RawMessage) {
	var req struct {
		MemberID     string     `json:"member_id"`
		Nickname     *string    `json:"nickname"`
		Deaf         *bool      `json:"deaf"`
		Mute         *bool      `json:"mute"`
		Roles        []string   `json:"roles"`
		TimeoutUntil *time.Time `json:"timeout_until"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		writeManageError(w, http.StatusBadRequest, "Invalid member_update data")
		return
	}
	if req.MemberID == "" {
		writeManageError(w, http.StatusBadRequest, "Missing member_id")
		return
	}

	// Nickname changes: the member can change their own, or ManageNicknames is required.
	if req.Nickname != nil && req.MemberID != userID {
		if !ss.hasManageGuildPermission(ctx, guildID, userID, permissions.ManageNicknames) {
			writeManageError(w, http.StatusForbidden, "Missing MANAGE_NICKNAMES permission")
			return
		}
	}
	// Deaf, mute, and timeout require TimeoutMembers permission.
	if req.Deaf != nil || req.Mute != nil || req.TimeoutUntil != nil {
		if !ss.hasManageGuildPermission(ctx, guildID, userID, permissions.TimeoutMembers) {
			writeManageError(w, http.StatusForbidden, "Missing TIMEOUT_MEMBERS permission")
			return
		}
	}
	// Role changes require ManageRoles.
	if req.Roles != nil {
		if !ss.hasManageGuildPermission(ctx, guildID, userID, permissions.ManageRoles) {
			writeManageError(w, http.StatusForbidden, "Missing MANAGE_ROLES permission")
			return
		}
	}

	if req.Nickname != nil {
		if _, err := ss.fed.pool.Exec(ctx,
			`UPDATE guild_members SET nickname = $3 WHERE guild_id = $1 AND user_id = $2`,
			guildID, req.MemberID, req.Nickname); err != nil {
			ss.logger.Error("manage member_update: failed to update nickname", slog.String("error", err.Error()))
			writeManageError(w, http.StatusInternalServerError, "Failed to update nickname")
			return
		}
	}

	if req.Deaf != nil {
		if _, err := ss.fed.pool.Exec(ctx,
			`UPDATE guild_members SET deaf = $3 WHERE guild_id = $1 AND user_id = $2`,
			guildID, req.MemberID, req.Deaf); err != nil {
			ss.logger.Error("manage member_update: failed to update deaf", slog.String("error", err.Error()))
			writeManageError(w, http.StatusInternalServerError, "Failed to update member")
			return
		}
	}

	if req.Mute != nil {
		if _, err := ss.fed.pool.Exec(ctx,
			`UPDATE guild_members SET mute = $3 WHERE guild_id = $1 AND user_id = $2`,
			guildID, req.MemberID, req.Mute); err != nil {
			ss.logger.Error("manage member_update: failed to update mute", slog.String("error", err.Error()))
			writeManageError(w, http.StatusInternalServerError, "Failed to update member")
			return
		}
	}

	if req.TimeoutUntil != nil {
		if _, err := ss.fed.pool.Exec(ctx,
			`UPDATE guild_members SET timeout_until = $3 WHERE guild_id = $1 AND user_id = $2`,
			guildID, req.MemberID, req.TimeoutUntil); err != nil {
			ss.logger.Error("manage member_update: failed to update timeout", slog.String("error", err.Error()))
			writeManageError(w, http.StatusInternalServerError, "Failed to update member")
			return
		}
	}

	if req.Roles != nil {
		// Replace all roles.
		if _, err := ss.fed.pool.Exec(ctx,
			`DELETE FROM member_roles WHERE guild_id = $1 AND user_id = $2`,
			guildID, req.MemberID); err != nil {
			ss.logger.Error("manage member_update: failed to clear roles", slog.String("error", err.Error()))
			writeManageError(w, http.StatusInternalServerError, "Failed to update roles")
			return
		}
		for _, roleID := range req.Roles {
			if _, err := ss.fed.pool.Exec(ctx,
				`INSERT INTO member_roles (guild_id, user_id, role_id) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
				guildID, req.MemberID, roleID); err != nil {
				ss.logger.Error("manage member_update: failed to assign role",
					slog.String("role_id", roleID), slog.String("error", err.Error()))
			}
		}
	}

	ss.bus.PublishGuildEvent(ctx, events.SubjectGuildMemberUpdate, "GUILD_MEMBER_UPDATE", guildID, map[string]string{
		"guild_id": guildID, "user_id": req.MemberID,
	})
	writeManageOK(w, map[string]string{"guild_id": guildID, "user_id": req.MemberID})
}

func (ss *SyncService) manageMemberRemove(ctx context.Context, w http.ResponseWriter, guildID, userID string, data json.RawMessage) {
	if !ss.hasManageGuildPermission(ctx, guildID, userID, permissions.KickMembers) {
		writeManageError(w, http.StatusForbidden, "Missing KICK_MEMBERS permission")
		return
	}

	var req struct {
		MemberID string `json:"member_id"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		writeManageError(w, http.StatusBadRequest, "Invalid member_remove data")
		return
	}
	if req.MemberID == "" {
		writeManageError(w, http.StatusBadRequest, "Missing member_id")
		return
	}

	// Can't kick the owner.
	var ownerID string
	_ = ss.fed.pool.QueryRow(ctx, `SELECT owner_id FROM guilds WHERE id = $1`, guildID).Scan(&ownerID)
	if req.MemberID == ownerID {
		writeManageError(w, http.StatusForbidden, "Cannot kick the guild owner")
		return
	}

	if _, err := ss.fed.pool.Exec(ctx,
		`DELETE FROM guild_members WHERE guild_id = $1 AND user_id = $2`, guildID, req.MemberID); err != nil {
		ss.logger.Error("manage member_remove: DB error", slog.String("error", err.Error()))
		writeManageError(w, http.StatusInternalServerError, "Failed to remove member")
		return
	}

	ss.bus.PublishGuildEvent(ctx, events.SubjectGuildMemberRemove, "GUILD_MEMBER_REMOVE", guildID, map[string]string{
		"guild_id": guildID, "user_id": req.MemberID,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (ss *SyncService) manageMemberBan(ctx context.Context, w http.ResponseWriter, guildID, userID string, data json.RawMessage) {
	if !ss.hasManageGuildPermission(ctx, guildID, userID, permissions.BanMembers) {
		writeManageError(w, http.StatusForbidden, "Missing BAN_MEMBERS permission")
		return
	}

	var req struct {
		UserID               string  `json:"user_id"`
		Reason               *string `json:"reason"`
		DurationSeconds      *int64  `json:"duration_seconds"`
		DeleteMessageSeconds *int64  `json:"delete_message_seconds"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		writeManageError(w, http.StatusBadRequest, "Invalid member_ban data")
		return
	}
	if req.UserID == "" {
		writeManageError(w, http.StatusBadRequest, "Missing user_id")
		return
	}

	// Can't ban the owner.
	var ownerID string
	_ = ss.fed.pool.QueryRow(ctx, `SELECT owner_id FROM guilds WHERE id = $1`, guildID).Scan(&ownerID)
	if req.UserID == ownerID {
		writeManageError(w, http.StatusForbidden, "Cannot ban the guild owner")
		return
	}

	// Compute expiry time for timed bans.
	var expiresAt *time.Time
	if req.DurationSeconds != nil && *req.DurationSeconds > 0 {
		t := time.Now().Add(time.Duration(*req.DurationSeconds) * time.Second)
		expiresAt = &t
	}

	// Remove from members.
	if _, err := ss.fed.pool.Exec(ctx,
		`DELETE FROM guild_members WHERE guild_id = $1 AND user_id = $2`, guildID, req.UserID); err != nil {
		ss.logger.Error("manage member_ban: failed to remove member", slog.String("error", err.Error()))
	}

	// Insert ban record with optional expiry.
	if _, err := ss.fed.pool.Exec(ctx,
		`INSERT INTO guild_bans (guild_id, user_id, reason, banned_by, expires_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, now())
		 ON CONFLICT (guild_id, user_id) DO UPDATE SET reason = $3, banned_by = $4, expires_at = $5`,
		guildID, req.UserID, req.Reason, userID, expiresAt); err != nil {
		ss.logger.Error("manage member_ban: DB error", slog.String("error", err.Error()))
		writeManageError(w, http.StatusInternalServerError, "Failed to ban member")
		return
	}

	// Delete recent messages if requested.
	if req.DeleteMessageSeconds != nil && *req.DeleteMessageSeconds > 0 {
		cutoff := time.Now().Add(-time.Duration(*req.DeleteMessageSeconds) * time.Second)
		if _, err := ss.fed.pool.Exec(ctx,
			`DELETE FROM messages
			 WHERE author_id = $1
			   AND channel_id IN (SELECT id FROM channels WHERE guild_id = $2)
			   AND created_at > $3`,
			req.UserID, guildID, cutoff); err != nil {
			ss.logger.Error("manage member_ban: failed to delete messages", slog.String("error", err.Error()))
		}
	}

	ss.bus.PublishGuildEvent(ctx, events.SubjectGuildBanAdd, "GUILD_BAN_ADD", guildID, map[string]string{
		"guild_id": guildID, "user_id": req.UserID,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (ss *SyncService) manageMemberUnban(ctx context.Context, w http.ResponseWriter, guildID, userID string, data json.RawMessage) {
	if !ss.hasManageGuildPermission(ctx, guildID, userID, permissions.BanMembers) {
		writeManageError(w, http.StatusForbidden, "Missing BAN_MEMBERS permission")
		return
	}

	var req struct {
		UserID string `json:"user_id"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		writeManageError(w, http.StatusBadRequest, "Invalid member_unban data")
		return
	}
	if req.UserID == "" {
		writeManageError(w, http.StatusBadRequest, "Missing user_id")
		return
	}

	tag, err := ss.fed.pool.Exec(ctx,
		`DELETE FROM guild_bans WHERE guild_id = $1 AND user_id = $2`, guildID, req.UserID)
	if err != nil {
		ss.logger.Error("manage member_unban: DB error", slog.String("error", err.Error()))
		writeManageError(w, http.StatusInternalServerError, "Failed to unban member")
		return
	}
	if tag.RowsAffected() == 0 {
		writeManageError(w, http.StatusNotFound, "User is not banned")
		return
	}

	ss.bus.PublishGuildEvent(ctx, events.SubjectGuildBanRemove, "GUILD_BAN_REMOVE", guildID, map[string]string{
		"guild_id": guildID, "user_id": req.UserID,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (ss *SyncService) manageCategoryCreate(ctx context.Context, w http.ResponseWriter, guildID, userID string, data json.RawMessage) {
	if !ss.hasManageGuildPermission(ctx, guildID, userID, permissions.ManageChannels) {
		writeManageError(w, http.StatusForbidden, "Missing MANAGE_CHANNELS permission")
		return
	}

	var req struct {
		Name     string `json:"name"`
		Position *int   `json:"position"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		writeManageError(w, http.StatusBadRequest, "Invalid category_create data")
		return
	}

	if req.Name == "" || len(req.Name) > 100 {
		writeManageError(w, http.StatusBadRequest, "Category name must be 1-100 characters")
		return
	}

	catID := models.NewULID().String()
	position := 0
	if req.Position != nil {
		position = *req.Position
	}

	var cat models.GuildCategory
	err := ss.fed.pool.QueryRow(ctx,
		`INSERT INTO guild_categories (id, guild_id, name, position, created_at)
		 VALUES ($1, $2, $3, $4, now())
		 RETURNING id, guild_id, name, position, created_at`,
		catID, guildID, req.Name, position,
	).Scan(&cat.ID, &cat.GuildID, &cat.Name, &cat.Position, &cat.CreatedAt)
	if err != nil {
		ss.logger.Error("manage category_create: DB error", slog.String("error", err.Error()))
		writeManageError(w, http.StatusInternalServerError, "Failed to create category")
		return
	}

	catData, _ := json.Marshal(cat)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(manageResponse{OK: true, Data: catData})
}

func (ss *SyncService) manageCategoryUpdate(ctx context.Context, w http.ResponseWriter, guildID, userID string, data json.RawMessage) {
	if !ss.hasManageGuildPermission(ctx, guildID, userID, permissions.ManageChannels) {
		writeManageError(w, http.StatusForbidden, "Missing MANAGE_CHANNELS permission")
		return
	}

	var req struct {
		CategoryID string  `json:"category_id"`
		Name       *string `json:"name"`
		Position   *int    `json:"position"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		writeManageError(w, http.StatusBadRequest, "Invalid category_update data")
		return
	}
	if req.CategoryID == "" {
		var idFallback struct{ ID string `json:"id"` }
		json.Unmarshal(data, &idFallback)
		req.CategoryID = idFallback.ID
	}
	if req.CategoryID == "" {
		writeManageError(w, http.StatusBadRequest, "Missing category_id")
		return
	}

	var cat models.GuildCategory
	err := ss.fed.pool.QueryRow(ctx,
		`UPDATE guild_categories SET
			name = COALESCE($3, name),
			position = COALESCE($4, position)
		 WHERE id = $1 AND guild_id = $2
		 RETURNING id, guild_id, name, position, created_at`,
		req.CategoryID, guildID, req.Name, req.Position,
	).Scan(&cat.ID, &cat.GuildID, &cat.Name, &cat.Position, &cat.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeManageError(w, http.StatusNotFound, "Category not found")
			return
		}
		ss.logger.Error("manage category_update: DB error", slog.String("error", err.Error()))
		writeManageError(w, http.StatusInternalServerError, "Failed to update category")
		return
	}

	writeManageOK(w, cat)
}

func (ss *SyncService) manageCategoryDelete(ctx context.Context, w http.ResponseWriter, guildID, userID string, data json.RawMessage) {
	if !ss.hasManageGuildPermission(ctx, guildID, userID, permissions.ManageChannels) {
		writeManageError(w, http.StatusForbidden, "Missing MANAGE_CHANNELS permission")
		return
	}

	var req struct {
		CategoryID string `json:"category_id"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		writeManageError(w, http.StatusBadRequest, "Invalid category_delete data")
		return
	}
	if req.CategoryID == "" {
		writeManageError(w, http.StatusBadRequest, "Missing category_id")
		return
	}

	tag, err := ss.fed.pool.Exec(ctx,
		`DELETE FROM guild_categories WHERE id = $1 AND guild_id = $2`, req.CategoryID, guildID)
	if err != nil {
		ss.logger.Error("manage category_delete: DB error", slog.String("error", err.Error()))
		writeManageError(w, http.StatusInternalServerError, "Failed to delete category")
		return
	}
	if tag.RowsAffected() == 0 {
		writeManageError(w, http.StatusNotFound, "Category not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (ss *SyncService) manageMessageDelete(ctx context.Context, w http.ResponseWriter, guildID, userID string, data json.RawMessage) {
	var req struct {
		MessageID string `json:"message_id"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		writeManageError(w, http.StatusBadRequest, "Invalid message_delete data")
		return
	}
	if req.MessageID == "" {
		writeManageError(w, http.StatusBadRequest, "Missing message_id")
		return
	}

	// Check if user authored the message or has ManageMessages permission.
	// JOIN through channels to verify the message belongs to the target guild.
	var authorID, channelID string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT m.author_id, m.channel_id FROM messages m
		 JOIN channels c ON c.id = m.channel_id
		 WHERE m.id = $1 AND c.guild_id = $2`, req.MessageID, guildID,
	).Scan(&authorID, &channelID); err != nil {
		writeManageError(w, http.StatusNotFound, "Message not found in this guild")
		return
	}

	if authorID != userID {
		if !ss.hasManageGuildPermission(ctx, guildID, userID, permissions.ManageMessages) {
			writeManageError(w, http.StatusForbidden, "Missing permission to delete this message")
			return
		}
	}

	if _, err := ss.fed.pool.Exec(ctx, `DELETE FROM messages WHERE id = $1`, req.MessageID); err != nil {
		ss.logger.Error("manage message_delete: DB error", slog.String("error", err.Error()))
		writeManageError(w, http.StatusInternalServerError, "Failed to delete message")
		return
	}

	ss.bus.Publish(ctx, events.SubjectMessageDelete, events.Event{
		Type:      "MESSAGE_DELETE",
		GuildID:   guildID,
		ChannelID: channelID,
		Data:      mustMarshalManage(map[string]string{"id": req.MessageID, "channel_id": channelID}),
	})
	w.WriteHeader(http.StatusNoContent)
}

func (ss *SyncService) manageMessagePin(ctx context.Context, w http.ResponseWriter, guildID, userID string, data json.RawMessage) {
	if !ss.hasManageGuildPermission(ctx, guildID, userID, permissions.ManageMessages) {
		writeManageError(w, http.StatusForbidden, "Missing MANAGE_MESSAGES permission")
		return
	}

	var req struct {
		MessageID string `json:"message_id"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		writeManageError(w, http.StatusBadRequest, "Invalid message_pin data")
		return
	}
	if req.MessageID == "" {
		writeManageError(w, http.StatusBadRequest, "Missing message_id")
		return
	}

	// Verify message belongs to this guild and get its channel_id.
	var channelID string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT m.channel_id FROM messages m
		 JOIN channels c ON c.id = m.channel_id
		 WHERE m.id = $1 AND c.guild_id = $2`,
		req.MessageID, guildID,
	).Scan(&channelID); err != nil {
		writeManageError(w, http.StatusNotFound, "Message not found in this guild")
		return
	}

	// Enforce pin limit (50 per channel).
	var pinCount int
	ss.fed.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM pins WHERE channel_id = $1`, channelID).Scan(&pinCount)
	if pinCount >= 50 {
		writeManageError(w, http.StatusBadRequest, "Channel has reached the maximum of 50 pinned messages")
		return
	}

	// Insert into pins table (matches local handler pattern).
	_, err := ss.fed.pool.Exec(ctx,
		`INSERT INTO pins (channel_id, message_id, pinned_by, pinned_at)
		 VALUES ($1, $2, $3, now())
		 ON CONFLICT (channel_id, message_id) DO NOTHING`,
		channelID, req.MessageID, userID,
	)
	if err != nil {
		ss.logger.Error("manage message_pin: DB error", slog.String("error", err.Error()))
		writeManageError(w, http.StatusInternalServerError, "Failed to pin message")
		return
	}

	ss.bus.PublishChannelEvent(ctx, events.SubjectChannelPinsUpdate, "CHANNEL_PINS_UPDATE", channelID, map[string]string{
		"channel_id": channelID,
	})

	writeManageOK(w, map[string]string{"message_id": req.MessageID, "pinned": "true"})
}

func (ss *SyncService) manageMessageUnpin(ctx context.Context, w http.ResponseWriter, guildID, userID string, data json.RawMessage) {
	if !ss.hasManageGuildPermission(ctx, guildID, userID, permissions.ManageMessages) {
		writeManageError(w, http.StatusForbidden, "Missing MANAGE_MESSAGES permission")
		return
	}

	var req struct {
		MessageID string `json:"message_id"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		writeManageError(w, http.StatusBadRequest, "Invalid message_unpin data")
		return
	}
	if req.MessageID == "" {
		writeManageError(w, http.StatusBadRequest, "Missing message_id")
		return
	}

	// Verify message belongs to this guild and get its channel_id.
	var channelID string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT m.channel_id FROM messages m
		 JOIN channels c ON c.id = m.channel_id
		 WHERE m.id = $1 AND c.guild_id = $2`,
		req.MessageID, guildID,
	).Scan(&channelID); err != nil {
		writeManageError(w, http.StatusNotFound, "Message not found in this guild")
		return
	}

	// Delete from pins table (matches local handler pattern).
	tag, err := ss.fed.pool.Exec(ctx,
		`DELETE FROM pins WHERE channel_id = $1 AND message_id = $2`,
		channelID, req.MessageID)
	if err != nil {
		ss.logger.Error("manage message_unpin: DB error", slog.String("error", err.Error()))
		writeManageError(w, http.StatusInternalServerError, "Failed to unpin message")
		return
	}
	if tag.RowsAffected() == 0 {
		writeManageError(w, http.StatusNotFound, "Message is not pinned")
		return
	}

	ss.bus.PublishChannelEvent(ctx, events.SubjectChannelPinsUpdate, "CHANNEL_PINS_UPDATE", channelID, map[string]string{
		"channel_id": channelID,
	})

	writeManageOK(w, map[string]string{"message_id": req.MessageID, "pinned": "false"})
}

func (ss *SyncService) manageMemberRoleRemove(ctx context.Context, w http.ResponseWriter, guildID, userID string, data json.RawMessage) {
	if !ss.hasManageGuildPermission(ctx, guildID, userID, permissions.ManageRoles) {
		writeManageError(w, http.StatusForbidden, "Missing MANAGE_ROLES permission")
		return
	}

	var req struct {
		MemberID string `json:"member_id"`
		RoleID   string `json:"role_id"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		writeManageError(w, http.StatusBadRequest, "Invalid member_role_remove data")
		return
	}
	if req.MemberID == "" || req.RoleID == "" {
		writeManageError(w, http.StatusBadRequest, "Missing member_id or role_id")
		return
	}

	if _, err := ss.fed.pool.Exec(ctx,
		`DELETE FROM member_roles WHERE guild_id = $1 AND user_id = $2 AND role_id = $3`,
		guildID, req.MemberID, req.RoleID); err != nil {
		ss.logger.Error("manage member_role_remove: DB error", slog.String("error", err.Error()))
		writeManageError(w, http.StatusInternalServerError, "Failed to remove role from member")
		return
	}

	ss.bus.PublishGuildEvent(ctx, events.SubjectGuildMemberUpdate, "GUILD_MEMBER_UPDATE", guildID, map[string]string{
		"guild_id": guildID, "user_id": req.MemberID,
	})
	w.WriteHeader(http.StatusNoContent)
}

// manageMemberJoin handles a federated user joining via an invite on the home instance.
// Creates user stub, adds to guild_members, registers channel peers, and returns success.
func (ss *SyncService) manageMemberJoin(ctx context.Context, w http.ResponseWriter, guildID, senderInstanceID, userID string, data json.RawMessage) {
	var req struct {
		InviteCode string `json:"invite_code"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		writeManageError(w, http.StatusBadRequest, "Invalid member_join data")
		return
	}

	// Validate the invite code is provided.
	if req.InviteCode == "" {
		writeManageError(w, http.StatusBadRequest, "Invite code is required")
		return
	}

	// Check if the user is banned from this guild (before starting the transaction).
	var banned bool
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM guild_bans WHERE guild_id = $1 AND user_id = $2)`,
		guildID, userID).Scan(&banned); err != nil {
		ss.logger.Error("failed to check guild ban for federated join",
			slog.String("guild_id", guildID), slog.String("user_id", userID),
			slog.String("error", err.Error()))
		writeManageError(w, http.StatusInternalServerError, "Failed to check ban status")
		return
	}
	if banned {
		writeManageError(w, http.StatusForbidden, "User is banned from this guild")
		return
	}

	// Create a minimal user stub if the user doesn't exist locally yet.
	ss.ensureRemoteUserStub(ctx, senderInstanceID, federatedUserInfo{
		ID: userID,
	})
	// Verify the stub was created and belongs to the sender instance.
	var stubInstanceID *string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT instance_id FROM users WHERE id = $1`, userID,
	).Scan(&stubInstanceID); err != nil {
		writeManageError(w, http.StatusInternalServerError, "Failed to register remote user")
		return
	}
	if stubInstanceID == nil || *stubInstanceID != senderInstanceID {
		writeManageError(w, http.StatusForbidden, "User does not belong to sender instance")
		return
	}

	// Use a transaction with row locks to enforce invite limits and member count atomically.
	tx, err := ss.fed.pool.Begin(ctx)
	if err != nil {
		writeManageError(w, http.StatusInternalServerError, "Failed to start join transaction")
		return
	}
	defer tx.Rollback(ctx)

	// Validate the invite exists, belongs to this guild, and is still usable (locked for update).
	var invGuildID string
	var invMaxUses *int
	var invUses int
	var invExpiresAt *time.Time
	if err := tx.QueryRow(ctx,
		`SELECT guild_id, max_uses, uses, expires_at FROM invites WHERE code = $1 FOR UPDATE`,
		req.InviteCode,
	).Scan(&invGuildID, &invMaxUses, &invUses, &invExpiresAt); err != nil {
		writeManageError(w, http.StatusNotFound, "Invite not found")
		return
	}
	if invGuildID != guildID {
		writeManageError(w, http.StatusBadRequest, "Invite does not belong to this guild")
		return
	}
	if invExpiresAt != nil && time.Now().After(*invExpiresAt) {
		writeManageError(w, http.StatusGone, "Invite has expired")
		return
	}
	if invMaxUses != nil && invUses >= *invMaxUses {
		writeManageError(w, http.StatusGone, "Invite has been exhausted")
		return
	}

	// Check max members (locked for update).
	var maxMembers, currentMembers int
	if err := tx.QueryRow(ctx,
		`SELECT max_members, member_count FROM guilds WHERE id = $1 FOR UPDATE`, guildID,
	).Scan(&maxMembers, &currentMembers); err != nil {
		writeManageError(w, http.StatusInternalServerError, "Failed to check guild capacity")
		return
	}
	if maxMembers > 0 && currentMembers >= maxMembers {
		writeManageError(w, http.StatusForbidden, "Guild has reached its maximum member count")
		return
	}

	// Add to guild_members.
	now := time.Now().UTC()
	tag, err := tx.Exec(ctx,
		`INSERT INTO guild_members (guild_id, user_id, joined_at)
		 VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
		guildID, userID, now)
	if err != nil {
		writeManageError(w, http.StatusInternalServerError, "Failed to add member")
		return
	}

	if tag.RowsAffected() > 0 {
		if _, err := tx.Exec(ctx,
			`UPDATE invites SET uses = uses + 1 WHERE code = $1`, req.InviteCode); err != nil {
			ss.logger.Error("failed to increment invite usage",
				slog.String("invite_code", req.InviteCode), slog.String("error", err.Error()))
		}

		if _, err := tx.Exec(ctx,
			`UPDATE guilds SET member_count = member_count + 1 WHERE id = $1`, guildID); err != nil {
			ss.logger.Error("failed to increment guild member count",
				slog.String("guild_id", guildID), slog.String("error", err.Error()))
		}
	}

	if err := tx.Commit(ctx); err != nil {
		writeManageError(w, http.StatusInternalServerError, "Failed to finalize join")
		return
	}

	if tag.RowsAffected() > 0 {
		// Register channel peers so events flow to the sender instance (outside tx).
		ss.addInstanceToGuildChannelPeers(ctx, guildID, senderInstanceID)

		ss.bus.PublishGuildEvent(ctx, events.SubjectGuildMemberAdd, "GUILD_MEMBER_ADD", guildID,
			map[string]interface{}{
				"guild_id":  guildID,
				"user_id":   userID,
				"joined_at": now,
			})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(manageResponse{OK: true})
}

// mustMarshalManage marshals v to JSON, returning nil on failure.
func mustMarshalManage(v interface{}) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
