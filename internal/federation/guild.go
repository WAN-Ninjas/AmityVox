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
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/models"
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
	ChannelsJSON json.RawMessage `json:"channels"`
	RolesJSON    json.RawMessage `json:"roles"`
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

	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	query := `SELECT m.id, m.channel_id, m.author_id, m.content, m.created_at,
	                 u.username, u.display_name, u.avatar_id
	          FROM messages m
	          JOIN users u ON u.id = m.author_id
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
		var displayName, avatarID *string
		if err := rows.Scan(&id, &chID, &authorID, &content, &createdAt,
			&username, &displayName, &avatarID); err != nil {
			ss.logger.Warn("failed to scan federated message row", slog.String("error", err.Error()))
			continue
		}
		messages = append(messages, map[string]interface{}{
			"id": id, "channel_id": chID, "author_id": authorID,
			"content": content, "created_at": createdAt,
			"author": map[string]interface{}{
				"id": authorID, "username": username,
				"display_name": displayName, "avatar_id": avatarID,
			},
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
		if err := ss.fed.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM messages WHERE id = ANY($1) AND channel_id = $2`,
			req.ReplyToIDs, channelID,
		).Scan(&validCount); err == nil && validCount == len(req.ReplyToIDs) {
			replyToIDs = req.ReplyToIDs
		}
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

	msg := map[string]interface{}{
		"id": msgID, "channel_id": channelID, "guild_id": guildID,
		"author_id": req.UserID, "content": req.Content, "created_at": now,
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
	ss.bus.PublishChannelEvent(ctx, events.SubjectMessageReactionAdd, "REACTION_ADD", channelID, evt)

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
	ss.bus.PublishChannelEvent(ctx, events.SubjectMessageReactionDel, "REACTION_REMOVE", channelID, evt)

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
		`SELECT i.domain FROM federation_guild_cache fgc
		 JOIN instances i ON i.id = fgc.instance_id
		 WHERE fgc.guild_id = $1 AND fgc.user_id = $2`,
		guildID, userID,
	).Scan(&instanceDomain); err != nil {
		http.Error(w, "Guild not found in federation cache", http.StatusNotFound)
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
		`SELECT id, name, icon_id, description, member_count FROM guilds WHERE id = $1`, guildID,
	).Scan(&resp.GuildID, &resp.Name, &resp.IconID, &resp.Description, &resp.MemberCount)
	if err != nil {
		return nil, fmt.Errorf("looking up guild: %w", err)
	}

	// Load non-private channels only (private channels require explicit access).
	channelRows, err := ss.fed.pool.Query(ctx,
		`SELECT id, channel_type, name, topic, position FROM channels
		 WHERE guild_id = $1 AND (channel_type <> 'private' OR channel_type IS NULL)
		 ORDER BY position`, guildID)
	if err == nil {
		defer channelRows.Close()
		channels := make([]map[string]interface{}, 0)
		for channelRows.Next() {
			var id string
			var channelType *string
			var name, topic *string
			var position int
			if channelRows.Scan(&id, &channelType, &name, &topic, &position) == nil {
				channels = append(channels, map[string]interface{}{
					"id": id, "channel_type": channelType, "name": name, "topic": topic, "position": position,
				})
			}
		}
		resp.ChannelsJSON, _ = json.Marshal(channels)
	}
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

