package federation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	neturl "net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/models"
	"github.com/amityvox/amityvox/internal/permissions"
)

// --- Request/Response types ---

type federatedGuildJoinRequest struct {
	UserID         string  `json:"user_id"`
	Username       string  `json:"username"`
	DisplayName    *string `json:"display_name,omitempty"`
	AvatarID       *string `json:"avatar_id,omitempty"`
	InstanceDomain string  `json:"instance_domain"`
}

type federatedGuildInviteRequest struct {
	InviteCode     string  `json:"invite_code"`
	UserID         string  `json:"user_id"`
	Username       string  `json:"username"`
	DisplayName    *string `json:"display_name,omitempty"`
	AvatarID       *string `json:"avatar_id,omitempty"`
	InstanceDomain string  `json:"instance_domain"`
}

type federatedGuildLeaveRequest struct {
	UserID string `json:"user_id"`
}

type federatedGuildMessagesRequest struct {
	UserID string `json:"user_id"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type federatedGuildPostMessageRequest struct {
	UserID      string   `json:"user_id"`
	Content     string   `json:"content"`
	Nonce       string   `json:"nonce,omitempty"`
	ReplyToIDs  []string `json:"reply_to_ids,omitempty"`
}

type federatedGuildMembersRequest struct {
	UserID string `json:"user_id"`
}

type federatedGuildReactionRequest struct {
	UserID string `json:"user_id"`
	Emoji  string `json:"emoji"`
}

type federatedGuildTypingRequest struct {
	UserID string `json:"user_id"`
}

type guildPreviewResponse struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Description  *string `json:"description,omitempty"`
	IconID       *string `json:"icon_id,omitempty"`
	MemberCount  int     `json:"member_count"`
	OnlineCount  int     `json:"online_count"`
	Discoverable bool    `json:"discoverable"`
}

type guildJoinResponse struct {
	GuildID      string          `json:"guild_id"`
	Name         string          `json:"name"`
	IconID       *string         `json:"icon_id,omitempty"`
	Description  *string         `json:"description,omitempty"`
	MemberCount  int             `json:"member_count"`
	OwnerID      string          `json:"owner_id"`
	InstanceID   string          `json:"instance_id"`
	ChannelsJSON json.RawMessage `json:"channels"`
	RolesJSON    json.RawMessage `json:"roles"`
	// Owner user stub info so the joining instance can create a local user stub.
	OwnerUsername    string  `json:"owner_username"`
	OwnerDisplayName *string `json:"owner_display_name,omitempty"`
	OwnerAvatarID    *string `json:"owner_avatar_id,omitempty"`
}

// ============================================================
// Remote-facing handlers (on the guild's host instance)
// ============================================================

// HandleFederatedGuildPreview returns a public preview of a discoverable guild.
// GET /federation/v1/guilds/{guildID}/preview
func (ss *SyncService) HandleFederatedGuildPreview(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")
	if guildID == "" {
		http.Error(w, "Missing guild ID", http.StatusBadRequest)
		return
	}

	var preview guildPreviewResponse
	err := ss.fed.pool.QueryRow(r.Context(),
		`SELECT id, name, description, icon_id, member_count, discoverable
		 FROM guilds WHERE id = $1`, guildID,
	).Scan(&preview.ID, &preview.Name, &preview.Description, &preview.IconID,
		&preview.MemberCount, &preview.Discoverable)
	if err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, "Guild not found", http.StatusNotFound)
		} else {
			http.Error(w, "Internal error", http.StatusInternalServerError)
		}
		return
	}

	if !preview.Discoverable {
		http.Error(w, "Guild not discoverable", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(preview)
}

// HandleFederatedGuildJoin handles a remote user joining a discoverable guild.
// POST /federation/v1/guilds/{guildID}/join
func (ss *SyncService) HandleFederatedGuildJoin(w http.ResponseWriter, r *http.Request) {
	signed, senderID, ok := ss.verifyFederationRequest(w, r)
	if !ok {
		return
	}

	guildID := chi.URLParam(r, "guildID")
	if guildID == "" {
		http.Error(w, "Missing guild ID", http.StatusBadRequest)
		return
	}

	var req federatedGuildJoinRequest
	if err := json.Unmarshal(signed.Payload, &req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}
	if req.UserID == "" || req.Username == "" || req.InstanceDomain == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Validate guild exists and is discoverable.
	var discoverable bool
	err := ss.fed.pool.QueryRow(ctx,
		`SELECT discoverable FROM guilds WHERE id = $1`, guildID,
	).Scan(&discoverable)
	if err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, "Guild not found", http.StatusNotFound)
		} else {
			http.Error(w, "Internal error", http.StatusInternalServerError)
		}
		return
	}
	if !discoverable {
		http.Error(w, "Guild is not open to federation joins", http.StatusForbidden)
		return
	}

	// Check user is not banned (fail closed on query error).
	var banned bool
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM guild_bans WHERE guild_id = $1 AND user_id = $2)`,
		guildID, req.UserID,
	).Scan(&banned); err != nil {
		ss.logger.Error("failed to check guild ban", slog.String("error", err.Error()))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	if banned {
		http.Error(w, "User is banned from this guild", http.StatusForbidden)
		return
	}

	// Validate that the claimed domain matches the signed sender.
	instanceID, ok := ss.validateSenderDomain(ctx, w, senderID, req.InstanceDomain)
	if !ok {
		return
	}

	// Create remote user stub.
	ss.ensureRemoteUserStub(ctx, instanceID, federatedUserInfo{
		ID:             req.UserID,
		Username:       req.Username,
		DisplayName:    req.DisplayName,
		AvatarID:       req.AvatarID,
		InstanceDomain: req.InstanceDomain,
	})

	// Add to guild_members (idempotent).
	tag, err := ss.fed.pool.Exec(ctx,
		`INSERT INTO guild_members (guild_id, user_id, joined_at)
		 VALUES ($1, $2, now()) ON CONFLICT DO NOTHING`,
		guildID, req.UserID,
	)
	if err != nil {
		ss.logger.Error("failed to add federated guild member",
			slog.String("guild_id", guildID),
			slog.String("user_id", req.UserID),
			slog.String("error", err.Error()),
		)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	// Only update counts and peers if a new row was inserted.
	if tag.RowsAffected() > 0 {
		if _, err := ss.fed.pool.Exec(ctx,
			`UPDATE guilds SET member_count = member_count + 1 WHERE id = $1`, guildID); err != nil {
			ss.logger.Warn("failed to increment member count", slog.String("error", err.Error()))
		}

		ss.addInstanceToGuildChannelPeers(ctx, guildID, instanceID)

		ss.bus.PublishGuildEvent(ctx, events.SubjectGuildMemberAdd, "GUILD_MEMBER_ADD", guildID, map[string]interface{}{
			"guild_id": guildID,
			"user_id":  req.UserID,
			"username": req.Username,
		})
	}

	resp, err := ss.buildGuildJoinResponse(ctx, guildID)
	if err != nil {
		ss.logger.Error("failed to build guild join response", slog.String("error", err.Error()))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

// HandleFederatedGuildLeave handles a remote user leaving a guild.
// POST /federation/v1/guilds/{guildID}/leave
func (ss *SyncService) HandleFederatedGuildLeave(w http.ResponseWriter, r *http.Request) {
	signed, senderID, ok := ss.verifyFederationRequest(w, r)
	if !ok {
		return
	}

	guildID := chi.URLParam(r, "guildID")
	if guildID == "" {
		http.Error(w, "Missing guild ID", http.StatusBadRequest)
		return
	}

	var req federatedGuildLeaveRequest
	if err := json.Unmarshal(signed.Payload, &req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}
	if req.UserID == "" {
		http.Error(w, "Missing user_id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Verify the user belongs to the sender's instance.
	if !ss.validateSenderUser(ctx, w, senderID, req.UserID) {
		return
	}

	tag, err := ss.fed.pool.Exec(ctx,
		`DELETE FROM guild_members WHERE guild_id = $1 AND user_id = $2`,
		guildID, req.UserID)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "Not a member", http.StatusNotFound)
		return
	}

	if _, err := ss.fed.pool.Exec(ctx,
		`UPDATE guilds SET member_count = GREATEST(member_count - 1, 0) WHERE id = $1`, guildID); err != nil {
		ss.logger.Warn("failed to decrement member count", slog.String("error", err.Error()))
	}

	// Check if any members from this instance remain.
	// Only remove channel peers if no members from this instance remain.
	// On query error, skip removal to avoid breaking federation for remaining members.
	var remainingCount int
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM guild_members gm
		 JOIN users u ON u.id = gm.user_id
		 WHERE gm.guild_id = $1 AND u.instance_id = $2`,
		guildID, senderID,
	).Scan(&remainingCount); err != nil {
		ss.logger.Warn("failed to count remaining members from instance, skipping peer cleanup",
			slog.String("error", err.Error()))
	} else if remainingCount == 0 {
		ss.fed.pool.Exec(ctx,
			`DELETE FROM federation_channel_peers
			 WHERE instance_id = $1 AND channel_id IN (SELECT id FROM channels WHERE guild_id = $2)`,
			senderID, guildID)
	}

	ss.bus.PublishGuildEvent(ctx, events.SubjectGuildMemberRemove, "GUILD_MEMBER_REMOVE", guildID, map[string]interface{}{
		"guild_id": guildID,
		"user_id":  req.UserID,
	})

	w.WriteHeader(http.StatusNoContent)
}

// HandleFederatedGuildInviteAccept handles a remote user accepting a guild invite.
// POST /federation/v1/guilds/invite-accept
func (ss *SyncService) HandleFederatedGuildInviteAccept(w http.ResponseWriter, r *http.Request) {
	signed, senderID, ok := ss.verifyFederationRequest(w, r)
	if !ok {
		return
	}

	var req federatedGuildInviteRequest
	if err := json.Unmarshal(signed.Payload, &req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}
	if req.InviteCode == "" || req.UserID == "" || req.Username == "" || req.InstanceDomain == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Validate invite.
	var guildID string
	var maxUses, uses int
	var expiresAt *time.Time
	err := ss.fed.pool.QueryRow(ctx,
		`SELECT guild_id, max_uses, uses, expires_at FROM invites WHERE code = $1`,
		req.InviteCode,
	).Scan(&guildID, &maxUses, &uses, &expiresAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, "Invalid invite code", http.StatusNotFound)
		} else {
			http.Error(w, "Internal error", http.StatusInternalServerError)
		}
		return
	}

	if expiresAt != nil && time.Now().After(*expiresAt) {
		http.Error(w, "Invite has expired", http.StatusGone)
		return
	}
	if maxUses > 0 && uses >= maxUses {
		http.Error(w, "Invite has been exhausted", http.StatusGone)
		return
	}

	// Check ban (fail closed on query error).
	var banned bool
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM guild_bans WHERE guild_id = $1 AND user_id = $2)`,
		guildID, req.UserID,
	).Scan(&banned); err != nil {
		ss.logger.Error("failed to check guild ban", slog.String("error", err.Error()))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	if banned {
		http.Error(w, "User is banned from this guild", http.StatusForbidden)
		return
	}

	// Validate that the claimed domain matches the signed sender.
	instanceID, ok := ss.validateSenderDomain(ctx, w, senderID, req.InstanceDomain)
	if !ok {
		return
	}

	// Create user stub.
	ss.ensureRemoteUserStub(ctx, instanceID, federatedUserInfo{
		ID: req.UserID, Username: req.Username,
		DisplayName: req.DisplayName, AvatarID: req.AvatarID,
		InstanceDomain: req.InstanceDomain,
	})

	// Add to guild_members.
	tag, err := ss.fed.pool.Exec(ctx,
		`INSERT INTO guild_members (guild_id, user_id, joined_at)
		 VALUES ($1, $2, now()) ON CONFLICT DO NOTHING`,
		guildID, req.UserID)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	if tag.RowsAffected() > 0 {
		ss.fed.pool.Exec(ctx, `UPDATE invites SET uses = uses + 1 WHERE code = $1`, req.InviteCode)
		ss.fed.pool.Exec(ctx, `UPDATE guilds SET member_count = member_count + 1 WHERE id = $1`, guildID)
		ss.addInstanceToGuildChannelPeers(ctx, guildID, instanceID)

		ss.bus.PublishGuildEvent(ctx, events.SubjectGuildMemberAdd, "GUILD_MEMBER_ADD", guildID, map[string]interface{}{
			"guild_id": guildID, "user_id": req.UserID, "username": req.Username,
		})
	}

	resp, err := ss.buildGuildJoinResponse(ctx, guildID)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

// HandleFederatedGuildMessages returns messages for a guild channel to a federated user.
// POST /federation/v1/guilds/{guildID}/channels/{channelID}/messages
func (ss *SyncService) HandleFederatedGuildMessages(w http.ResponseWriter, r *http.Request) {
	signed, senderID, ok := ss.verifyFederationRequest(w, r)
	if !ok {
		return
	}

	guildID := chi.URLParam(r, "guildID")
	channelID := chi.URLParam(r, "channelID")
	if guildID == "" || channelID == "" {
		http.Error(w, "Missing guild or channel ID", http.StatusBadRequest)
		return
	}

	var req federatedGuildMessagesRequest
	if err := json.Unmarshal(signed.Payload, &req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}
	if req.UserID == "" {
		http.Error(w, "Missing user_id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Verify the user belongs to the sender's instance.
	if !ss.validateSenderUser(ctx, w, senderID, req.UserID) {
		return
	}

	// Verify channel belongs to guild and is not private.
	var channelGuildID *string
	var channelType *string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT guild_id, channel_type FROM channels WHERE id = $1`, channelID,
	).Scan(&channelGuildID, &channelType); err != nil || channelGuildID == nil || *channelGuildID != guildID {
		http.Error(w, "Channel not found in guild", http.StatusNotFound)
		return
	}
	if channelType != nil && *channelType == "private" {
		http.Error(w, "Channel not accessible", http.StatusForbidden)
		return
	}

	// Verify user is a member.
	var isMember bool
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM guild_members WHERE guild_id = $1 AND user_id = $2)`,
		guildID, req.UserID,
	).Scan(&isMember); err != nil {
		ss.logger.Error("failed to check guild membership", slog.String("error", err.Error()))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	if !isMember {
		http.Error(w, "Not a guild member", http.StatusForbidden)
		return
	}

	// Check ViewChannel + ReadHistory permissions for the requesting user.
	if !ss.hasChannelPermission(ctx, guildID, channelID, req.UserID, permissions.ViewChannel|permissions.ReadHistory) {
		http.Error(w, "Missing ViewChannel or ReadHistory permission", http.StatusForbidden)
		return
	}

	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	query := `SELECT m.id, m.channel_id, m.author_id, m.content, m.created_at,
	                 u.username, u.display_name, u.avatar_id, u.instance_id, COALESCE(i.domain, '')
	          FROM messages m
	          JOIN users u ON u.id = m.author_id
	          LEFT JOIN instances i ON i.id = u.instance_id
	          WHERE m.channel_id = $1`
	args := []interface{}{channelID}
	argN := 2

	if req.Before != "" {
		query += fmt.Sprintf(` AND m.id < $%d`, argN)
		args = append(args, req.Before)
		argN++
	}
	if req.After != "" {
		query += fmt.Sprintf(` AND m.id > $%d`, argN)
		args = append(args, req.After)
		argN++
	}

	if req.After != "" {
		query += ` ORDER BY m.id ASC`
	} else {
		query += ` ORDER BY m.id DESC`
	}
	query += fmt.Sprintf(` LIMIT $%d`, argN)
	args = append(args, limit)

	rows, err := ss.fed.pool.Query(ctx, query, args...)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	messages := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id, chID, authorID string
		var content *string
		var createdAt time.Time
		var username string
		var displayName, avatarID, instanceID *string
		var instanceDomain string
		if err := rows.Scan(&id, &chID, &authorID, &content, &createdAt,
			&username, &displayName, &avatarID, &instanceID, &instanceDomain); err != nil {
			ss.logger.Warn("failed to scan federated message row", slog.String("error", err.Error()))
			continue
		}
		authorObj := map[string]interface{}{
			"id": authorID, "username": username,
			"display_name": displayName, "avatar_id": avatarID,
		}
		if instanceID != nil {
			authorObj["instance_id"] = *instanceID
		}
		if instanceDomain != "" {
			authorObj["instance_domain"] = instanceDomain
		}
		messages = append(messages, map[string]interface{}{
			"id": id, "channel_id": chID, "author_id": authorID,
			"content": content, "created_at": createdAt,
			"author": authorObj,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": messages})
}

// HandleFederatedGuildPostMessage creates a message in a guild channel from a federated user.
// POST /federation/v1/guilds/{guildID}/channels/{channelID}/messages/create
func (ss *SyncService) HandleFederatedGuildPostMessage(w http.ResponseWriter, r *http.Request) {
	signed, senderID, ok := ss.verifyFederationRequest(w, r)
	if !ok {
		return
	}

	guildID := chi.URLParam(r, "guildID")
	channelID := chi.URLParam(r, "channelID")
	if guildID == "" || channelID == "" {
		http.Error(w, "Missing guild or channel ID", http.StatusBadRequest)
		return
	}

	var req federatedGuildPostMessageRequest
	if err := json.Unmarshal(signed.Payload, &req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}
	if req.UserID == "" || req.Content == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Verify the user belongs to the sender's instance.
	if !ss.validateSenderUser(ctx, w, senderID, req.UserID) {
		return
	}

	// Verify channel belongs to guild, is not private, and is not locked.
	var channelGuildID *string
	var locked bool
	var channelType *string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT guild_id, locked, channel_type FROM channels WHERE id = $1`, channelID,
	).Scan(&channelGuildID, &locked, &channelType); err != nil || channelGuildID == nil || *channelGuildID != guildID {
		http.Error(w, "Channel not found in guild", http.StatusNotFound)
		return
	}
	if channelType != nil && *channelType == "private" {
		http.Error(w, "Channel not accessible", http.StatusForbidden)
		return
	}
	if locked {
		http.Error(w, "Channel is locked", http.StatusForbidden)
		return
	}

	// Verify membership.
	var isMember bool
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM guild_members WHERE guild_id = $1 AND user_id = $2)`,
		guildID, req.UserID,
	).Scan(&isMember); err != nil {
		ss.logger.Error("failed to check guild membership", slog.String("error", err.Error()))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	if !isMember {
		http.Error(w, "Not a guild member", http.StatusForbidden)
		return
	}

	msgID := models.NewULID().String()
	now := time.Now()

	// Validate reply_to_ids if provided — all must exist in this channel.
	var replyToIDs []string
	if len(req.ReplyToIDs) > 0 {
		var validCount int
		err := ss.fed.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM messages WHERE id = ANY($1) AND channel_id = $2`,
			req.ReplyToIDs, channelID,
		).Scan(&validCount)
		if err != nil || validCount != len(req.ReplyToIDs) {
			http.Error(w, "One or more reply_to_ids not found in channel", http.StatusBadRequest)
			return
		}
		replyToIDs = req.ReplyToIDs
	}

	_, err := ss.fed.pool.Exec(ctx,
		`INSERT INTO messages (id, channel_id, author_id, content, reply_to_ids, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		msgID, channelID, req.UserID, req.Content, replyToIDs, now)
	if err != nil {
		ss.logger.Error("failed to create federated guild message", slog.String("error", err.Error()))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	if _, err := ss.fed.pool.Exec(ctx,
		`UPDATE channels SET last_message_id = $1 WHERE id = $2`, msgID, channelID); err != nil {
		ss.logger.Warn("failed to update last_message_id", slog.String("error", err.Error()))
	}

	// Fetch author data so the event includes the full author object.
	// Without this, the frontend falls back to author_id (a ULID) as the display name.
	var authorUsername string
	var authorDisplayName, authorAvatarID *string
	var authorInstanceDomain string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT u.username, u.display_name, u.avatar_id, COALESCE(i.domain, '')
		 FROM users u LEFT JOIN instances i ON i.id = u.instance_id
		 WHERE u.id = $1`, req.UserID,
	).Scan(&authorUsername, &authorDisplayName, &authorAvatarID, &authorInstanceDomain); err != nil {
		ss.logger.Warn("failed to fetch author for federated message", slog.String("error", err.Error()))
		authorUsername = req.UserID // fallback so the event isn't blank
	}

	authorObj := map[string]interface{}{
		"id": req.UserID, "username": authorUsername,
		"display_name": authorDisplayName, "avatar_id": authorAvatarID,
	}
	if authorInstanceDomain != "" {
		authorObj["instance_domain"] = authorInstanceDomain
	}

	msg := map[string]interface{}{
		"id": msgID, "channel_id": channelID, "guild_id": guildID,
		"author_id": req.UserID, "content": req.Content, "created_at": now,
		"reply_to_ids": replyToIDs, "author": authorObj,
	}
	ss.bus.PublishChannelEvent(ctx, events.SubjectMessageCreate, "MESSAGE_CREATE", channelID, msg)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"data": msg})
}

// HandleFederatedGuildMembers returns the member list for a guild to a federated peer.
// POST /federation/v1/guilds/{guildID}/members
func (ss *SyncService) HandleFederatedGuildMembers(w http.ResponseWriter, r *http.Request) {
	signed, senderID, ok := ss.verifyFederationRequest(w, r)
	if !ok {
		return
	}

	guildID := chi.URLParam(r, "guildID")
	if guildID == "" {
		http.Error(w, "Missing guild ID", http.StatusBadRequest)
		return
	}

	var req federatedGuildMembersRequest
	if err := json.Unmarshal(signed.Payload, &req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}
	if req.UserID == "" {
		http.Error(w, "Missing user_id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Verify the user belongs to the sender's instance.
	if !ss.validateSenderUser(ctx, w, senderID, req.UserID) {
		return
	}

	// Verify requesting user is a member.
	var isMember bool
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM guild_members WHERE guild_id = $1 AND user_id = $2)`,
		guildID, req.UserID,
	).Scan(&isMember); err != nil || !isMember {
		http.Error(w, "Not a guild member", http.StatusForbidden)
		return
	}

	rows, err := ss.fed.pool.Query(ctx,
		`SELECT gm.user_id, u.username, u.display_name, u.avatar_id,
		        COALESCE(i.domain, '') AS instance_domain,
		        COALESCE(
		          (SELECT json_agg(mr.role_id) FROM member_roles mr
		           WHERE mr.guild_id = gm.guild_id AND mr.user_id = gm.user_id),
		          '[]'::json
		        ) AS role_ids,
		        gm.joined_at
		 FROM guild_members gm
		 JOIN users u ON u.id = gm.user_id
		 LEFT JOIN instances i ON i.id = u.instance_id
		 WHERE gm.guild_id = $1
		 ORDER BY gm.joined_at ASC
		 LIMIT 200`, guildID)
	if err != nil {
		ss.logger.Error("failed to query guild members", slog.String("error", err.Error()))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	members := make([]map[string]interface{}, 0)
	for rows.Next() {
		var userID, username string
		var displayName, avatarID *string
		var instanceDomain string
		var roleIDsJSON json.RawMessage
		var joinedAt time.Time
		if err := rows.Scan(&userID, &username, &displayName, &avatarID,
			&instanceDomain, &roleIDsJSON, &joinedAt); err != nil {
			ss.logger.Warn("failed to scan member row", slog.String("error", err.Error()))
			continue
		}
		members = append(members, map[string]interface{}{
			"user_id":         userID,
			"username":        username,
			"display_name":    displayName,
			"avatar_id":       avatarID,
			"instance_domain": instanceDomain,
			"role_ids":        roleIDsJSON,
			"joined_at":       joinedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": members})
}

// HandleFederatedGuildReactionAdd adds a reaction on behalf of a federated user.
// POST /federation/v1/guilds/{guildID}/channels/{channelID}/messages/{messageID}/reactions
func (ss *SyncService) HandleFederatedGuildReactionAdd(w http.ResponseWriter, r *http.Request) {
	signed, senderID, ok := ss.verifyFederationRequest(w, r)
	if !ok {
		return
	}

	guildID := chi.URLParam(r, "guildID")
	channelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")
	if guildID == "" || channelID == "" || messageID == "" {
		http.Error(w, "Missing path parameters", http.StatusBadRequest)
		return
	}

	var req federatedGuildReactionRequest
	if err := json.Unmarshal(signed.Payload, &req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}
	if req.UserID == "" || req.Emoji == "" {
		http.Error(w, "Missing user_id or emoji", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	if !ss.validateSenderUser(ctx, w, senderID, req.UserID) {
		return
	}

	// Verify channel belongs to guild and is not private.
	var channelGuildID *string
	var channelType *string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT guild_id, channel_type FROM channels WHERE id = $1`, channelID,
	).Scan(&channelGuildID, &channelType); err != nil || channelGuildID == nil || *channelGuildID != guildID {
		http.Error(w, "Channel not found in guild", http.StatusNotFound)
		return
	}
	if channelType != nil && *channelType == "private" {
		http.Error(w, "Channel not accessible", http.StatusForbidden)
		return
	}

	// Verify membership.
	var isMember bool
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM guild_members WHERE guild_id = $1 AND user_id = $2)`,
		guildID, req.UserID,
	).Scan(&isMember); err != nil || !isMember {
		http.Error(w, "Not a guild member", http.StatusForbidden)
		return
	}

	// Verify message exists in the channel.
	var msgChannelID string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT channel_id FROM messages WHERE id = $1`, messageID,
	).Scan(&msgChannelID); err != nil || msgChannelID != channelID {
		http.Error(w, "Message not found", http.StatusNotFound)
		return
	}

	if _, err := ss.fed.pool.Exec(ctx,
		`INSERT INTO message_reactions (message_id, user_id, emoji, created_at)
		 VALUES ($1, $2, $3, now()) ON CONFLICT DO NOTHING`,
		messageID, req.UserID, req.Emoji); err != nil {
		ss.logger.Error("failed to add federated reaction", slog.String("error", err.Error()))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	evt := map[string]interface{}{
		"message_id": messageID, "channel_id": channelID, "guild_id": guildID,
		"user_id": req.UserID, "emoji": req.Emoji,
	}
	ss.bus.PublishChannelEvent(ctx, events.SubjectMessageReactionAdd, "MESSAGE_REACTION_ADD", channelID, evt)

	w.WriteHeader(http.StatusNoContent)
}

// HandleFederatedGuildReactionRemove removes a reaction on behalf of a federated user.
// POST /federation/v1/guilds/{guildID}/channels/{channelID}/messages/{messageID}/reactions/remove
func (ss *SyncService) HandleFederatedGuildReactionRemove(w http.ResponseWriter, r *http.Request) {
	signed, senderID, ok := ss.verifyFederationRequest(w, r)
	if !ok {
		return
	}

	guildID := chi.URLParam(r, "guildID")
	channelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")
	if guildID == "" || channelID == "" || messageID == "" {
		http.Error(w, "Missing path parameters", http.StatusBadRequest)
		return
	}

	var req federatedGuildReactionRequest
	if err := json.Unmarshal(signed.Payload, &req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}
	if req.UserID == "" || req.Emoji == "" {
		http.Error(w, "Missing user_id or emoji", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	if !ss.validateSenderUser(ctx, w, senderID, req.UserID) {
		return
	}

	// Verify channel belongs to guild and is not private.
	var channelGuildID *string
	var channelType *string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT guild_id, channel_type FROM channels WHERE id = $1`, channelID,
	).Scan(&channelGuildID, &channelType); err != nil || channelGuildID == nil || *channelGuildID != guildID {
		http.Error(w, "Channel not found in guild", http.StatusNotFound)
		return
	}
	if channelType != nil && *channelType == "private" {
		http.Error(w, "Channel not accessible", http.StatusForbidden)
		return
	}

	// Verify membership.
	var isMember bool
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM guild_members WHERE guild_id = $1 AND user_id = $2)`,
		guildID, req.UserID,
	).Scan(&isMember); err != nil || !isMember {
		http.Error(w, "Not a guild member", http.StatusForbidden)
		return
	}

	// Verify message exists in the channel.
	var msgChannelID string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT channel_id FROM messages WHERE id = $1`, messageID,
	).Scan(&msgChannelID); err != nil || msgChannelID != channelID {
		http.Error(w, "Message not found", http.StatusNotFound)
		return
	}

	if _, err := ss.fed.pool.Exec(ctx,
		`DELETE FROM message_reactions WHERE message_id = $1 AND user_id = $2 AND emoji = $3`,
		messageID, req.UserID, req.Emoji); err != nil {
		ss.logger.Error("failed to remove federated reaction", slog.String("error", err.Error()))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	evt := map[string]interface{}{
		"message_id": messageID, "channel_id": channelID, "guild_id": guildID,
		"user_id": req.UserID, "emoji": req.Emoji,
	}
	ss.bus.PublishChannelEvent(ctx, events.SubjectMessageReactionDel, "MESSAGE_REACTION_REMOVE", channelID, evt)

	w.WriteHeader(http.StatusNoContent)
}

// HandleFederatedGuildTyping handles a typing indicator from a federated user.
// POST /federation/v1/guilds/{guildID}/channels/{channelID}/typing
func (ss *SyncService) HandleFederatedGuildTyping(w http.ResponseWriter, r *http.Request) {
	signed, senderID, ok := ss.verifyFederationRequest(w, r)
	if !ok {
		return
	}

	guildID := chi.URLParam(r, "guildID")
	channelID := chi.URLParam(r, "channelID")
	if guildID == "" || channelID == "" {
		http.Error(w, "Missing path parameters", http.StatusBadRequest)
		return
	}

	var req federatedGuildTypingRequest
	if err := json.Unmarshal(signed.Payload, &req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}
	if req.UserID == "" {
		http.Error(w, "Missing user_id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	if !ss.validateSenderUser(ctx, w, senderID, req.UserID) {
		return
	}

	// Verify channel belongs to guild and is not private.
	var channelGuildID *string
	var channelType *string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT guild_id, channel_type FROM channels WHERE id = $1`, channelID,
	).Scan(&channelGuildID, &channelType); err != nil || channelGuildID == nil || *channelGuildID != guildID {
		http.Error(w, "Channel not found in guild", http.StatusNotFound)
		return
	}
	if channelType != nil && *channelType == "private" {
		http.Error(w, "Channel not accessible", http.StatusForbidden)
		return
	}

	// Verify membership.
	var isMember bool
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM guild_members WHERE guild_id = $1 AND user_id = $2)`,
		guildID, req.UserID,
	).Scan(&isMember); err != nil || !isMember {
		http.Error(w, "Not a guild member", http.StatusForbidden)
		return
	}

	// Publish typing event to NATS — no persistence needed.
	evt := map[string]interface{}{
		"channel_id": channelID,
		"guild_id":   guildID,
		"user_id":    req.UserID,
	}
	ss.bus.PublishChannelEvent(ctx, events.SubjectTypingStart, "TYPING_START", channelID, evt)

	w.WriteHeader(http.StatusNoContent)
}

// HandleProxyFederatedTyping proxies a typing indicator to a remote guild.
// POST /api/v1/federation/guilds/{guildID}/channels/{channelID}/typing
func (ss *SyncService) HandleProxyFederatedTyping(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	guildID := chi.URLParam(r, "guildID")
	channelID := chi.URLParam(r, "channelID")
	if guildID == "" || channelID == "" {
		http.Error(w, "Missing path parameters", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	var instanceDomain string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT i.domain FROM guilds g
		 JOIN instances i ON i.id = g.instance_id
		 WHERE g.id = $1`,
		guildID,
	).Scan(&instanceDomain); err != nil {
		http.Error(w, "Guild not found", http.StatusNotFound)
		return
	}

	remoteURL := fmt.Sprintf("https://%s/federation/v1/guilds/%s/channels/%s/typing",
		instanceDomain, guildID, channelID)
	_, statusCode, err := ss.signAndPost(ctx, remoteURL, federatedGuildTypingRequest{UserID: userID})
	if err != nil {
		http.Error(w, "Failed to contact remote instance", http.StatusBadGateway)
		return
	}

	w.WriteHeader(statusCode)
}

// ============================================================
// Helper methods
// ============================================================

// validateSenderDomain verifies that the claimed instance_domain in a federation
// payload matches the signed sender's actual domain. Returns the instance ID and
// true on success, or writes an HTTP error and returns false.
func (ss *SyncService) validateSenderDomain(ctx context.Context, w http.ResponseWriter, senderID, claimedDomain string) (string, bool) {
	var senderDomain string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT domain FROM instances WHERE id = $1`, senderID,
	).Scan(&senderDomain); err != nil {
		http.Error(w, "Unknown sender instance", http.StatusForbidden)
		return "", false
	}
	if senderDomain != claimedDomain {
		http.Error(w, "instance_domain does not match signed sender", http.StatusForbidden)
		return "", false
	}
	return senderID, true
}

// validateSenderUser verifies that the claimed user_id belongs to the signed
// sender's instance, preventing cross-instance user_id spoofing.
func (ss *SyncService) validateSenderUser(ctx context.Context, w http.ResponseWriter, senderID, userID string) bool {
	var instanceID string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT instance_id FROM users WHERE id = $1`, userID,
	).Scan(&instanceID); err != nil {
		http.Error(w, "Unknown user", http.StatusForbidden)
		return false
	}
	if instanceID != senderID {
		http.Error(w, "user_id does not match signed sender", http.StatusForbidden)
		return false
	}
	return true
}

// addInstanceToGuildChannelPeers registers a remote instance as a federation
// peer for non-private channels in a guild.
func (ss *SyncService) addInstanceToGuildChannelPeers(ctx context.Context, guildID, instanceID string) {
	// Only register peers for non-private channels to prevent private data leaks.
	rows, err := ss.fed.pool.Query(ctx,
		`SELECT id FROM channels WHERE guild_id = $1 AND (channel_type <> 'private' OR channel_type IS NULL)`, guildID)
	if err != nil {
		ss.logger.Warn("failed to query guild channels for peer addition", slog.String("error", err.Error()))
		return
	}
	defer rows.Close()
	for rows.Next() {
		var channelID string
		if rows.Scan(&channelID) == nil {
			if _, err := ss.fed.pool.Exec(ctx,
				`INSERT INTO federation_channel_peers (channel_id, instance_id)
				 VALUES ($1, $2) ON CONFLICT DO NOTHING`,
				channelID, instanceID); err != nil {
				ss.logger.Warn("failed to add channel peer",
					slog.String("channel_id", channelID), slog.String("error", err.Error()))
			}
		}
	}
}

func (ss *SyncService) buildGuildJoinResponse(ctx context.Context, guildID string) (*guildJoinResponse, error) {
	var resp guildJoinResponse
	err := ss.fed.pool.QueryRow(ctx,
		`SELECT g.id, g.name, g.icon_id, g.description, g.member_count, g.owner_id, COALESCE(g.instance_id, ''),
		        u.username, u.display_name, u.avatar_id
		 FROM guilds g
		 JOIN users u ON u.id = g.owner_id
		 WHERE g.id = $1`, guildID,
	).Scan(&resp.GuildID, &resp.Name, &resp.IconID, &resp.Description, &resp.MemberCount,
		&resp.OwnerID, &resp.InstanceID,
		&resp.OwnerUsername, &resp.OwnerDisplayName, &resp.OwnerAvatarID)
	if err != nil {
		return nil, fmt.Errorf("looking up guild: %w", err)
	}

	// Load guild categories and include them as synthetic "category" channel entries
	// so federated clients can group channels under their parent categories.
	channels := make([]map[string]interface{}, 0)
	catRows, err := ss.fed.pool.Query(ctx,
		`SELECT id, name, position FROM guild_categories WHERE guild_id = $1 ORDER BY position`, guildID)
	if err == nil {
		defer catRows.Close()
		for catRows.Next() {
			var id, name string
			var position int
			if catRows.Scan(&id, &name, &position) == nil {
				channels = append(channels, map[string]interface{}{
					"id": id, "channel_type": "category", "name": name, "topic": nil, "position": position, "category_id": nil,
				})
			}
		}
	}

	// Load non-private channels only (private channels require explicit access).
	channelRows, err := ss.fed.pool.Query(ctx,
		`SELECT id, channel_type, name, topic, position, category_id, parent_channel_id, encrypted FROM channels
		 WHERE guild_id = $1 AND (channel_type <> 'private' OR channel_type IS NULL)
		 ORDER BY position`, guildID)
	if err == nil {
		defer channelRows.Close()
		for channelRows.Next() {
			var id string
			var channelType *string
			var name, topic *string
			var position int
			var categoryID, parentChannelID *string
			var encrypted bool
			if channelRows.Scan(&id, &channelType, &name, &topic, &position, &categoryID, &parentChannelID, &encrypted) == nil {
				channels = append(channels, map[string]interface{}{
					"id": id, "channel_type": channelType, "name": name, "topic": topic,
					"position": position, "category_id": categoryID,
					"parent_channel_id": parentChannelID, "encrypted": encrypted,
				})
			}
		}
	}
	resp.ChannelsJSON, _ = json.Marshal(channels)
	if resp.ChannelsJSON == nil {
		resp.ChannelsJSON = json.RawMessage("[]")
	}

	// Load roles.
	roleRows, err := ss.fed.pool.Query(ctx,
		`SELECT id, name, color, position FROM roles WHERE guild_id = $1 ORDER BY position`, guildID)
	if err == nil {
		defer roleRows.Close()
		roles := make([]map[string]interface{}, 0)
		for roleRows.Next() {
			var id, name string
			var color *string
			var position int
			if roleRows.Scan(&id, &name, &color, &position) == nil {
				roles = append(roles, map[string]interface{}{
					"id": id, "name": name, "color": color, "position": position,
				})
			}
		}
		resp.RolesJSON, _ = json.Marshal(roles)
	}
	if resp.RolesJSON == nil {
		resp.RolesJSON = json.RawMessage("[]")
	}

	return &resp, nil
}

// updateFederatedGuildFromEvent updates real tables when receiving inbound
// guild-level events from remote instances (GUILD_UPDATE, CHANNEL_CREATE,
// CHANNEL_UPDATE, CHANNEL_DELETE, GUILD_MEMBER_ADD, GUILD_MEMBER_REMOVE,
// GUILD_DELETE).
func (ss *SyncService) updateFederatedGuildFromEvent(ctx context.Context, senderID, eventType, guildID string, data json.RawMessage) {
	// Verify the sender instance owns this guild to prevent a malicious peer
	// from modifying or deleting guilds it doesn't own.
	var guildInstanceID *string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT instance_id FROM guilds WHERE id = $1`, guildID,
	).Scan(&guildInstanceID); err != nil {
		ss.logger.Warn("federation event for unknown guild",
			slog.String("guild_id", guildID), slog.String("sender", senderID))
		return
	}
	if guildInstanceID == nil || *guildInstanceID != senderID {
		ownerStr := "<nil>"
		if guildInstanceID != nil {
			ownerStr = *guildInstanceID
		}
		ss.logger.Warn("federation event sender does not own guild",
			slog.String("guild_id", guildID), slog.String("sender", senderID),
			slog.String("owner", ownerStr))
		return
	}

	switch eventType {
	case "GUILD_UPDATE":
		var update struct {
			Name        *string `json:"name"`
			Description *string `json:"description"`
			IconID      *string `json:"icon_id"`
			MemberCount *int    `json:"member_count"`
		}
		if json.Unmarshal(data, &update) != nil {
			return
		}
		setClauses := []string{}
		args := []interface{}{}
		argN := 1
		if update.Name != nil {
			setClauses = append(setClauses, fmt.Sprintf("name = $%d", argN))
			args = append(args, *update.Name)
			argN++
		}
		if update.Description != nil {
			setClauses = append(setClauses, fmt.Sprintf("description = $%d", argN))
			args = append(args, *update.Description)
			argN++
		}
		if update.IconID != nil {
			setClauses = append(setClauses, fmt.Sprintf("icon_id = $%d", argN))
			args = append(args, *update.IconID)
			argN++
		}
		if update.MemberCount != nil {
			setClauses = append(setClauses, fmt.Sprintf("member_count = $%d", argN))
			args = append(args, *update.MemberCount)
			argN++
		}
		if len(setClauses) > 0 {
			query := fmt.Sprintf("UPDATE guilds SET %s WHERE id = $%d",
				strings.Join(setClauses, ", "), argN)
			args = append(args, guildID)
			if _, err := ss.fed.pool.Exec(ctx, query, args...); err != nil {
				ss.logger.Warn("failed to update federated guild from event",
					slog.String("guild_id", guildID), slog.String("error", err.Error()))
			}
		}

	case "CHANNEL_CREATE":
		var ch struct {
			ID              string  `json:"id"`
			ChannelType     string  `json:"channel_type"`
			Name            *string `json:"name"`
			Topic           *string `json:"topic"`
			Position        int     `json:"position"`
			CategoryID      *string `json:"category_id"`
			ParentChannelID *string `json:"parent_channel_id"`
			Encrypted       bool    `json:"encrypted"`
			GuildID         string  `json:"guild_id"`
		}
		if json.Unmarshal(data, &ch) != nil {
			return
		}
		if ch.ChannelType == "category" {
			if _, err := ss.fed.pool.Exec(ctx,
				`INSERT INTO guild_categories (id, guild_id, name, position)
				 VALUES ($1, $2, $3, $4) ON CONFLICT (id) DO UPDATE SET
				 name = EXCLUDED.name, position = EXCLUDED.position`,
				ch.ID, guildID, ch.Name, ch.Position); err != nil {
				ss.logger.Warn("failed to insert federated category from event",
					slog.String("id", ch.ID), slog.String("error", err.Error()))
			}
		} else {
			if _, err := ss.fed.pool.Exec(ctx,
				`INSERT INTO channels (id, guild_id, channel_type, name, topic, position, category_id, parent_channel_id, encrypted)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) ON CONFLICT (id) DO UPDATE SET
				 name = EXCLUDED.name, topic = EXCLUDED.topic, position = EXCLUDED.position,
				 category_id = EXCLUDED.category_id, parent_channel_id = EXCLUDED.parent_channel_id,
					 encrypted = EXCLUDED.encrypted`,
				ch.ID, guildID, ch.ChannelType, ch.Name, ch.Topic, ch.Position,
				ch.CategoryID, ch.ParentChannelID, ch.Encrypted); err != nil {
				ss.logger.Warn("failed to insert federated channel from event",
					slog.String("id", ch.ID), slog.String("error", err.Error()))
			}
		}

	case "CHANNEL_UPDATE":
		var ch struct {
			ID              string  `json:"id"`
			ChannelType     string  `json:"channel_type"`
			Name            *string `json:"name"`
			Topic           *string `json:"topic"`
			Position        *int    `json:"position"`
			Encrypted       *bool   `json:"encrypted"`
			CategoryID      *string `json:"category_id"`
			ParentChannelID *string `json:"parent_channel_id"`
		}
		if json.Unmarshal(data, &ch) != nil {
			return
		}
		if ch.ChannelType == "category" {
			setClauses := []string{}
			args := []interface{}{}
			argN := 1
			if ch.Name != nil {
				setClauses = append(setClauses, fmt.Sprintf("name = $%d", argN))
				args = append(args, *ch.Name)
				argN++
			}
			if ch.Position != nil {
				setClauses = append(setClauses, fmt.Sprintf("position = $%d", argN))
				args = append(args, *ch.Position)
				argN++
			}
			if len(setClauses) > 0 {
				query := fmt.Sprintf("UPDATE guild_categories SET %s WHERE id = $%d",
					strings.Join(setClauses, ", "), argN)
				args = append(args, ch.ID)
				ss.fed.pool.Exec(ctx, query, args...)
			}
		} else {
			setClauses := []string{}
			args := []interface{}{}
			argN := 1
			if ch.Name != nil {
				setClauses = append(setClauses, fmt.Sprintf("name = $%d", argN))
				args = append(args, *ch.Name)
				argN++
			}
			if ch.Topic != nil {
				setClauses = append(setClauses, fmt.Sprintf("topic = $%d", argN))
				args = append(args, *ch.Topic)
				argN++
			}
			if ch.Position != nil {
				setClauses = append(setClauses, fmt.Sprintf("position = $%d", argN))
				args = append(args, *ch.Position)
				argN++
			}
			if ch.Encrypted != nil {
				setClauses = append(setClauses, fmt.Sprintf("encrypted = $%d", argN))
				args = append(args, *ch.Encrypted)
				argN++
			}
			if ch.CategoryID != nil {
				setClauses = append(setClauses, fmt.Sprintf("category_id = $%d", argN))
				args = append(args, *ch.CategoryID)
				argN++
			}
			if ch.ParentChannelID != nil {
				setClauses = append(setClauses, fmt.Sprintf("parent_channel_id = $%d", argN))
				args = append(args, *ch.ParentChannelID)
				argN++
			}
			if len(setClauses) > 0 {
				query := fmt.Sprintf("UPDATE channels SET %s WHERE id = $%d",
					strings.Join(setClauses, ", "), argN)
				args = append(args, ch.ID)
				ss.fed.pool.Exec(ctx, query, args...)
			}
		}

	case "CHANNEL_DELETE":
		var ch struct {
			ID          string `json:"id"`
			ChannelType string `json:"channel_type"`
		}
		if json.Unmarshal(data, &ch) != nil {
			return
		}
		if ch.ChannelType == "category" {
			ss.fed.pool.Exec(ctx, `DELETE FROM guild_categories WHERE id = $1`, ch.ID)
		} else {
			ss.fed.pool.Exec(ctx, `DELETE FROM channels WHERE id = $1`, ch.ID)
		}

	case "GUILD_MEMBER_ADD":
		var member struct {
			GuildID     string  `json:"guild_id"`
			UserID      string  `json:"user_id"`
			Username    string  `json:"username"`
			DisplayName *string `json:"display_name"`
			AvatarID    *string `json:"avatar_id"`
		}
		if json.Unmarshal(data, &member) != nil || member.UserID == "" {
			return
		}
		// Create or update user stub with the sender's instance_id so the user
		// is correctly marked as federated (instance_id != NULL).
		ss.ensureRemoteUserStub(ctx, senderID, federatedUserInfo{
			ID:          member.UserID,
			Username:    member.Username,
			DisplayName: member.DisplayName,
			AvatarID:    member.AvatarID,
		})
		// Insert the member into guild_members (idempotent).
		if _, err := ss.fed.pool.Exec(ctx,
			`INSERT INTO guild_members (guild_id, user_id, joined_at)
			 VALUES ($1, $2, now()) ON CONFLICT DO NOTHING`,
			guildID, member.UserID); err != nil {
			ss.logger.Warn("failed to insert federated guild member from event",
				slog.String("guild_id", guildID), slog.String("user_id", member.UserID),
				slog.String("error", err.Error()))
		}

	case "GUILD_MEMBER_REMOVE":
		var member struct {
			UserID string `json:"user_id"`
		}
		if json.Unmarshal(data, &member) != nil || member.UserID == "" {
			return
		}
		if _, err := ss.fed.pool.Exec(ctx,
			`DELETE FROM guild_members WHERE guild_id = $1 AND user_id = $2`,
			guildID, member.UserID); err != nil {
			ss.logger.Warn("failed to remove federated guild member from event",
				slog.String("guild_id", guildID), slog.String("user_id", member.UserID),
				slog.String("error", err.Error()))
		}

	case "GUILD_DELETE":
		// Cascading deletes via FK will clean up channels, categories, roles, members.
		if _, err := ss.fed.pool.Exec(ctx, `DELETE FROM guilds WHERE id = $1`, guildID); err != nil {
			ss.logger.Warn("failed to delete federated guild from event",
				slog.String("guild_id", guildID), slog.String("error", err.Error()))
		}
	}
}

// ============================================================
// Proxy endpoints (on the user's HOME instance, auth required)
// ============================================================

// HandleProxyJoinFederatedGuild proxies a join request to a remote guild's instance.
// POST /api/v1/federation/guilds/join
func (ss *SyncService) HandleProxyJoinFederatedGuild(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		InstanceDomain string `json:"instance_domain"`
		GuildID        string `json:"guild_id"`
		InviteCode     string `json:"invite_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.InstanceDomain == "" || (req.GuildID == "" && req.InviteCode == "") {
		http.Error(w, "Missing instance_domain and guild_id or invite_code", http.StatusBadRequest)
		return
	}
	if err := ValidateFederationDomain(req.InstanceDomain); err != nil {
		http.Error(w, "Invalid domain", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Look up local user info.
	var username string
	var displayName, avatarID *string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT username, display_name, avatar_id FROM users WHERE id = $1`, userID,
	).Scan(&username, &displayName, &avatarID); err != nil {
		http.Error(w, "User not found", http.StatusInternalServerError)
		return
	}

	var remoteURL string
	var payload interface{}

	if req.InviteCode != "" {
		remoteURL = fmt.Sprintf("https://%s/federation/v1/guilds/invite-accept", req.InstanceDomain)
		payload = federatedGuildInviteRequest{
			InviteCode: req.InviteCode, UserID: userID, Username: username,
			DisplayName: displayName, AvatarID: avatarID, InstanceDomain: ss.fed.domain,
		}
	} else {
		remoteURL = fmt.Sprintf("https://%s/federation/v1/guilds/%s/join", req.InstanceDomain, req.GuildID)
		payload = federatedGuildJoinRequest{
			UserID: userID, Username: username,
			DisplayName: displayName, AvatarID: avatarID, InstanceDomain: ss.fed.domain,
		}
	}

	respBody, statusCode, err := ss.signAndPost(ctx, remoteURL, payload)
	if err != nil {
		ss.logger.Warn("failed to proxy guild join", slog.String("error", err.Error()))
		http.Error(w, "Failed to contact remote instance", http.StatusBadGateway)
		return
	}

	if statusCode != http.StatusCreated && statusCode != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		w.Write(respBody)
		return
	}

	// Store federated guild data in real tables.
	var joinResp guildJoinResponse
	if err := json.Unmarshal(respBody, &joinResp); err != nil || joinResp.GuildID == "" {
		ss.logger.Warn("failed to parse guild join response", slog.String("error", fmt.Sprintf("%v", err)))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write(respBody)
		return
	}

	// Ensure remote instance is registered locally.
	var remoteInstanceID string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT id FROM instances WHERE domain = $1`, req.InstanceDomain,
	).Scan(&remoteInstanceID); err != nil {
		disc, discErr := DiscoverInstance(ctx, req.InstanceDomain)
		if discErr != nil {
			ss.logger.Error("failed to discover remote instance for guild join",
				slog.String("domain", req.InstanceDomain), slog.String("error", discErr.Error()))
			http.Error(w, "Failed to register remote instance", http.StatusBadGateway)
			return
		}
		ss.fed.RegisterRemoteInstance(ctx, disc)
		remoteInstanceID = disc.InstanceID
	}

	// Ensure guild owner exists as a local user stub (required by guilds.owner_id FK).
	ss.ensureRemoteUserStub(ctx, remoteInstanceID, federatedUserInfo{
		ID:          joinResp.OwnerID,
		Username:    joinResp.OwnerUsername,
		DisplayName: joinResp.OwnerDisplayName,
		AvatarID:    joinResp.OwnerAvatarID,
	})

	// Insert guild into real guilds table (idempotent via ON CONFLICT DO UPDATE).
	if _, err := ss.fed.pool.Exec(ctx,
		`INSERT INTO guilds (id, instance_id, owner_id, name, description, icon_id, member_count, discoverable, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, false, now())
		 ON CONFLICT (id) DO UPDATE SET
		   name = EXCLUDED.name, description = EXCLUDED.description, icon_id = EXCLUDED.icon_id,
		   member_count = EXCLUDED.member_count, owner_id = EXCLUDED.owner_id`,
		joinResp.GuildID, remoteInstanceID, joinResp.OwnerID,
		joinResp.Name, joinResp.Description, joinResp.IconID, joinResp.MemberCount,
	); err != nil {
		ss.logger.Error("failed to insert federated guild into guilds table",
			slog.String("guild_id", joinResp.GuildID), slog.String("error", err.Error()))
		// Still return success to the user — the remote join succeeded.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{"data": joinResp})
		return
	}

	// Parse and insert channels (categories go to guild_categories, others to channels).
	var channels []struct {
		ID              string  `json:"id"`
		ChannelType     *string `json:"channel_type"`
		Name            *string `json:"name"`
		Topic           *string `json:"topic"`
		Position        int     `json:"position"`
		CategoryID      *string `json:"category_id"`
		ParentChannelID *string `json:"parent_channel_id"`
		Encrypted       bool    `json:"encrypted"`
	}
	if json.Unmarshal(joinResp.ChannelsJSON, &channels) == nil {
		// Insert categories first (channels may reference them via category_id FK).
		for _, ch := range channels {
			if ch.ChannelType != nil && *ch.ChannelType == "category" {
				if _, err := ss.fed.pool.Exec(ctx,
					`INSERT INTO guild_categories (id, guild_id, name, position)
					 VALUES ($1, $2, $3, $4) ON CONFLICT (id) DO UPDATE SET
					 name = EXCLUDED.name, position = EXCLUDED.position`,
					ch.ID, joinResp.GuildID, ch.Name, ch.Position); err != nil {
					ss.logger.Warn("failed to insert federated category",
						slog.String("id", ch.ID), slog.String("error", err.Error()))
				}
			}
		}
		// Then insert non-category channels.
		for _, ch := range channels {
			if ch.ChannelType == nil || *ch.ChannelType != "category" {
				chType := "text"
				if ch.ChannelType != nil {
					chType = *ch.ChannelType
				}
				if _, err := ss.fed.pool.Exec(ctx,
					`INSERT INTO channels (id, guild_id, channel_type, name, topic, position, category_id, parent_channel_id, encrypted)
					 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) ON CONFLICT (id) DO UPDATE SET
					 name = EXCLUDED.name, topic = EXCLUDED.topic, position = EXCLUDED.position,
					 category_id = EXCLUDED.category_id, parent_channel_id = EXCLUDED.parent_channel_id,
					 encrypted = EXCLUDED.encrypted`,
					ch.ID, joinResp.GuildID, chType, ch.Name, ch.Topic, ch.Position,
					ch.CategoryID, ch.ParentChannelID, ch.Encrypted); err != nil {
					ss.logger.Warn("failed to insert federated channel",
						slog.String("id", ch.ID), slog.String("error", err.Error()))
				}
			}
		}
	}

	// Parse and insert roles.
	var roles []struct {
		ID       string  `json:"id"`
		Name     string  `json:"name"`
		Color    *string `json:"color"`
		Position int     `json:"position"`
	}
	if json.Unmarshal(joinResp.RolesJSON, &roles) == nil {
		for _, role := range roles {
			if _, err := ss.fed.pool.Exec(ctx,
				`INSERT INTO roles (id, guild_id, name, color, position)
				 VALUES ($1, $2, $3, $4, $5) ON CONFLICT (id) DO UPDATE SET
				 name = EXCLUDED.name, color = EXCLUDED.color, position = EXCLUDED.position`,
				role.ID, joinResp.GuildID, role.Name, role.Color, role.Position); err != nil {
				ss.logger.Warn("failed to insert federated role",
					slog.String("id", role.ID), slog.String("error", err.Error()))
			}
		}
	}

	// Add the joining user as a guild member (idempotent).
	if _, err := ss.fed.pool.Exec(ctx,
		`INSERT INTO guild_members (guild_id, user_id, joined_at)
		 VALUES ($1, $2, now()) ON CONFLICT DO NOTHING`,
		joinResp.GuildID, userID); err != nil {
		ss.logger.Warn("failed to insert local user as federated guild member",
			slog.String("guild_id", joinResp.GuildID), slog.String("error", err.Error()))
	}

	// Register this instance as a peer for the guild's channels so events route here.
	ss.addInstanceToGuildChannelPeers(ctx, joinResp.GuildID, ss.fed.instanceID)

	// Wrap in API response envelope for frontend compatibility.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"data": joinResp})
}

// HandleProxyLeaveFederatedGuild proxies a leave request to a remote guild's instance.
// POST /api/v1/federation/guilds/{guildID}/leave
func (ss *SyncService) HandleProxyLeaveFederatedGuild(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	guildID := chi.URLParam(r, "guildID")
	if guildID == "" {
		http.Error(w, "Missing guild ID", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Look up the remote instance domain from the real guilds table.
	var instanceDomain string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT i.domain FROM guilds g
		 JOIN instances i ON i.id = g.instance_id
		 WHERE g.id = $1`,
		guildID,
	).Scan(&instanceDomain); err != nil {
		http.Error(w, "Guild not found", http.StatusNotFound)
		return
	}

	remoteURL := fmt.Sprintf("https://%s/federation/v1/guilds/%s/leave", instanceDomain, guildID)
	respBody, statusCode, err := ss.signAndPost(ctx, remoteURL, federatedGuildLeaveRequest{UserID: userID})
	if err != nil {
		http.Error(w, "Failed to contact remote instance", http.StatusBadGateway)
		return
	}

	if statusCode != http.StatusNoContent && statusCode != http.StatusOK {
		w.WriteHeader(statusCode)
		w.Write(respBody)
		return
	}

	// Remove the local user from guild_members.
	ss.fed.pool.Exec(ctx,
		`DELETE FROM guild_members WHERE guild_id = $1 AND user_id = $2`, guildID, userID)

	// If no local members remain in this federated guild, clean up the local guild data.
	// Local users have NULL instance_id, so use IS NULL instead of matching instance_id.
	var remainingLocalMembers int
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM guild_members gm
		 JOIN users u ON u.id = gm.user_id
		 WHERE gm.guild_id = $1 AND u.instance_id IS NULL`,
		guildID,
	).Scan(&remainingLocalMembers); err != nil {
		ss.logger.Warn("failed to count remaining local members in federated guild",
			slog.String("guild_id", guildID), slog.String("error", err.Error()))
	} else if remainingLocalMembers == 0 {
		// No local users remain — remove channel peers and optionally the guild.
		ss.fed.pool.Exec(ctx,
			`DELETE FROM federation_channel_peers
			 WHERE instance_id = $1 AND channel_id IN (SELECT id FROM channels WHERE guild_id = $2)`,
			ss.fed.instanceID, guildID)
		// Delete the federated guild and its cascaded data (channels, categories, roles, members).
		// Only delete if instance_id IS NOT NULL to prevent accidentally deleting local guilds.
		ss.fed.pool.Exec(ctx, `DELETE FROM guilds WHERE id = $1 AND instance_id IS NOT NULL`, guildID)
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleProxyGetFederatedGuildMessages proxies message fetch to a remote guild.
// GET /api/v1/federation/guilds/{guildID}/channels/{channelID}/messages
func (ss *SyncService) HandleProxyGetFederatedGuildMessages(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	guildID := chi.URLParam(r, "guildID")
	channelID := chi.URLParam(r, "channelID")
	if guildID == "" || channelID == "" {
		http.Error(w, "Missing guild or channel ID", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	var instanceDomain string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT i.domain FROM guilds g
		 JOIN instances i ON i.id = g.instance_id
		 WHERE g.id = $1`,
		guildID,
	).Scan(&instanceDomain); err != nil {
		http.Error(w, "Guild not found", http.StatusNotFound)
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	payload := federatedGuildMessagesRequest{
		UserID: userID, Before: r.URL.Query().Get("before"),
		After: r.URL.Query().Get("after"), Limit: limit,
	}

	remoteURL := fmt.Sprintf("https://%s/federation/v1/guilds/%s/channels/%s/messages",
		instanceDomain, guildID, channelID)
	respBody, statusCode, err := ss.signAndPost(ctx, remoteURL, payload)
	if err != nil {
		http.Error(w, "Failed to contact remote instance", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	w.Write(respBody)
}

// HandleProxyPostFederatedGuildMessage proxies message creation to a remote guild.
// POST /api/v1/federation/guilds/{guildID}/channels/{channelID}/messages
func (ss *SyncService) HandleProxyPostFederatedGuildMessage(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	guildID := chi.URLParam(r, "guildID")
	channelID := chi.URLParam(r, "channelID")
	if guildID == "" || channelID == "" {
		http.Error(w, "Missing guild or channel ID", http.StatusBadRequest)
		return
	}

	var localReq struct {
		Content    string   `json:"content"`
		Nonce      string   `json:"nonce"`
		ReplyToIDs []string `json:"reply_to_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&localReq); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if localReq.Content == "" {
		http.Error(w, "Message content required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	var instanceDomain string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT i.domain FROM guilds g
		 JOIN instances i ON i.id = g.instance_id
		 WHERE g.id = $1`,
		guildID,
	).Scan(&instanceDomain); err != nil {
		http.Error(w, "Guild not found", http.StatusNotFound)
		return
	}

	payload := federatedGuildPostMessageRequest{
		UserID: userID, Content: localReq.Content, Nonce: localReq.Nonce,
		ReplyToIDs: localReq.ReplyToIDs,
	}

	remoteURL := fmt.Sprintf("https://%s/federation/v1/guilds/%s/channels/%s/messages/create",
		instanceDomain, guildID, channelID)
	respBody, statusCode, err := ss.signAndPost(ctx, remoteURL, payload)
	if err != nil {
		http.Error(w, "Failed to contact remote instance", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	w.Write(respBody)
}

// HandleFederatedGuildDiscover returns discoverable guilds to a verified federated peer.
// POST /federation/v1/guilds/discover?q=&tag=&limit=
// Requires federation signature verification — only authenticated peers can discover.
func (ss *SyncService) HandleFederatedGuildDiscover(w http.ResponseWriter, r *http.Request) {
	// Verify the sender is an authenticated federation peer.
	_, _, ok := ss.verifyFederationRequest(w, r)
	if !ok {
		return
	}

	query := r.URL.Query().Get("q")
	tag := r.URL.Query().Get("tag")
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	baseSQL := `SELECT g.id, g.name, g.description, g.icon_id, g.banner_id,
	            g.tags, g.member_count, g.created_at
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
	baseSQL += fmt.Sprintf(` ORDER BY g.member_count DESC LIMIT $%d`, argN)
	args = append(args, limit)

	rows, err := ss.fed.pool.Query(r.Context(), baseSQL, args...)
	if err != nil {
		http.Error(w, "Failed to query guilds", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type discoverGuild struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		Description *string  `json:"description,omitempty"`
		IconID      *string  `json:"icon_id,omitempty"`
		BannerID    *string  `json:"banner_id,omitempty"`
		Tags        []string `json:"tags,omitempty"`
		MemberCount int      `json:"member_count"`
		CreatedAt   string   `json:"created_at"`
	}

	guilds := make([]discoverGuild, 0)
	for rows.Next() {
		var g discoverGuild
		var tags []string
		var createdAt time.Time
		if err := rows.Scan(&g.ID, &g.Name, &g.Description, &g.IconID, &g.BannerID,
			&tags, &g.MemberCount, &createdAt); err != nil {
			http.Error(w, "Failed to read guilds", http.StatusInternalServerError)
			return
		}
		g.Tags = tags
		g.CreatedAt = createdAt.Format(time.RFC3339)
		guilds = append(guilds, g)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(guilds)
}

// HandleProxyDiscoverRemoteGuilds proxies a discover request to a remote peer's instance.
// GET /api/v1/federation/peers/{peerID}/guilds?q=&tag=&limit=
func (ss *SyncService) HandleProxyDiscoverRemoteGuilds(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	peerID := chi.URLParam(r, "peerID")
	if peerID == "" {
		http.Error(w, "Missing peer ID", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Verify peer exists and is active.
	var peerDomain string
	var peerStatus string
	err := ss.fed.pool.QueryRow(ctx,
		`SELECT i.domain, fp.status FROM federation_peers fp
		 JOIN instances i ON i.id = fp.peer_id
		 WHERE fp.instance_id = $1 AND fp.peer_id = $2`,
		ss.fed.instanceID, peerID,
	).Scan(&peerDomain, &peerStatus)
	if err != nil {
		http.Error(w, "Federation peer not found", http.StatusNotFound)
		return
	}
	if peerStatus != "active" {
		http.Error(w, "Federation peer is not active", http.StatusForbidden)
		return
	}

	// Build query string for remote instance's discover endpoint.
	remoteQuery := neturl.Values{}
	if q := r.URL.Query().Get("q"); q != "" {
		remoteQuery.Set("q", q)
	}
	if tag := r.URL.Query().Get("tag"); tag != "" {
		remoteQuery.Set("tag", tag)
	}
	limit := "50"
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = strconv.Itoa(n)
		}
	}
	remoteQuery.Set("limit", limit)

	remoteURL := fmt.Sprintf("https://%s/federation/v1/guilds/discover?%s", peerDomain, remoteQuery.Encode())

	// Proxy the discover request to the remote instance with federation signing.
	payload := map[string]string{"action": "discover"}
	respBody, statusCode, err := ss.signAndPost(ctx, remoteURL, payload)
	if err != nil {
		ss.logger.Warn("failed to proxy remote guild discover",
			slog.String("peer", peerDomain), slog.String("error", err.Error()))
		http.Error(w, "Failed to contact remote instance", http.StatusBadGateway)
		return
	}

	if statusCode != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		w.Write(respBody)
		return
	}

	// Parse the remote response and inject instance_domain into each guild.
	var remoteGuilds []json.RawMessage
	if err := json.Unmarshal(respBody, &remoteGuilds); err != nil {
		// Try unwrapping from data envelope.
		var envelope struct {
			Data []json.RawMessage `json:"data"`
		}
		if err2 := json.Unmarshal(respBody, &envelope); err2 != nil {
			// Pass through as-is.
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(respBody)
			return
		}
		remoteGuilds = envelope.Data
	}

	// Enrich each guild with instance_domain.
	enriched := make([]map[string]interface{}, 0, len(remoteGuilds))
	for _, raw := range remoteGuilds {
		var guild map[string]interface{}
		if json.Unmarshal(raw, &guild) == nil {
			guild["instance_domain"] = peerDomain
			enriched = append(enriched, guild)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": enriched})
}

// HandleGetPublicFederationPeers returns basic info for all active federation peers.
// GET /api/v1/federation/peers/public
func (ss *SyncService) HandleGetPublicFederationPeers(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	rows, err := ss.fed.pool.Query(r.Context(),
		`SELECT fp.peer_id, i.domain, i.name, fp.status, fp.established_at
		 FROM federation_peers fp
		 JOIN instances i ON i.id = fp.peer_id
		 WHERE fp.instance_id = $1 AND fp.status = 'active'
		 ORDER BY i.domain`, ss.fed.instanceID)
	if err != nil {
		http.Error(w, "Failed to query peers", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type publicPeer struct {
		ID            string    `json:"id"`
		Domain        string    `json:"domain"`
		Name          *string   `json:"name,omitempty"`
		Status        string    `json:"status"`
		EstablishedAt time.Time `json:"established_at"`
	}

	peers := []publicPeer{}
	for rows.Next() {
		var p publicPeer
		if err := rows.Scan(&p.ID, &p.Domain, &p.Name, &p.Status, &p.EstablishedAt); err != nil {
			http.Error(w, "Failed to read peers", http.StatusInternalServerError)
			return
		}
		peers = append(peers, p)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": peers})
}

// HandleProxyGetFederatedGuildMembers proxies member list fetch to a remote guild.
// The remote returns flat member objects; this handler transforms them into the
// GuildMember shape the frontend expects (with a nested "user" object).
// GET /api/v1/federation/guilds/{guildID}/members
func (ss *SyncService) HandleProxyGetFederatedGuildMembers(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	guildID := chi.URLParam(r, "guildID")
	if guildID == "" {
		http.Error(w, "Missing guild ID", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	var instanceDomain string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT i.domain FROM guilds g
		 JOIN instances i ON i.id = g.instance_id
		 WHERE g.id = $1`,
		guildID,
	).Scan(&instanceDomain); err != nil {
		http.Error(w, "Guild not found", http.StatusNotFound)
		return
	}

	remoteURL := fmt.Sprintf("https://%s/federation/v1/guilds/%s/members", instanceDomain, guildID)
	respBody, statusCode, err := ss.signAndPost(ctx, remoteURL, federatedGuildMembersRequest{UserID: userID})
	if err != nil {
		http.Error(w, "Failed to contact remote instance", http.StatusBadGateway)
		return
	}

	if statusCode != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		w.Write(respBody)
		return
	}

	// Parse the flat remote response and transform into GuildMember shape.
	var remoteResp struct {
		Data []struct {
			UserID         string          `json:"user_id"`
			Username       string          `json:"username"`
			DisplayName    *string         `json:"display_name"`
			AvatarID       *string         `json:"avatar_id"`
			InstanceDomain string          `json:"instance_domain"`
			RoleIDs        json.RawMessage `json:"role_ids"`
			JoinedAt       time.Time       `json:"joined_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &remoteResp); err != nil {
		// Fallback: return raw response if parsing fails.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		w.Write(respBody)
		return
	}

	// Build domain → instance_id map for avatar proxy URLs.
	domainSet := map[string]bool{}
	for _, m := range remoteResp.Data {
		domain := m.InstanceDomain
		if domain == "" {
			domain = instanceDomain
		}
		domainSet[domain] = true
	}
	domains := make([]string, 0, len(domainSet))
	for d := range domainSet {
		domains = append(domains, d)
	}
	domainToID := map[string]string{}
	if len(domains) > 0 {
		rows, err := ss.fed.pool.Query(ctx,
			`SELECT domain, id FROM instances WHERE domain = ANY($1)`, domains)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var d, id string
				if rows.Scan(&d, &id) == nil {
					domainToID[d] = id
				}
			}
		}
	}

	members := make([]map[string]interface{}, 0, len(remoteResp.Data))
	for _, m := range remoteResp.Data {
		// Parse role_ids array.
		var roles []string
		if len(m.RoleIDs) > 0 {
			json.Unmarshal(m.RoleIDs, &roles)
		}
		if roles == nil {
			roles = []string{}
		}

		// Determine the instance_domain to use: if empty, it's a local user
		// on the remote instance, so use the remote instance domain.
		domain := m.InstanceDomain
		if domain == "" {
			domain = instanceDomain
		}

		user := map[string]interface{}{
			"id":              m.UserID,
			"username":        m.Username,
			"display_name":    m.DisplayName,
			"avatar_id":       m.AvatarID,
			"instance_domain": domain,
		}
		if id := domainToID[domain]; id != "" {
			user["instance_id"] = id
		}
		members = append(members, map[string]interface{}{
			"guild_id":  guildID,
			"user_id":   m.UserID,
			"nickname":  nil,
			"avatar_id": nil,
			"joined_at": m.JoinedAt,
			"roles":     roles,
			"user":      user,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": members})
}

// HandleProxyAddFederatedReaction proxies adding a reaction to a remote guild message.
// PUT /api/v1/federation/guilds/{guildID}/channels/{channelID}/messages/{messageID}/reactions/{emoji}
func (ss *SyncService) HandleProxyAddFederatedReaction(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	guildID := chi.URLParam(r, "guildID")
	channelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")
	emoji := chi.URLParam(r, "emoji")
	if guildID == "" || channelID == "" || messageID == "" || emoji == "" {
		http.Error(w, "Missing path parameters", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	var instanceDomain string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT i.domain FROM guilds g
		 JOIN instances i ON i.id = g.instance_id
		 WHERE g.id = $1`,
		guildID,
	).Scan(&instanceDomain); err != nil {
		http.Error(w, "Guild not found", http.StatusNotFound)
		return
	}

	remoteURL := fmt.Sprintf("https://%s/federation/v1/guilds/%s/channels/%s/messages/%s/reactions",
		instanceDomain, guildID, channelID, messageID)
	respBody, statusCode, err := ss.signAndPost(ctx, remoteURL, federatedGuildReactionRequest{
		UserID: userID, Emoji: emoji,
	})
	if err != nil {
		http.Error(w, "Failed to contact remote instance", http.StatusBadGateway)
		return
	}

	if statusCode == http.StatusNoContent {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	w.Write(respBody)
}

// HandleProxyRemoveFederatedReaction proxies removing a reaction from a remote guild message.
// DELETE /api/v1/federation/guilds/{guildID}/channels/{channelID}/messages/{messageID}/reactions/{emoji}
func (ss *SyncService) HandleProxyRemoveFederatedReaction(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	guildID := chi.URLParam(r, "guildID")
	channelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")
	emoji := chi.URLParam(r, "emoji")
	if guildID == "" || channelID == "" || messageID == "" || emoji == "" {
		http.Error(w, "Missing path parameters", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	var instanceDomain string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT i.domain FROM guilds g
		 JOIN instances i ON i.id = g.instance_id
		 WHERE g.id = $1`,
		guildID,
	).Scan(&instanceDomain); err != nil {
		http.Error(w, "Guild not found", http.StatusNotFound)
		return
	}

	// The remote-facing handler uses POST for deletion (federation always signs POST payloads),
	// but we route differently by using the remove endpoint path.
	remoteURL := fmt.Sprintf("https://%s/federation/v1/guilds/%s/channels/%s/messages/%s/reactions/remove",
		instanceDomain, guildID, channelID, messageID)
	respBody, statusCode, err := ss.signAndPost(ctx, remoteURL, federatedGuildReactionRequest{
		UserID: userID, Emoji: emoji,
	})
	if err != nil {
		http.Error(w, "Failed to contact remote instance", http.StatusBadGateway)
		return
	}

	if statusCode == http.StatusNoContent {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	w.Write(respBody)
}

// hasChannelPermission computes a user's effective permissions for a channel
// and checks whether the required permission bits are set. Used by federation
// handlers to enforce ViewChannel/ReadHistory before serving content.
func (ss *SyncService) hasChannelPermission(ctx context.Context, guildID, channelID, userID string, perm uint64) bool {
	// Guild owner has all permissions.
	var ownerID string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT owner_id FROM guilds WHERE id = $1`, guildID,
	).Scan(&ownerID); err != nil {
		return false
	}
	if userID == ownerID {
		return true
	}

	// Start with guild default permissions.
	var defaultPerms int64
	ss.fed.pool.QueryRow(ctx,
		`SELECT default_permissions FROM guilds WHERE id = $1`, guildID,
	).Scan(&defaultPerms)
	computed := uint64(defaultPerms)

	// Apply role allow/deny.
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
			computed |= uint64(allow)
			computed &^= uint64(deny)
		}
	}

	// Administrator bypasses everything.
	if computed&permissions.Administrator != 0 {
		return true
	}

	// Apply channel-level permission overrides.
	overrideRows, _ := ss.fed.pool.Query(ctx,
		`SELECT target_type, target_id, allow_bits, deny_bits
		 FROM channel_permission_overrides
		 WHERE channel_id = $1`,
		channelID,
	)
	if overrideRows != nil {
		defer overrideRows.Close()

		// Collect member role IDs for matching role overrides.
		memberRoles := map[string]bool{}
		roleRows, _ := ss.fed.pool.Query(ctx,
			`SELECT role_id FROM member_roles WHERE guild_id = $1 AND user_id = $2`,
			guildID, userID,
		)
		if roleRows != nil {
			defer roleRows.Close()
			for roleRows.Next() {
				var rid string
				roleRows.Scan(&rid)
				memberRoles[rid] = true
			}
		}

		var roleAllow, roleDeny uint64
		var memberAllow, memberDeny uint64
		for overrideRows.Next() {
			var targetType, targetID string
			var allow, deny int64
			overrideRows.Scan(&targetType, &targetID, &allow, &deny)
			if targetType == "role" && memberRoles[targetID] {
				roleAllow |= uint64(allow)
				roleDeny |= uint64(deny)
			} else if targetType == "member" && targetID == userID {
				memberAllow |= uint64(allow)
				memberDeny |= uint64(deny)
			}
		}
		computed |= roleAllow
		computed &^= roleDeny
		computed |= memberAllow
		computed &^= memberDeny
	}

	// No ViewChannel means no permissions at all.
	if computed&permissions.ViewChannel == 0 {
		return false
	}

	return computed&perm == perm
}

// signAndPost signs a payload with the federation service and POSTs to a URL.
// Returns the response body, status code, and any error.
// The URL must use HTTPS and the host must pass SSRF validation.
func (ss *SyncService) signAndPost(ctx context.Context, targetURL string, payload interface{}) ([]byte, int, error) {
	parsed, err := neturl.Parse(targetURL)
	if err != nil || parsed.Scheme != "https" {
		return nil, 0, fmt.Errorf("invalid federation URL: %s", targetURL)
	}
	if err := ValidateFederationDomain(parsed.Hostname()); err != nil {
		return nil, 0, fmt.Errorf("SSRF validation failed for %s: %w", parsed.Hostname(), err)
	}

	signed, err := ss.fed.Sign(payload)
	if err != nil {
		return nil, 0, fmt.Errorf("signing payload: %w", err)
	}
	body, err := json.Marshal(signed)
	if err != nil {
		return nil, 0, fmt.Errorf("marshaling signed payload: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "AmityVox/1.0 (+federation)")

	resp, err := ss.client.Do(httpReq)
	if err != nil {
		return nil, 0, fmt.Errorf("sending request to %s: %w", targetURL, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("reading response from %s: %w", targetURL, err)
	}
	return respBody, resp.StatusCode, nil
}

// HandleProxyEnsureFederatedUser creates or updates a local user stub for a
// remote user so that local operations (e.g. creating a DM) can succeed.
// POST /api/v1/federation/users/ensure
func (ss *SyncService) HandleProxyEnsureFederatedUser(w http.ResponseWriter, r *http.Request) {
	callerID := auth.UserIDFromContext(r.Context())
	if callerID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		UserID         string  `json:"user_id"`
		InstanceDomain string  `json:"instance_domain"`
		Username       string  `json:"username"`
		DisplayName    *string `json:"display_name"`
		AvatarID       *string `json:"avatar_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.UserID == "" || req.InstanceDomain == "" || req.Username == "" {
		http.Error(w, "user_id, instance_domain, and username are required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Resolve instance_domain → instance_id.
	var instanceID string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT id FROM instances WHERE domain = $1`, req.InstanceDomain,
	).Scan(&instanceID); err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, "Unknown instance domain", http.StatusBadRequest)
			return
		}
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	ss.ensureRemoteUserStub(ctx, instanceID, federatedUserInfo{
		ID:          req.UserID,
		Username:    req.Username,
		DisplayName: req.DisplayName,
		AvatarID:    req.AvatarID,
	})

	w.WriteHeader(http.StatusNoContent)
}

// --- Aggregated Guild Discovery ---

// aggregatedDiscoverGuild is a single guild in the aggregated discover response.
// Local guilds have empty InstanceID/InstanceDomain/InstanceShorthand.
type aggregatedDiscoverGuild struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Description       *string  `json:"description,omitempty"`
	IconID            *string  `json:"icon_id,omitempty"`
	BannerID          *string  `json:"banner_id,omitempty"`
	Tags              []string `json:"tags,omitempty"`
	MemberCount       int      `json:"member_count"`
	CreatedAt         string   `json:"created_at"`
	InstanceID        string   `json:"instance_id,omitempty"`
	InstanceDomain    string   `json:"instance_domain,omitempty"`
	InstanceShorthand string   `json:"instance_shorthand,omitempty"`
}

// HandleAggregatedDiscover fans out guild discovery to all active federation
// peers in parallel, merges results with local discoverable guilds, deduplicates,
// sorts by member_count DESC, and returns a unified list.
// GET /api/v1/federation/discover?q=&tag=&limit=
func (ss *SyncService) HandleAggregatedDiscover(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	query := r.URL.Query().Get("q")
	tag := r.URL.Query().Get("tag")
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	ctx := r.Context()

	// 1. Query local discoverable guilds (instance_id IS NULL = local only).
	localGuilds := ss.queryLocalDiscoverableGuilds(ctx, query, tag, limit)

	// 2. Get all active federation peers.
	type peerInfo struct {
		id        string
		domain    string
		shorthand *string
	}

	peerRows, err := ss.fed.pool.Query(ctx,
		`SELECT fp.peer_id, i.domain, i.shorthand
		 FROM federation_peers fp
		 JOIN instances i ON i.id = fp.peer_id
		 WHERE fp.instance_id = $1 AND fp.status = 'active'`,
		ss.fed.instanceID)
	if err != nil {
		ss.logger.Error("aggregated discover: failed to query peers", slog.String("error", err.Error()))
		// Still return local results even if peer query fails.
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"data": localGuilds})
		return
	}
	defer peerRows.Close()

	var peers []peerInfo
	for peerRows.Next() {
		var p peerInfo
		if err := peerRows.Scan(&p.id, &p.domain, &p.shorthand); err != nil {
			ss.logger.Warn("aggregated discover: failed to scan peer row", slog.String("error", err.Error()))
			continue
		}
		peers = append(peers, p)
	}

	// 3. Fan out signed discovery requests to all peers in parallel.
	type peerResult struct {
		guilds []aggregatedDiscoverGuild
	}
	results := make(chan peerResult, len(peers))

	var wg sync.WaitGroup
	for _, peer := range peers {
		wg.Add(1)
		go func(p peerInfo) {
			defer wg.Done()

			peerCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			// Build remote discover URL with query params.
			remoteQuery := neturl.Values{}
			if query != "" {
				remoteQuery.Set("q", query)
			}
			if tag != "" {
				remoteQuery.Set("tag", tag)
			}
			remoteQuery.Set("limit", strconv.Itoa(limit))

			remoteURL := fmt.Sprintf("https://%s/federation/v1/guilds/discover?%s", p.domain, remoteQuery.Encode())

			// Sign and POST using existing signAndPost method.
			respBody, statusCode, err := ss.signAndPost(peerCtx, remoteURL, map[string]string{"action": "discover"})
			if err != nil {
				ss.logger.Warn("aggregated discover: peer request failed",
					slog.String("peer", p.domain), slog.String("error", err.Error()))
				return
			}
			if statusCode != http.StatusOK {
				ss.logger.Warn("aggregated discover: peer returned non-200",
					slog.String("peer", p.domain), slog.Int("status", statusCode))
				return
			}

			// Parse remote guilds — try bare array first, then data envelope.
			var rawGuilds []json.RawMessage
			if err := json.Unmarshal(respBody, &rawGuilds); err != nil {
				var envelope struct {
					Data []json.RawMessage `json:"data"`
				}
				if err2 := json.Unmarshal(respBody, &envelope); err2 != nil {
					ss.logger.Warn("aggregated discover: failed to parse peer response",
						slog.String("peer", p.domain), slog.String("error", err2.Error()))
					return
				}
				rawGuilds = envelope.Data
			}

			// Enrich each guild with instance info.
			shorthand := ""
			if p.shorthand != nil {
				shorthand = *p.shorthand
			}

			var enriched []aggregatedDiscoverGuild
			for _, raw := range rawGuilds {
				var g aggregatedDiscoverGuild
				if err := json.Unmarshal(raw, &g); err != nil {
					continue
				}
				g.InstanceID = p.id
				g.InstanceDomain = p.domain
				g.InstanceShorthand = shorthand
				enriched = append(enriched, g)
			}

			results <- peerResult{guilds: enriched}
		}(peer)
	}

	// Close channel when all goroutines complete.
	go func() {
		wg.Wait()
		close(results)
	}()

	// 4. Collect all results: local guilds first, then remote.
	allGuilds := make([]aggregatedDiscoverGuild, 0, len(localGuilds)+len(peers)*limit)
	allGuilds = append(allGuilds, localGuilds...)
	for pr := range results {
		allGuilds = append(allGuilds, pr.guilds...)
	}

	// 5. Deduplicate by (instance_id + guild_id). Empty instance_id = local.
	seen := make(map[string]bool, len(allGuilds))
	deduped := make([]aggregatedDiscoverGuild, 0, len(allGuilds))
	for _, g := range allGuilds {
		key := g.InstanceID + ":" + g.ID
		if seen[key] {
			continue
		}
		seen[key] = true
		deduped = append(deduped, g)
	}

	// 6. Sort by member_count DESC.
	sort.Slice(deduped, func(i, j int) bool {
		return deduped[i].MemberCount > deduped[j].MemberCount
	})

	// 7. Apply limit.
	if len(deduped) > limit {
		deduped = deduped[:limit]
	}

	// 8. Return.
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": deduped})
}

// queryLocalDiscoverableGuilds queries the local database for discoverable
// guilds that are owned by this instance (instance_id IS NULL).
func (ss *SyncService) queryLocalDiscoverableGuilds(ctx context.Context, query, tag string, limit int) []aggregatedDiscoverGuild {
	baseSQL := `SELECT g.id, g.name, g.description, g.icon_id, g.banner_id,
	            g.tags, g.member_count, g.created_at
	     FROM guilds g
	     WHERE g.discoverable = true AND g.instance_id IS NULL`

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
	baseSQL += fmt.Sprintf(` ORDER BY g.member_count DESC LIMIT $%d`, argN)
	args = append(args, limit)

	rows, err := ss.fed.pool.Query(ctx, baseSQL, args...)
	if err != nil {
		ss.logger.Error("aggregated discover: failed to query local guilds", slog.String("error", err.Error()))
		return nil
	}
	defer rows.Close()

	guilds := make([]aggregatedDiscoverGuild, 0)
	for rows.Next() {
		var g aggregatedDiscoverGuild
		var tags []string
		var createdAt time.Time
		if err := rows.Scan(&g.ID, &g.Name, &g.Description, &g.IconID, &g.BannerID,
			&tags, &g.MemberCount, &createdAt); err != nil {
			ss.logger.Error("aggregated discover: failed to scan local guild", slog.String("error", err.Error()))
			continue
		}
		g.Tags = tags
		g.CreatedAt = createdAt.Format(time.RFC3339)
		// InstanceID/InstanceDomain/InstanceShorthand left empty for local guilds.
		guilds = append(guilds, g)
	}

	return guilds
}
