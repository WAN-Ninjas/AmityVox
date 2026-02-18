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
	UserID  string `json:"user_id"`
	Content string `json:"content"`
	Nonce   string `json:"nonce,omitempty"`
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

	// Check user is not banned.
	var banned bool
	ss.fed.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM guild_bans WHERE guild_id = $1 AND user_id = $2)`,
		guildID, req.UserID,
	).Scan(&banned)
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

		ss.bus.PublishJSON(ctx, events.SubjectGuildMemberAdd, "GUILD_MEMBER_ADD", map[string]interface{}{
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
	var remainingCount int
	ss.fed.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM guild_members gm
		 JOIN users u ON u.id = gm.user_id
		 WHERE gm.guild_id = $1 AND u.instance_id = $2`,
		guildID, senderID,
	).Scan(&remainingCount)

	if remainingCount == 0 {
		ss.fed.pool.Exec(ctx,
			`DELETE FROM federation_channel_peers
			 WHERE instance_id = $1 AND channel_id IN (SELECT id FROM channels WHERE guild_id = $2)`,
			senderID, guildID)
	}

	ss.bus.PublishJSON(ctx, events.SubjectGuildMemberRemove, "GUILD_MEMBER_REMOVE", map[string]interface{}{
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

	// Check ban.
	var banned bool
	ss.fed.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM guild_bans WHERE guild_id = $1 AND user_id = $2)`,
		guildID, req.UserID,
	).Scan(&banned)
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

		ss.bus.PublishJSON(ctx, events.SubjectGuildMemberAdd, "GUILD_MEMBER_ADD", map[string]interface{}{
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
	signed, _, ok := ss.verifyFederationRequest(w, r)
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

	// Verify channel belongs to guild.
	var channelGuildID *string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT guild_id FROM channels WHERE id = $1`, channelID,
	).Scan(&channelGuildID); err != nil || channelGuildID == nil || *channelGuildID != guildID {
		http.Error(w, "Channel not found in guild", http.StatusNotFound)
		return
	}

	// Verify user is a member.
	var isMember bool
	ss.fed.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM guild_members WHERE guild_id = $1 AND user_id = $2)`,
		guildID, req.UserID,
	).Scan(&isMember)
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
	signed, _, ok := ss.verifyFederationRequest(w, r)
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

	// Verify channel belongs to guild and is not locked.
	var channelGuildID *string
	var locked bool
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT guild_id, locked FROM channels WHERE id = $1`, channelID,
	).Scan(&channelGuildID, &locked); err != nil || channelGuildID == nil || *channelGuildID != guildID {
		http.Error(w, "Channel not found in guild", http.StatusNotFound)
		return
	}
	if locked {
		http.Error(w, "Channel is locked", http.StatusForbidden)
		return
	}

	// Verify membership.
	var isMember bool
	ss.fed.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM guild_members WHERE guild_id = $1 AND user_id = $2)`,
		guildID, req.UserID,
	).Scan(&isMember)
	if !isMember {
		http.Error(w, "Not a guild member", http.StatusForbidden)
		return
	}

	msgID := models.NewULID().String()
	now := time.Now()

	_, err := ss.fed.pool.Exec(ctx,
		`INSERT INTO messages (id, channel_id, author_id, content, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		msgID, channelID, req.UserID, req.Content, now)
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
	ss.bus.PublishJSON(ctx, events.SubjectMessageCreate, "MESSAGE_CREATE", msg)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"data": msg})
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
			var id, channelType string
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
		if update.Name != nil {
			ss.fed.pool.Exec(ctx,
				`UPDATE federation_guild_cache SET name = $1, cached_at = now() WHERE guild_id = $2`,
				*update.Name, guildID)
		}
		if update.Description != nil {
			ss.fed.pool.Exec(ctx,
				`UPDATE federation_guild_cache SET description = $1, cached_at = now() WHERE guild_id = $2`,
				*update.Description, guildID)
		}
		if update.IconID != nil {
			ss.fed.pool.Exec(ctx,
				`UPDATE federation_guild_cache SET icon_id = $1, cached_at = now() WHERE guild_id = $2`,
				*update.IconID, guildID)
		}
		if update.MemberCount != nil {
			ss.fed.pool.Exec(ctx,
				`UPDATE federation_guild_cache SET member_count = $1, cached_at = now() WHERE guild_id = $2`,
				*update.MemberCount, guildID)
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
		Content string `json:"content"`
		Nonce   string `json:"nonce"`
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

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return respBody, resp.StatusCode, nil
}