// updateGuildCacheFromEvent updates the local federation_guild_cache when
// receiving inbound guild-level events from remote instances (GUILD_UPDATE,
// CHANNEL_CREATE, CHANNEL_DELETE, GUILD_DELETE).
func (ss *SyncService) updateGuildCacheFromEvent(ctx context.Context, senderID, eventType, guildID string, data json.RawMessage) {
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
		// Build a single UPDATE with only the changed fields.
		setClauses := []string{"cached_at = now()"}
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
		if len(args) > 0 {
			query := fmt.Sprintf("UPDATE federation_guild_cache SET %s WHERE guild_id = $%d",
				strings.Join(setClauses, ", "), argN)
			args = append(args, guildID)
			ss.fed.pool.Exec(ctx, query, args...)
		}

	case "CHANNEL_CREATE", "CHANNEL_UPDATE", "CHANNEL_DELETE":
		// Re-fetch channels for this guild and update the cache.
		// The guild is hosted on the remote instance — rebuild channels_json from local knowledge
		// when events arrive. For now, mark cache as stale by updating cached_at.
		ss.fed.pool.Exec(ctx,
			`UPDATE federation_guild_cache SET cached_at = now() WHERE guild_id = $1`, guildID)

	case "GUILD_DELETE":
		ss.fed.pool.Exec(ctx,
			`DELETE FROM federation_guild_cache WHERE guild_id = $1`, guildID)
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

	// Cache guild metadata.
	var joinResp guildJoinResponse
	if err := json.Unmarshal(respBody, &joinResp); err == nil && joinResp.GuildID != "" {
		var remoteInstanceID string
		if err := ss.fed.pool.QueryRow(ctx,
			`SELECT id FROM instances WHERE domain = $1`, req.InstanceDomain,
		).Scan(&remoteInstanceID); err != nil {
			if disc, err := DiscoverInstance(ctx, req.InstanceDomain); err == nil {
				ss.fed.RegisterRemoteInstance(ctx, disc)
				remoteInstanceID = disc.InstanceID
			}
		}

		if remoteInstanceID != "" {
			if _, err := ss.fed.pool.Exec(ctx,
				`INSERT INTO federation_guild_cache
				 (guild_id, user_id, instance_id, name, icon_id, description, member_count, channels_json, roles_json, cached_at)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
				 ON CONFLICT (guild_id, user_id) DO UPDATE SET
				   name = EXCLUDED.name, icon_id = EXCLUDED.icon_id,
				   description = EXCLUDED.description, member_count = EXCLUDED.member_count,
				   channels_json = EXCLUDED.channels_json, roles_json = EXCLUDED.roles_json, cached_at = now()`,
				joinResp.GuildID, userID, remoteInstanceID,
				joinResp.Name, joinResp.IconID, joinResp.Description, joinResp.MemberCount,
				joinResp.ChannelsJSON, joinResp.RolesJSON,
			); err != nil {
				ss.logger.Warn("failed to cache guild metadata", slog.String("error", err.Error()))
			}
		}
	}

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

	var instanceDomain string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT i.domain FROM federation_guild_cache fgc
		 JOIN instances i ON i.id = fgc.instance_id
		 WHERE fgc.guild_id = $1 AND fgc.user_id = $2`,
		guildID, userID,
	).Scan(&instanceDomain); err != nil {
		http.Error(w, "Guild not found in federation cache", http.StatusNotFound)
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

	ss.fed.pool.Exec(ctx,
		`DELETE FROM federation_guild_cache WHERE guild_id = $1 AND user_id = $2`, guildID, userID)

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
		`SELECT i.domain FROM federation_guild_cache fgc
		 JOIN instances i ON i.id = fgc.instance_id
		 WHERE fgc.guild_id = $1 AND fgc.user_id = $2`,
		guildID, userID,
	).Scan(&instanceDomain); err != nil {
		http.Error(w, "Guild not found in federation cache", http.StatusNotFound)
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
		`SELECT i.domain FROM federation_guild_cache fgc
		 JOIN instances i ON i.id = fgc.instance_id
		 WHERE fgc.guild_id = $1 AND fgc.user_id = $2`,
		guildID, userID,
	).Scan(&instanceDomain); err != nil {
		http.Error(w, "Guild not found in federation cache", http.StatusNotFound)
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
		`SELECT i.domain FROM federation_guild_cache fgc
		 JOIN instances i ON i.id = fgc.instance_id
		 WHERE fgc.guild_id = $1 AND fgc.user_id = $2`,
		guildID, userID,
	).Scan(&instanceDomain); err != nil {
		http.Error(w, "Guild not found in federation cache", http.StatusNotFound)
		return
	}

	remoteURL := fmt.Sprintf("https://%s/federation/v1/guilds/%s/members", instanceDomain, guildID)
	respBody, statusCode, err := ss.signAndPost(ctx, remoteURL, federatedGuildMembersRequest{UserID: userID})
	if err != nil {
		http.Error(w, "Failed to contact remote instance", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	w.Write(respBody)
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
		`SELECT i.domain FROM federation_guild_cache fgc
		 JOIN instances i ON i.id = fgc.instance_id
		 WHERE fgc.guild_id = $1 AND fgc.user_id = $2`,
		guildID, userID,
	).Scan(&instanceDomain); err != nil {
		http.Error(w, "Guild not found in federation cache", http.StatusNotFound)
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
		`SELECT i.domain FROM federation_guild_cache fgc
		 JOIN instances i ON i.id = fgc.instance_id
		 WHERE fgc.guild_id = $1 AND fgc.user_id = $2`,
		guildID, userID,
	).Scan(&instanceDomain); err != nil {
		http.Error(w, "Guild not found in federation cache", http.StatusNotFound)
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
