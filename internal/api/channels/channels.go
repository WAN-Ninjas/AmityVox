// Package channels implements REST API handlers for channel operations including
// fetching, updating, and deleting channels, managing messages, reactions, pins,
// typing indicators, read state acknowledgment, and permission overrides.
// Mounted under /api/v1/channels.
package channels

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/amityvox/amityvox/internal/api/apiutil"
	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/mentions"
	"github.com/amityvox/amityvox/internal/models"
	"github.com/amityvox/amityvox/internal/permissions"
)

// Handler implements channel-related REST API endpoints.
type Handler struct {
	Pool     *pgxpool.Pool
	EventBus *events.Bus
	Logger   *slog.Logger
	FedProxy apiutil.FederationProxy // optional, nil if federation disabled
}

// --- DM Spam Detection ---

// dmSpamTracker tracks recent DM sends per user to detect spam patterns.
// A user is flagged when they send the same content to 5+ different DM
// recipients within a 10-minute sliding window.
var dmSpamTracker = &dmTracker{
	sends: make(map[string][]dmSendEntry),
}

const (
	dmSpamRecipientThreshold = 5           // same content to this many different recipients = flagged
	dmSpamWindow             = 10 * time.Minute
)

type dmTracker struct {
	mu    sync.Mutex
	sends map[string][]dmSendEntry // key: "userID:contentHash"
}

type dmSendEntry struct {
	recipientID string
	ts          time.Time
}

// contentHash returns a hex-encoded SHA-256 hash of the content for compact storage.
func contentHash(content string) string {
	h := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(content))))
	return hex.EncodeToString(h[:])
}

// trackDMSend records a DM send and returns true if the user is flagged as a spammer.
func (dt *dmTracker) trackDMSend(userID, recipientID, content string) bool {
	hash := contentHash(content)
	key := userID + ":" + hash
	now := time.Now()
	cutoff := now.Add(-dmSpamWindow)

	dt.mu.Lock()
	defer dt.mu.Unlock()

	// Prune old entries.
	entries := dt.sends[key]
	pruned := entries[:0]
	for _, e := range entries {
		if e.ts.After(cutoff) {
			pruned = append(pruned, e)
		}
	}

	// Add the current send.
	pruned = append(pruned, dmSendEntry{recipientID: recipientID, ts: now})
	dt.sends[key] = pruned

	// Count unique recipients.
	uniqueRecipients := make(map[string]struct{})
	for _, e := range pruned {
		uniqueRecipients[e.recipientID] = struct{}{}
	}

	return len(uniqueRecipients) >= dmSpamRecipientThreshold
}

// cleanup removes stale entries older than the window.
func (dt *dmTracker) cleanup() {
	dt.mu.Lock()
	defer dt.mu.Unlock()
	cutoff := time.Now().Add(-dmSpamWindow)
	for key, entries := range dt.sends {
		pruned := entries[:0]
		for _, e := range entries {
			if e.ts.After(cutoff) {
				pruned = append(pruned, e)
			}
		}
		if len(pruned) == 0 {
			delete(dt.sends, key)
		} else {
			dt.sends[key] = pruned
		}
	}
}

func init() {
	// Periodically clean up the DM spam tracker in the background.
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		for range ticker.C {
			dmSpamTracker.cleanup()
		}
	}()
}

type updateChannelRequest struct {
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

type createMessageRequest struct {
	Content             *string  `json:"content"`
	Nonce               *string  `json:"nonce"`
	AttachmentIDs       []string `json:"attachment_ids"`
	ReplyToIDs          []string `json:"reply_to_ids"`
	MentionUserIDs      []string `json:"mention_user_ids"`
	MentionRoleIDs      []string `json:"mention_role_ids"`
	MentionHere     bool     `json:"mention_here"`
	Silent              bool     `json:"silent"`
	Encrypted           bool     `json:"encrypted"`
	EncryptionSessionID *string  `json:"encryption_session_id"`
}

type scheduleMessageRequest struct {
	Content       *string  `json:"content"`
	AttachmentIDs []string `json:"attachment_ids"`
	ScheduledFor  string   `json:"scheduled_for"`
}

type updateMessageRequest struct {
	Content *string `json:"content"`
}

type permissionOverrideRequest struct {
	TargetType       string `json:"target_type"`
	PermissionsAllow int64  `json:"permissions_allow"`
	PermissionsDeny  int64  `json:"permissions_deny"`
}

// HandleGetChannel returns a channel's details.
// GET /api/v1/channels/{channelID}
func (h *Handler) HandleGetChannel(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	// Permission check: ViewChannel.
	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ViewChannel) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need VIEW_CHANNEL permission")
		return
	}

	channel, err := h.getChannel(r.Context(), channelID)
	if err != nil {
		if err == pgx.ErrNoRows {
			apiutil.WriteError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
			return
		}
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get channel")
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, channel)
}

// HandleUpdateChannel updates a channel's settings.
// PATCH /api/v1/channels/{channelID}
func (h *Handler) HandleUpdateChannel(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	// Threads can be updated with ManageThreads or ManageChannels.
	var parentChID *string
	if err := h.Pool.QueryRow(r.Context(), `SELECT parent_channel_id FROM channels WHERE id = $1`, channelID).Scan(&parentChID); err != nil && err != pgx.ErrNoRows {
		apiutil.InternalError(w, h.Logger, "Failed to query channel", err)
		return
	}
	if parentChID != nil {
		if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ManageThreads) &&
			!h.hasChannelPermission(r.Context(), channelID, userID, permissions.ManageChannels) {
			apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_THREADS permission")
			return
		}
	} else {
		if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ManageChannels) {
			apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_CHANNELS permission")
			return
		}
	}

	var req updateChannelRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	// Forward to home instance if channel belongs to a federated guild.
	if h.FedProxy != nil {
		var guildID *string
		var instanceID *string
		if err := h.Pool.QueryRow(r.Context(),
			`SELECT c.guild_id, g.instance_id FROM channels c
			 LEFT JOIN guilds g ON g.id = c.guild_id
			 WHERE c.id = $1`, channelID,
		).Scan(&guildID, &instanceID); err != nil && err != pgx.ErrNoRows {
			apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to look up channel")
			return
		}
		if guildID != nil && h.FedProxy.ProxyToHomeInstance(w, r, *guildID, instanceID, "channel_update", userID, req) {
			return
		}
	}

	// Validate encryption toggle: only text, DM, and group channels support encryption.
	if req.Encrypted != nil {
		var channelType string
		if err := h.Pool.QueryRow(r.Context(), `SELECT channel_type FROM channels WHERE id = $1`, channelID).Scan(&channelType); err != nil {
			if err == pgx.ErrNoRows {
				apiutil.WriteError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
				return
			}
			apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get channel type")
			return
		}
		if channelType != "text" && channelType != "dm" && channelType != "group" {
			apiutil.WriteError(w, http.StatusBadRequest, "encryption_not_supported",
				"Encryption is only supported for text, DM, and group channels")
			return
		}
	}

	// Validate auto-archive duration if provided.
	if req.DefaultAutoArchiveDuration != nil {
		valid := map[int]bool{0: true, 60: true, 1440: true, 4320: true, 10080: true}
		if !valid[*req.DefaultAutoArchiveDuration] {
			apiutil.WriteError(w, http.StatusBadRequest, "invalid_auto_archive",
				"Auto-archive duration must be 0 (never), 60 (1h), 1440 (1d), 4320 (3d), or 10080 (7d)")
			return
		}
	}

	var channel models.Channel
	err := h.Pool.QueryRow(r.Context(),
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
		           pinned, reply_count, created_at`,
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
		&channel.Pinned, &channel.ReplyCount, &channel.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			apiutil.WriteError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
			return
		}
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update channel")
		return
	}

	guildID := ""
	if channel.GuildID != nil {
		guildID = *channel.GuildID
	}
	h.EventBus.Publish(r.Context(), events.SubjectChannelUpdate, events.Event{
		Type:      "CHANNEL_UPDATE",
		GuildID:   guildID,
		ChannelID: channelID,
		Data:      mustMarshal(channel),
	})

	apiutil.WriteJSON(w, http.StatusOK, channel)
}

// HandleDeleteChannel deletes a channel.
// DELETE /api/v1/channels/{channelID}
func (h *Handler) HandleDeleteChannel(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	// Forward to home instance if channel belongs to a federated guild.
	if h.FedProxy != nil {
		var fedGuildID *string
		var instanceID *string
		if err := h.Pool.QueryRow(r.Context(),
			`SELECT c.guild_id, g.instance_id FROM channels c
			 LEFT JOIN guilds g ON g.id = c.guild_id
			 WHERE c.id = $1`, channelID,
		).Scan(&fedGuildID, &instanceID); err != nil && err != pgx.ErrNoRows {
			apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to look up channel")
			return
		}
		if fedGuildID != nil && h.FedProxy.ProxyToHomeInstance(w, r, *fedGuildID, instanceID, "channel_delete", userID, nil) {
			return
		}
	}

	// Threads (channels with parent_channel_id) can be deleted with ManageThreads or ManageChannels.
	// Regular channels require ManageChannels.
	var parentChannelID *string
	if err := h.Pool.QueryRow(r.Context(), `SELECT parent_channel_id FROM channels WHERE id = $1`, channelID).Scan(&parentChannelID); err != nil && err != pgx.ErrNoRows {
		apiutil.InternalError(w, h.Logger, "Failed to query channel", err)
		return
	}
	if parentChannelID != nil {
		if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ManageThreads) &&
			!h.hasChannelPermission(r.Context(), channelID, userID, permissions.ManageChannels) {
			apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_THREADS permission")
			return
		}
	} else {
		if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ManageChannels) {
			apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_CHANNELS permission")
			return
		}
	}

	// Fetch guild_id BEFORE deleting so we can route the event to guild members.
	var guildID *string
	if err := h.Pool.QueryRow(r.Context(), `SELECT guild_id FROM channels WHERE id = $1`, channelID).Scan(&guildID); err != nil && err != pgx.ErrNoRows {
		h.Logger.Warn("failed to fetch guild_id before channel delete", slog.String("channel_id", channelID), slog.String("error", err.Error()))
	}

	// Clear thread_id references in parent channel messages before deleting,
	// and delete within a transaction to ensure consistency.
	var rowsAffected int64
	if err := apiutil.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		// Nullify thread_id on messages that reference this channel as a thread
		if _, err := tx.Exec(r.Context(), `UPDATE messages SET thread_id = NULL WHERE thread_id = $1`, channelID); err != nil {
			return fmt.Errorf("clearing thread references: %w", err)
		}
		// Also nullify thread_id references in user_reports
		if _, err := tx.Exec(r.Context(), `UPDATE user_reports SET context_channel_id = NULL WHERE context_channel_id = $1`, channelID); err != nil {
			return fmt.Errorf("clearing user_reports references: %w", err)
		}
		tag, err := tx.Exec(r.Context(), `DELETE FROM channels WHERE id = $1`, channelID)
		if err != nil {
			return fmt.Errorf("deleting channel: %w", err)
		}
		rowsAffected = tag.RowsAffected()
		return nil
	}); err != nil {
		h.Logger.Error("failed to delete channel", slog.String("channel_id", channelID), slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete channel")
		return
	}
	if rowsAffected == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
		return
	}

	deleteGuildID := ""
	if guildID != nil {
		deleteGuildID = *guildID
	}
	h.EventBus.Publish(r.Context(), events.SubjectChannelDelete, events.Event{
		Type:      "CHANNEL_DELETE",
		GuildID:   deleteGuildID,
		ChannelID: channelID,
		Data:      mustMarshal(map[string]string{"id": channelID, "guild_id": deleteGuildID}),
	})

	w.WriteHeader(http.StatusNoContent)
}

// HandleGetMessages returns paginated messages from a channel.
// GET /api/v1/channels/{channelID}/messages?before=&after=&around=&limit=
func (h *Handler) HandleGetMessages(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	// Permission check: ViewChannel + ReadHistory.
	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ReadHistory) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need READ_HISTORY permission")
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	before := r.URL.Query().Get("before")
	after := r.URL.Query().Get("after")
	around := r.URL.Query().Get("around")

	var query string
	var args []interface{}

	switch {
	case before != "":
		query = `SELECT id, channel_id, author_id, content, nonce, message_type, edited_at, flags,
		                reply_to_ids, mention_user_ids, mention_role_ids, mention_here,
		                thread_id, masquerade_name, masquerade_avatar, masquerade_color,
		                encrypted, encryption_session_id, created_at
		         FROM messages WHERE channel_id = $1 AND id < $2
		         ORDER BY id DESC LIMIT $3`
		args = []interface{}{channelID, before, limit}
	case after != "":
		query = `SELECT id, channel_id, author_id, content, nonce, message_type, edited_at, flags,
		                reply_to_ids, mention_user_ids, mention_role_ids, mention_here,
		                thread_id, masquerade_name, masquerade_avatar, masquerade_color,
		                encrypted, encryption_session_id, created_at
		         FROM messages WHERE channel_id = $1 AND id > $2
		         ORDER BY id ASC LIMIT $3`
		args = []interface{}{channelID, after, limit}
	case around != "":
		halfLimit := limit / 2
		query = `(SELECT id, channel_id, author_id, content, nonce, message_type, edited_at, flags,
		                 reply_to_ids, mention_user_ids, mention_role_ids, mention_here,
		                 thread_id, masquerade_name, masquerade_avatar, masquerade_color,
		                 encrypted, encryption_session_id, created_at
		          FROM messages WHERE channel_id = $1 AND id <= $2
		          ORDER BY id DESC LIMIT $3)
		         UNION ALL
		         (SELECT id, channel_id, author_id, content, nonce, message_type, edited_at, flags,
		                 reply_to_ids, mention_user_ids, mention_role_ids, mention_here,
		                 thread_id, masquerade_name, masquerade_avatar, masquerade_color,
		                 encrypted, encryption_session_id, created_at
		          FROM messages WHERE channel_id = $1 AND id > $2
		          ORDER BY id ASC LIMIT $4)
		         ORDER BY id DESC`
		args = []interface{}{channelID, around, halfLimit, halfLimit}
	default:
		query = `SELECT id, channel_id, author_id, content, nonce, message_type, edited_at, flags,
		                reply_to_ids, mention_user_ids, mention_role_ids, mention_here,
		                thread_id, masquerade_name, masquerade_avatar, masquerade_color,
		                encrypted, encryption_session_id, created_at
		         FROM messages WHERE channel_id = $1
		         ORDER BY id DESC LIMIT $2`
		args = []interface{}{channelID, limit}
	}

	rows, err := h.Pool.Query(r.Context(), query, args...)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get messages", err)
		return
	}
	defer rows.Close()

	messages := make([]models.Message, 0)
	for rows.Next() {
		var m models.Message
		if err := rows.Scan(
			&m.ID, &m.ChannelID, &m.AuthorID, &m.Content, &m.Nonce, &m.MessageType,
			&m.EditedAt, &m.Flags, &m.ReplyToIDs, &m.MentionUserIDs, &m.MentionRoleIDs,
			&m.MentionHere, &m.ThreadID, &m.MasqueradeName, &m.MasqueradeAvatar,
			&m.MasqueradeColor, &m.Encrypted, &m.EncryptionSessionID, &m.CreatedAt,
		); err != nil {
			apiutil.InternalError(w, h.Logger, "Failed to read messages", err)
			return
		}
		messages = append(messages, m)
	}

	h.enrichMessagesWithAuthors(r.Context(), messages)
	h.enrichMessagesWithAttachments(r.Context(), messages)
	h.enrichMessagesWithEmbeds(r.Context(), messages)

	apiutil.WriteJSON(w, http.StatusOK, messages)
}

// HandleCreateMessage sends a new message in a channel.
// POST /api/v1/channels/{channelID}/messages
func (h *Handler) HandleCreateMessage(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	// Load all channel state + guild ownership + user permissions in 2 queries
	// (down from ~25 sequential queries in the original implementation).
	cc, err := h.loadChannelCtx(r.Context(), channelID, userID)
	if err != nil {
		apiutil.WriteError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
		return
	}

	// Permission check: SendMessages.
	if !cc.hasPerm(permissions.SendMessages) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need SEND_MESSAGES permission")
		return
	}

	// Check if channel is locked, archived, or read-only.
	if cc.Archived {
		apiutil.WriteError(w, http.StatusForbidden, "channel_archived", "This channel is archived and read-only")
		return
	}
	if cc.Locked {
		apiutil.WriteError(w, http.StatusForbidden, "channel_locked", "This channel is locked")
		return
	}
	// Read-only check: owners, admins, and Administrator role holders bypass.
	if cc.ReadOnly {
		allowed := cc.IsOwner || cc.IsAdmin || cc.hasPerm(permissions.Administrator)
		if !allowed && len(cc.ReadOnlyRoleIDs) > 0 && cc.GuildID != nil {
			var matchCount int
			h.Pool.QueryRow(r.Context(),
				`SELECT COUNT(*) FROM member_roles
				 WHERE guild_id = $1 AND user_id = $2 AND role_id = ANY($3)`,
				*cc.GuildID, userID, cc.ReadOnlyRoleIDs,
			).Scan(&matchCount)
			if matchCount > 0 {
				allowed = true
			}
		}
		if !allowed {
			apiutil.WriteError(w, http.StatusForbidden, "channel_read_only",
				"This channel is read-only. Only users with specific roles can post.")
			return
		}
	}

	var req createMessageRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	hasContent := req.Content != nil && *req.Content != ""
	hasAttachments := len(req.AttachmentIDs) > 0
	if !hasContent && !hasAttachments {
		apiutil.WriteError(w, http.StatusBadRequest, "empty_content", "Message content or attachments required")
		return
	}

	if hasContent && len(*req.Content) > 4000 {
		apiutil.WriteError(w, http.StatusBadRequest, "content_too_long", "Message content must be at most 4000 characters")
		return
	}

	// Validate encryption consistency.
	if cc.Encrypted && !req.Encrypted {
		apiutil.WriteError(w, http.StatusBadRequest, "encryption_required",
			"This channel is encrypted. Messages must be sent with encrypted: true")
		return
	}
	if req.Encrypted && !cc.Encrypted {
		apiutil.WriteError(w, http.StatusBadRequest, "channel_not_encrypted",
			"Cannot send encrypted messages to an unencrypted channel")
		return
	}

	// Enforce slowmode. Users with ManageMessages or ManageChannels bypass.
	if cc.SlowmodeSeconds > 0 && !cc.hasPerm(permissions.ManageMessages) && !cc.hasPerm(permissions.ManageChannels) {
		var lastSent *time.Time
		h.Pool.QueryRow(r.Context(),
			`SELECT MAX(created_at) FROM messages WHERE channel_id = $1 AND author_id = $2`,
			channelID, userID).Scan(&lastSent)
		if lastSent != nil {
			elapsed := time.Since(*lastSent)
			if elapsed < time.Duration(cc.SlowmodeSeconds)*time.Second {
				remaining := time.Duration(cc.SlowmodeSeconds)*time.Second - elapsed
				apiutil.WriteError(w, http.StatusTooManyRequests, "slowmode",
					fmt.Sprintf("Slowmode active. Try again in %.0f seconds", remaining.Seconds()))
				return
			}
		}
	}

	// Check if the user is timed out in this guild.
	if cc.TimeoutUntil != nil && cc.TimeoutUntil.After(time.Now()) {
		apiutil.WriteError(w, http.StatusForbidden, "timed_out", "You are timed out and cannot send messages")
		return
	}

	// DM spam detection.
	if cc.GuildID == nil && hasContent {
		var recipientID string
		err := h.Pool.QueryRow(r.Context(),
			`SELECT user_id FROM channel_recipients WHERE channel_id = $1 AND user_id != $2 LIMIT 1`,
			channelID, userID,
		).Scan(&recipientID)
		if err == nil && recipientID != "" {
			if dmSpamTracker.trackDMSend(userID, recipientID, *req.Content) {
				h.Logger.Warn("DM spam detected: user sending identical content to multiple recipients",
					slog.String("user_id", userID),
					slog.String("channel_id", channelID),
				)
				apiutil.WriteError(w, http.StatusTooManyRequests, "dm_spam_detected",
					"You are sending similar messages to too many users. Please slow down.")
				return
			}
		}
	}

	// Handle silent messages: check for "@silent " prefix or silent field.
	var flags int
	if req.Silent {
		flags |= models.MessageFlagSilent
	}
	if req.Content != nil && strings.HasPrefix(*req.Content, "@silent ") {
		flags |= models.MessageFlagSilent
		trimmed := strings.TrimPrefix(*req.Content, "@silent ")
		req.Content = &trimmed
	}

	// Extract and validate mentions from content.
	var mentionUserIDs []string
	var mentionRoleIDs []string
	var mentionHere bool

	if req.Encrypted {
		// Encrypted messages: trust client-supplied mention fields (server can't parse ciphertext).
		mentionUserIDs = req.MentionUserIDs
		mentionRoleIDs = req.MentionRoleIDs
		mentionHere = req.MentionHere
	} else if hasContent {
		parsed := mentions.Parse(*req.Content)
		mentionHere = parsed.MentionHere
		mentionUserIDs = parsed.UserIDs
		mentionRoleIDs = parsed.RoleIDs
	}

	// Validate @here permission — silently strip if user lacks MentionHere.
	if mentionHere && cc.GuildID != nil && !cc.hasPerm(permissions.MentionHere) {
		mentionHere = false
	}
	// No @here in DMs.
	if mentionHere && cc.GuildID == nil {
		mentionHere = false
	}

	// Validate user mentions: only store IDs of actual guild members (or DM recipients).
	if len(mentionUserIDs) > 0 {
		if cc.GuildID != nil {
			rows, qErr := h.Pool.Query(r.Context(),
				`SELECT user_id FROM guild_members WHERE guild_id = $1 AND user_id = ANY($2)`,
				*cc.GuildID, mentionUserIDs)
			if qErr != nil {
				h.Logger.Warn("mention user validation query failed, clearing mentions", slog.String("error", qErr.Error()))
				mentionUserIDs = nil
			} else {
				valid := map[string]bool{}
				for rows.Next() {
					var uid string
					if rows.Scan(&uid) == nil {
						valid[uid] = true
					}
				}
				rows.Close()
				filtered := mentionUserIDs[:0]
				for _, id := range mentionUserIDs {
					if valid[id] {
						filtered = append(filtered, id)
					}
				}
				mentionUserIDs = filtered
			}
		} else {
			// DMs: validate against channel recipients.
			rows, qErr := h.Pool.Query(r.Context(),
				`SELECT user_id FROM channel_recipients WHERE channel_id = $1 AND user_id = ANY($2)`,
				channelID, mentionUserIDs)
			if qErr != nil {
				h.Logger.Warn("mention user validation query failed, clearing mentions", slog.String("error", qErr.Error()))
				mentionUserIDs = nil
			} else {
				valid := map[string]bool{}
				for rows.Next() {
					var uid string
					if rows.Scan(&uid) == nil {
						valid[uid] = true
					}
				}
				rows.Close()
				filtered := mentionUserIDs[:0]
				for _, id := range mentionUserIDs {
					if valid[id] {
						filtered = append(filtered, id)
					}
				}
				mentionUserIDs = filtered
			}
		}
	}

	// Validate role mentions: must be mentionable (or user has ManageRoles).
	if len(mentionRoleIDs) > 0 && cc.GuildID != nil {
		if !cc.hasPerm(permissions.ManageRoles) {
			rows, qErr := h.Pool.Query(r.Context(),
				`SELECT id FROM roles WHERE guild_id = $1 AND id = ANY($2) AND mentionable = true`,
				*cc.GuildID, mentionRoleIDs)
			if qErr != nil {
				h.Logger.Warn("mention role validation query failed, clearing role mentions", slog.String("error", qErr.Error()))
				mentionRoleIDs = nil
			} else {
				valid := map[string]bool{}
				for rows.Next() {
					var rid string
					if rows.Scan(&rid) == nil {
						valid[rid] = true
					}
				}
				rows.Close()
				filtered := mentionRoleIDs[:0]
				for _, id := range mentionRoleIDs {
					if valid[id] {
						filtered = append(filtered, id)
					}
				}
				mentionRoleIDs = filtered
			}
		}
	} else if cc.GuildID == nil {
		// No role mentions in DMs.
		mentionRoleIDs = nil
	}

	msgID := models.NewULID().String()
	msgType := models.MessageTypeDefault
	if len(req.ReplyToIDs) > 0 {
		msgType = models.MessageTypeReply
	}

	var msg models.Message
	err = h.Pool.QueryRow(r.Context(),
		`INSERT INTO messages (id, channel_id, author_id, content, nonce, message_type, flags,
		                       reply_to_ids, mention_user_ids, mention_role_ids, mention_here,
		                       encrypted, encryption_session_id, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, now())
		 RETURNING id, channel_id, author_id, content, nonce, message_type, edited_at, flags,
		           reply_to_ids, mention_user_ids, mention_role_ids, mention_here,
		           thread_id, masquerade_name, masquerade_avatar, masquerade_color,
		           encrypted, encryption_session_id, created_at`,
		msgID, channelID, userID, req.Content, req.Nonce, msgType, flags,
		req.ReplyToIDs, mentionUserIDs, mentionRoleIDs, mentionHere,
		req.Encrypted, req.EncryptionSessionID,
	).Scan(
		&msg.ID, &msg.ChannelID, &msg.AuthorID, &msg.Content, &msg.Nonce, &msg.MessageType,
		&msg.EditedAt, &msg.Flags, &msg.ReplyToIDs, &msg.MentionUserIDs, &msg.MentionRoleIDs,
		&msg.MentionHere, &msg.ThreadID, &msg.MasqueradeName, &msg.MasqueradeAvatar,
		&msg.MasqueradeColor, &msg.Encrypted, &msg.EncryptionSessionID, &msg.CreatedAt,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to send message", err)
		return
	}

	// Link attachments to the message.
	if len(req.AttachmentIDs) > 0 {
		h.Pool.Exec(r.Context(),
			`UPDATE attachments SET message_id = $1 WHERE id = ANY($2) AND uploader_id = $3 AND message_id IS NULL`,
			msgID, req.AttachmentIDs, userID)
		msg.Attachments = h.loadAttachments(r.Context(), msgID)
	}

	// Update last_message_id on the channel.
	h.Pool.Exec(r.Context(),
		`UPDATE channels SET last_message_id = $1 WHERE id = $2`, msgID, channelID)

	// Update last_activity_at and reply_count for thread channels (fire-and-forget).
	h.Pool.Exec(r.Context(),
		`UPDATE channels SET last_activity_at = now(), reply_count = reply_count + 1
		 WHERE id = $1 AND parent_channel_id IS NOT NULL`,
		channelID)

	// Populate author user data for the response and event.
	h.enrichMessageWithAuthor(r.Context(), &msg)

	h.EventBus.Publish(r.Context(), events.SubjectMessageCreate, events.Event{
		Type:      "MESSAGE_CREATE",
		ChannelID: channelID,
		Data:      mustMarshal(msg),
	})

	apiutil.WriteJSON(w, http.StatusCreated, msg)
}

// HandleGetMessage returns a single message by ID.
// GET /api/v1/channels/{channelID}/messages/{messageID}
func (h *Handler) HandleGetMessage(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")

	// Permission check: ViewChannel.
	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ViewChannel) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need VIEW_CHANNEL permission")
		return
	}

	msg, err := h.getMessage(r.Context(), channelID, messageID)
	if err != nil {
		if err == pgx.ErrNoRows {
			apiutil.WriteError(w, http.StatusNotFound, "message_not_found", "Message not found")
			return
		}
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get message")
		return
	}

	msg.Attachments = h.loadAttachments(r.Context(), messageID)
	msg.Embeds = h.loadEmbeds(r.Context(), messageID)

	apiutil.WriteJSON(w, http.StatusOK, msg)
}

// HandleUpdateMessage edits a message's content. Only the author can edit.
// PATCH /api/v1/channels/{channelID}/messages/{messageID}
func (h *Handler) HandleUpdateMessage(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")

	var req updateMessageRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	if req.Content == nil {
		apiutil.WriteError(w, http.StatusBadRequest, "missing_content", "Content is required")
		return
	}

	// Verify ownership and get current content for edit history.
	var authorID string
	var currentContent *string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT author_id, content FROM messages WHERE id = $1 AND channel_id = $2`,
		messageID, channelID,
	).Scan(&authorID, &currentContent)
	if err != nil {
		apiutil.WriteError(w, http.StatusNotFound, "message_not_found", "Message not found")
		return
	}
	if authorID != userID {
		apiutil.WriteError(w, http.StatusForbidden, "not_author", "You can only edit your own messages")
		return
	}

	// Save previous content to edit history.
	if currentContent != nil {
		editID := models.NewULID().String()
		h.Pool.Exec(r.Context(),
			`INSERT INTO message_edits (id, message_id, content, edited_at) VALUES ($1, $2, $3, now())`,
			editID, messageID, *currentContent)
	}

	// Re-parse mentions from the edited content.
	var editMentionUserIDs []string
	var editMentionRoleIDs []string
	var editMentionHere bool

	// Check if message is encrypted.
	var encrypted bool
	var guildID *string
	h.Pool.QueryRow(r.Context(),
		`SELECT m.encrypted, c.guild_id FROM messages m JOIN channels c ON c.id = m.channel_id
		 WHERE m.id = $1`, messageID).Scan(&encrypted, &guildID)

	if !encrypted && req.Content != nil {
		parsed := mentions.Parse(*req.Content)
		editMentionUserIDs = parsed.UserIDs
		editMentionRoleIDs = parsed.RoleIDs
		editMentionHere = parsed.MentionHere

		// Strip @here if in DMs.
		if guildID == nil {
			editMentionHere = false
			editMentionRoleIDs = nil
		}

		// Validate mentions the same way as the create path.
		if guildID != nil {
			// Load channel context for permission checks.
			cc, ccErr := h.loadChannelCtx(r.Context(), channelID, userID)
			if ccErr != nil {
				h.Logger.Warn("failed to load channel context for edit mention validation", slog.String("error", ccErr.Error()))
				// Fail closed: strip all special mentions.
				editMentionHere = false
				editMentionUserIDs = nil
				editMentionRoleIDs = nil
			} else {
				// Validate @here permission.
				if editMentionHere && !cc.hasPerm(permissions.MentionHere) {
					editMentionHere = false
				}

				// Validate user mentions: only actual guild members.
				if len(editMentionUserIDs) > 0 {
					rows, qErr := h.Pool.Query(r.Context(),
						`SELECT user_id FROM guild_members WHERE guild_id = $1 AND user_id = ANY($2)`,
						*guildID, editMentionUserIDs)
					if qErr != nil {
						h.Logger.Warn("edit mention user validation query failed, clearing mentions", slog.String("error", qErr.Error()))
						editMentionUserIDs = nil
					} else {
						valid := map[string]bool{}
						for rows.Next() {
							var uid string
							if rows.Scan(&uid) == nil {
								valid[uid] = true
							}
						}
						rows.Close()
						filtered := editMentionUserIDs[:0]
						for _, id := range editMentionUserIDs {
							if valid[id] {
								filtered = append(filtered, id)
							}
						}
						editMentionUserIDs = filtered
					}
				}

				// Validate role mentions: must be mentionable (or user has ManageRoles).
				if len(editMentionRoleIDs) > 0 {
					if !cc.hasPerm(permissions.ManageRoles) {
						rows, qErr := h.Pool.Query(r.Context(),
							`SELECT id FROM roles WHERE guild_id = $1 AND id = ANY($2) AND mentionable = true`,
							*guildID, editMentionRoleIDs)
						if qErr != nil {
							h.Logger.Warn("edit mention role validation query failed, clearing role mentions", slog.String("error", qErr.Error()))
							editMentionRoleIDs = nil
						} else {
							valid := map[string]bool{}
							for rows.Next() {
								var rid string
								if rows.Scan(&rid) == nil {
									valid[rid] = true
								}
							}
							rows.Close()
							filtered := editMentionRoleIDs[:0]
							for _, id := range editMentionRoleIDs {
								if valid[id] {
									filtered = append(filtered, id)
								}
							}
							editMentionRoleIDs = filtered
						}
					}
				}
			}
		} else {
			// DMs: validate user mentions against channel recipients.
			if len(editMentionUserIDs) > 0 {
				rows, qErr := h.Pool.Query(r.Context(),
					`SELECT user_id FROM channel_recipients WHERE channel_id = $1 AND user_id = ANY($2)`,
					channelID, editMentionUserIDs)
				if qErr != nil {
					h.Logger.Warn("edit mention user validation query failed, clearing mentions", slog.String("error", qErr.Error()))
					editMentionUserIDs = nil
				} else {
					valid := map[string]bool{}
					for rows.Next() {
						var uid string
						if rows.Scan(&uid) == nil {
							valid[uid] = true
						}
					}
					rows.Close()
					filtered := editMentionUserIDs[:0]
					for _, id := range editMentionUserIDs {
						if valid[id] {
							filtered = append(filtered, id)
						}
					}
					editMentionUserIDs = filtered
				}
			}
		}
	}

	var msg models.Message
	err = h.Pool.QueryRow(r.Context(),
		`UPDATE messages SET content = $3, edited_at = now(),
		        mention_user_ids = $4, mention_role_ids = $5, mention_here = $6
		 WHERE id = $1 AND channel_id = $2
		 RETURNING id, channel_id, author_id, content, nonce, message_type, edited_at, flags,
		           reply_to_ids, mention_user_ids, mention_role_ids, mention_here,
		           thread_id, masquerade_name, masquerade_avatar, masquerade_color,
		           encrypted, encryption_session_id, created_at`,
		messageID, channelID, req.Content, editMentionUserIDs, editMentionRoleIDs, editMentionHere,
	).Scan(
		&msg.ID, &msg.ChannelID, &msg.AuthorID, &msg.Content, &msg.Nonce, &msg.MessageType,
		&msg.EditedAt, &msg.Flags, &msg.ReplyToIDs, &msg.MentionUserIDs, &msg.MentionRoleIDs,
		&msg.MentionHere, &msg.ThreadID, &msg.MasqueradeName, &msg.MasqueradeAvatar,
		&msg.MasqueradeColor, &msg.Encrypted, &msg.EncryptionSessionID, &msg.CreatedAt,
	)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update message")
		return
	}

	h.enrichMessageWithAuthor(r.Context(), &msg)

	h.EventBus.Publish(r.Context(), events.SubjectMessageUpdate, events.Event{
		Type:      "MESSAGE_UPDATE",
		ChannelID: channelID,
		Data:      mustMarshal(msg),
	})

	apiutil.WriteJSON(w, http.StatusOK, msg)
}

// HandleGetMessageEdits returns the edit history for a message.
// GET /api/v1/channels/{channelID}/messages/{messageID}/edits
func (h *Handler) HandleGetMessageEdits(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")

	// Permission check: ReadHistory.
	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ReadHistory) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need READ_HISTORY permission")
		return
	}

	// Verify the message exists in this channel.
	var exists bool
	h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM messages WHERE id = $1 AND channel_id = $2)`,
		messageID, channelID).Scan(&exists)
	if !exists {
		apiutil.WriteError(w, http.StatusNotFound, "message_not_found", "Message not found")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, message_id, content, edited_at
		 FROM message_edits WHERE message_id = $1
		 ORDER BY edited_at DESC
		 LIMIT 50`,
		messageID,
	)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get edit history")
		return
	}
	defer rows.Close()

	type editEntry struct {
		ID        string    `json:"id"`
		MessageID string    `json:"message_id"`
		Content   string    `json:"content"`
		EditedAt  time.Time `json:"edited_at"`
	}

	edits := make([]editEntry, 0)
	for rows.Next() {
		var e editEntry
		if err := rows.Scan(&e.ID, &e.MessageID, &e.Content, &e.EditedAt); err != nil {
			apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to read edit history")
			return
		}
		edits = append(edits, e)
	}

	apiutil.WriteJSON(w, http.StatusOK, edits)
}

// HandleDeleteMessage deletes a message. Author or users with MANAGE_MESSAGES can delete.
// DELETE /api/v1/channels/{channelID}/messages/{messageID}
func (h *Handler) HandleDeleteMessage(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")

	// Forward to home instance if channel belongs to a federated guild.
	if h.FedProxy != nil {
		var guildID *string
		var instanceID *string
		if err := h.Pool.QueryRow(r.Context(),
			`SELECT c.guild_id, g.instance_id FROM channels c
			 LEFT JOIN guilds g ON g.id = c.guild_id
			 WHERE c.id = $1`, channelID,
		).Scan(&guildID, &instanceID); err != nil && err != pgx.ErrNoRows {
			apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to look up channel")
			return
		}
		if guildID != nil && h.FedProxy.ProxyToHomeInstance(w, r, *guildID, instanceID, "message_delete", userID, map[string]string{"message_id": messageID}) {
			return
		}
	}

	// Check authorship (permission-based deletion requires guild context, simplified here).
	var authorID string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT author_id FROM messages WHERE id = $1 AND channel_id = $2`,
		messageID, channelID,
	).Scan(&authorID)
	if err != nil {
		apiutil.WriteError(w, http.StatusNotFound, "message_not_found", "Message not found")
		return
	}

	if authorID != userID {
		// Non-authors need MANAGE_MESSAGES permission in the guild.
		if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ManageMessages) {
			apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_MESSAGES permission to delete others' messages")
			return
		}
	}

	_, err = h.Pool.Exec(r.Context(),
		`DELETE FROM messages WHERE id = $1 AND channel_id = $2`, messageID, channelID)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete message")
		return
	}

	// Decrement reply_count for thread channels (fire-and-forget).
	h.Pool.Exec(r.Context(),
		`UPDATE channels SET reply_count = GREATEST(reply_count - 1, 0)
		 WHERE id = $1 AND parent_channel_id IS NOT NULL`,
		channelID)

	h.EventBus.Publish(r.Context(), events.SubjectMessageDelete, events.Event{
		Type:      "MESSAGE_DELETE",
		ChannelID: channelID,
		Data:      mustMarshal(map[string]string{"id": messageID, "channel_id": channelID}),
	})

	w.WriteHeader(http.StatusNoContent)
}

// HandleBulkDeleteMessages deletes multiple messages in a channel at once.
// POST /api/v1/channels/{channelID}/messages/bulk-delete
func (h *Handler) HandleBulkDeleteMessages(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ManageMessages) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_MESSAGES permission")
		return
	}

	var req struct {
		MessageIDs []string `json:"message_ids"`
	}
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	if len(req.MessageIDs) == 0 {
		apiutil.WriteError(w, http.StatusBadRequest, "empty_ids", "At least one message ID is required")
		return
	}
	if len(req.MessageIDs) > 100 {
		apiutil.WriteError(w, http.StatusBadRequest, "too_many_ids", "Cannot bulk delete more than 100 messages at once")
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM messages WHERE channel_id = $1 AND id = ANY($2)`,
		channelID, req.MessageIDs,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to delete messages", err)
		return
	}

	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "no_messages_found", "No matching messages found in this channel")
		return
	}

	h.EventBus.PublishChannelEvent(r.Context(), events.SubjectMessageDeleteBulk, "MESSAGE_DELETE_BULK", channelID, map[string]interface{}{
		"channel_id":  channelID,
		"message_ids": req.MessageIDs,
	})

	w.WriteHeader(http.StatusNoContent)
}

// HandleGetReactions returns aggregated reaction counts and the reacting users for a message.
// GET /api/v1/channels/{channelID}/messages/{messageID}/reactions
func (h *Handler) HandleGetReactions(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")

	// Permission check: ViewChannel.
	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ViewChannel) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need VIEW_CHANNEL permission")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT emoji, COUNT(*) as count, array_agg(user_id ORDER BY created_at) as users
		 FROM reactions WHERE message_id = $1
		 GROUP BY emoji
		 ORDER BY count DESC`,
		messageID,
	)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get reactions")
		return
	}
	defer rows.Close()

	type reactionGroup struct {
		Emoji string   `json:"emoji"`
		Count int      `json:"count"`
		Users []string `json:"users"`
	}

	reactions := make([]reactionGroup, 0)
	for rows.Next() {
		var rg reactionGroup
		if err := rows.Scan(&rg.Emoji, &rg.Count, &rg.Users); err != nil {
			apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to read reactions")
			return
		}
		reactions = append(reactions, rg)
	}

	apiutil.WriteJSON(w, http.StatusOK, reactions)
}

// HandleAddReaction adds an emoji reaction to a message.
// PUT /api/v1/channels/{channelID}/messages/{messageID}/reactions/{emoji}
func (h *Handler) HandleAddReaction(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")
	emoji := chi.URLParam(r, "emoji")

	// Permission check: AddReactions.
	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.AddReactions) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need ADD_REACTIONS permission")
		return
	}

	// Verify message exists in channel.
	var exists bool
	h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM messages WHERE id = $1 AND channel_id = $2)`,
		messageID, channelID,
	).Scan(&exists)
	if !exists {
		apiutil.WriteError(w, http.StatusNotFound, "message_not_found", "Message not found")
		return
	}

	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO reactions (message_id, user_id, emoji, created_at)
		 VALUES ($1, $2, $3, now())
		 ON CONFLICT (message_id, user_id, emoji) DO NOTHING`,
		messageID, userID, emoji,
	)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to add reaction")
		return
	}

	h.EventBus.PublishChannelEvent(r.Context(), events.SubjectMessageReactionAdd, "MESSAGE_REACTION_ADD", channelID, map[string]string{
		"message_id": messageID, "channel_id": channelID, "user_id": userID, "emoji": emoji,
	})

	w.WriteHeader(http.StatusNoContent)
}

// HandleRemoveReaction removes an emoji reaction from a message.
// DELETE /api/v1/channels/{channelID}/messages/{messageID}/reactions/{emoji}
func (h *Handler) HandleRemoveReaction(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")
	emoji := chi.URLParam(r, "emoji")

	_, err := h.Pool.Exec(r.Context(),
		`DELETE FROM reactions WHERE message_id = $1 AND user_id = $2 AND emoji = $3`,
		messageID, userID, emoji,
	)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to remove reaction")
		return
	}

	h.EventBus.PublishChannelEvent(r.Context(), events.SubjectMessageReactionDel, "MESSAGE_REACTION_REMOVE", channelID, map[string]string{
		"message_id": messageID, "user_id": userID, "emoji": emoji,
	})

	w.WriteHeader(http.StatusNoContent)
}

// HandleRemoveUserReaction removes another user's reaction (moderator action).
// DELETE /api/v1/channels/{channelID}/messages/{messageID}/reactions/{emoji}/{userID}
func (h *Handler) HandleRemoveUserReaction(w http.ResponseWriter, r *http.Request) {
	actorID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")
	emoji := chi.URLParam(r, "emoji")
	targetUserID := chi.URLParam(r, "targetUserID")

	// Permission check: ManageMessages required to remove others' reactions.
	if !h.hasChannelPermission(r.Context(), channelID, actorID, permissions.ManageMessages) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_MESSAGES permission")
		return
	}

	result, err := h.Pool.Exec(r.Context(),
		`DELETE FROM reactions WHERE message_id = $1 AND user_id = $2 AND emoji = $3`,
		messageID, targetUserID, emoji,
	)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to remove reaction")
		return
	}
	if result.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "reaction_not_found", "Reaction not found")
		return
	}

	h.EventBus.PublishChannelEvent(r.Context(), events.SubjectMessageReactionDel, "MESSAGE_REACTION_REMOVE", channelID, map[string]string{
		"message_id": messageID, "channel_id": channelID, "user_id": targetUserID, "emoji": emoji,
	})

	w.WriteHeader(http.StatusNoContent)
}

// HandleGetPins returns pinned messages in a channel.
// GET /api/v1/channels/{channelID}/pins
func (h *Handler) HandleGetPins(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	// Permission check: ViewChannel.
	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ViewChannel) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need VIEW_CHANNEL permission")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT m.id, m.channel_id, m.author_id, m.content, m.nonce, m.message_type,
		        m.edited_at, m.flags, m.reply_to_ids, m.mention_user_ids, m.mention_role_ids,
		        m.mention_here, m.thread_id, m.masquerade_name, m.masquerade_avatar,
		        m.masquerade_color, m.encrypted, m.encryption_session_id, m.created_at
		 FROM messages m
		 JOIN pins p ON m.id = p.message_id
		 WHERE p.channel_id = $1
		 ORDER BY p.pinned_at DESC`,
		channelID,
	)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get pins")
		return
	}
	defer rows.Close()

	messages := make([]models.Message, 0)
	for rows.Next() {
		var m models.Message
		if err := rows.Scan(
			&m.ID, &m.ChannelID, &m.AuthorID, &m.Content, &m.Nonce, &m.MessageType,
			&m.EditedAt, &m.Flags, &m.ReplyToIDs, &m.MentionUserIDs, &m.MentionRoleIDs,
			&m.MentionHere, &m.ThreadID, &m.MasqueradeName, &m.MasqueradeAvatar,
			&m.MasqueradeColor, &m.Encrypted, &m.EncryptionSessionID, &m.CreatedAt,
		); err != nil {
			apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to read pins")
			return
		}
		messages = append(messages, m)
	}

	apiutil.WriteJSON(w, http.StatusOK, messages)
}

// HandlePinMessage pins a message in a channel.
// PUT /api/v1/channels/{channelID}/pins/{messageID}
func (h *Handler) HandlePinMessage(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")

	// Forward to home instance if channel belongs to a federated guild.
	if h.FedProxy != nil {
		var guildID *string
		var instanceID *string
		if err := h.Pool.QueryRow(r.Context(),
			`SELECT c.guild_id, g.instance_id FROM channels c
			 LEFT JOIN guilds g ON g.id = c.guild_id
			 WHERE c.id = $1`, channelID,
		).Scan(&guildID, &instanceID); err != nil && err != pgx.ErrNoRows {
			apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to look up channel")
			return
		}
		if guildID != nil && h.FedProxy.ProxyToHomeInstance(w, r, *guildID, instanceID, "message_pin", userID, map[string]string{"message_id": messageID}) {
			return
		}
	}

	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ManageMessages) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_MESSAGES permission")
		return
	}

	// Verify message exists.
	var exists bool
	h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM messages WHERE id = $1 AND channel_id = $2)`,
		messageID, channelID,
	).Scan(&exists)
	if !exists {
		apiutil.WriteError(w, http.StatusNotFound, "message_not_found", "Message not found")
		return
	}

	// Enforce pin limit (50 per channel).
	var pinCount int
	h.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM pins WHERE channel_id = $1`, channelID).Scan(&pinCount)
	if pinCount >= 50 {
		apiutil.WriteError(w, http.StatusBadRequest, "pin_limit", "Channel has reached the maximum of 50 pinned messages")
		return
	}

	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO pins (channel_id, message_id, pinned_by, pinned_at)
		 VALUES ($1, $2, $3, now())
		 ON CONFLICT (channel_id, message_id) DO NOTHING`,
		channelID, messageID, userID,
	)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to pin message")
		return
	}

	h.EventBus.PublishChannelEvent(r.Context(), events.SubjectChannelPinsUpdate, "CHANNEL_PINS_UPDATE", channelID, map[string]string{
		"channel_id": channelID,
	})

	w.WriteHeader(http.StatusNoContent)
}

// HandleUnpinMessage unpins a message from a channel.
// DELETE /api/v1/channels/{channelID}/pins/{messageID}
func (h *Handler) HandleUnpinMessage(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")

	// Forward to home instance if channel belongs to a federated guild.
	if h.FedProxy != nil {
		var guildID *string
		var instanceID *string
		if err := h.Pool.QueryRow(r.Context(),
			`SELECT c.guild_id, g.instance_id FROM channels c
			 LEFT JOIN guilds g ON g.id = c.guild_id
			 WHERE c.id = $1`, channelID,
		).Scan(&guildID, &instanceID); err != nil && err != pgx.ErrNoRows {
			apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to look up channel")
			return
		}
		if guildID != nil && h.FedProxy.ProxyToHomeInstance(w, r, *guildID, instanceID, "message_unpin", userID, map[string]string{"message_id": messageID}) {
			return
		}
	}

	// Permission check: ManageMessages.
	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ManageMessages) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_MESSAGES permission")
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM pins WHERE channel_id = $1 AND message_id = $2`, channelID, messageID)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to unpin message")
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "pin_not_found", "Message is not pinned")
		return
	}

	h.EventBus.PublishChannelEvent(r.Context(), events.SubjectChannelPinsUpdate, "CHANNEL_PINS_UPDATE", channelID, map[string]string{
		"channel_id": channelID,
	})

	w.WriteHeader(http.StatusNoContent)
}

// HandleTriggerTyping sends a typing indicator event for the channel.
// POST /api/v1/channels/{channelID}/typing
func (h *Handler) HandleTriggerTyping(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	// Permission check: SendMessages (typing implies intent to send).
	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.SendMessages) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need SEND_MESSAGES permission")
		return
	}

	h.EventBus.PublishChannelEvent(r.Context(), events.SubjectTypingStart, "TYPING_START", channelID, map[string]string{
		"channel_id": channelID,
		"user_id":    userID,
	})

	w.WriteHeader(http.StatusNoContent)
}

// HandleAckChannel marks a channel as read up to the latest message.
// POST /api/v1/channels/{channelID}/ack
func (h *Handler) HandleAckChannel(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	// Permission check: ViewChannel.
	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ViewChannel) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need VIEW_CHANNEL permission")
		return
	}

	// Get the latest message ID for this channel.
	var lastMessageID *string
	h.Pool.QueryRow(r.Context(),
		`SELECT last_message_id FROM channels WHERE id = $1`, channelID,
	).Scan(&lastMessageID)

	if lastMessageID != nil {
		h.Pool.Exec(r.Context(),
			`INSERT INTO read_state (user_id, channel_id, last_read_id, mention_count)
			 VALUES ($1, $2, $3, 0)
			 ON CONFLICT (user_id, channel_id) DO UPDATE SET last_read_id = $3, mention_count = 0`,
			userID, channelID, lastMessageID,
		)
	}

	h.EventBus.PublishUserEvent(r.Context(), events.SubjectChannelAck, "CHANNEL_ACK", userID, map[string]string{
		"channel_id": channelID, "user_id": userID,
	})

	w.WriteHeader(http.StatusNoContent)
}

// HandleSetChannelPermission sets a permission override on a channel.
// PUT /api/v1/channels/{channelID}/permissions/{overrideID}
func (h *Handler) HandleSetChannelPermission(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	overrideID := chi.URLParam(r, "overrideID")

	// Permission check: ManageChannels.
	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ManageChannels) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_CHANNELS permission")
		return
	}

	var req permissionOverrideRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	if req.TargetType != "role" && req.TargetType != "user" {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_target_type", "Target type must be 'role' or 'user'")
		return
	}

	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO channel_permission_overrides (channel_id, target_type, target_id, permissions_allow, permissions_deny)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (channel_id, target_type, target_id) DO UPDATE
		 SET permissions_allow = $4, permissions_deny = $5`,
		channelID, req.TargetType, overrideID, req.PermissionsAllow, req.PermissionsDeny,
	)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to set permission override")
		return
	}

	h.EventBus.PublishChannelEvent(r.Context(), events.SubjectChannelUpdate, "CHANNEL_UPDATE", channelID, map[string]string{
		"channel_id": channelID,
	})

	apiutil.WriteJSON(w, http.StatusOK, models.ChannelPermissionOverride{
		ChannelID:        channelID,
		TargetType:       req.TargetType,
		TargetID:         overrideID,
		PermissionsAllow: req.PermissionsAllow,
		PermissionsDeny:  req.PermissionsDeny,
	})
}

// HandleDeleteChannelPermission removes a permission override from a channel.
// DELETE /api/v1/channels/{channelID}/permissions/{overrideID}
func (h *Handler) HandleDeleteChannelPermission(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	overrideID := chi.URLParam(r, "overrideID")

	// Permission check: ManageChannels.
	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ManageChannels) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_CHANNELS permission")
		return
	}

	_, err := h.Pool.Exec(r.Context(),
		`DELETE FROM channel_permission_overrides WHERE channel_id = $1 AND target_id = $2`,
		channelID, overrideID,
	)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete permission override")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleCreateThread creates a new thread from a message.
// POST /api/v1/channels/{channelID}/messages/{messageID}/threads
func (h *Handler) HandleCreateThread(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")

	// Permission check: SendMessages (thread creation requires ability to send).
	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.SendMessages) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need SEND_MESSAGES permission")
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" || len(req.Name) > 100 {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_name", "Thread name must be 1-100 characters")
		return
	}

	// Verify the parent message exists.
	var exists bool
	h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM messages WHERE id = $1 AND channel_id = $2)`,
		messageID, channelID).Scan(&exists)
	if !exists {
		apiutil.WriteError(w, http.StatusNotFound, "message_not_found", "Message not found")
		return
	}

	// Check the parent channel is in a guild and fetch its auto-archive duration + encryption flag.
	var guildID *string
	var parentAutoArchive int
	var parentEncrypted bool
	if err := h.Pool.QueryRow(r.Context(),
		`SELECT guild_id, default_auto_archive_duration, encrypted FROM channels WHERE id = $1`, channelID,
	).Scan(&guildID, &parentAutoArchive, &parentEncrypted); err != nil {
		if err == pgx.ErrNoRows {
			apiutil.WriteError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
			return
		}
		apiutil.InternalError(w, h.Logger, "Failed to query channel", err)
		return
	}
	if guildID == nil {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_channel", "Threads can only be created in guild channels")
		return
	}

	threadID := models.NewULID().String()

	var thread models.Channel
	err := apiutil.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		// Create the thread as a new channel linked to the guild, inheriting the parent's auto-archive duration.
		if err := tx.QueryRow(r.Context(),
			`INSERT INTO channels (id, guild_id, category_id, channel_type, name, owner_id, position, default_auto_archive_duration, encrypted, parent_channel_id, last_activity_at, created_at)
			 VALUES ($1, $2, NULL, 'text', $3, $4, 0, $5, $6, $7, now(), now())
			 RETURNING id, guild_id, category_id, channel_type, name, topic, position,
			           slowmode_seconds, nsfw, encrypted, last_message_id, owner_id,
			           default_permissions, user_limit, bitrate, locked, locked_by, locked_at,
			           archived, read_only, read_only_role_ids, default_auto_archive_duration,
			           parent_channel_id, last_activity_at, created_at`,
			threadID, guildID, req.Name, userID, parentAutoArchive, parentEncrypted, channelID,
		).Scan(
			&thread.ID, &thread.GuildID, &thread.CategoryID, &thread.ChannelType, &thread.Name,
			&thread.Topic, &thread.Position, &thread.SlowmodeSeconds, &thread.NSFW, &thread.Encrypted,
			&thread.LastMessageID, &thread.OwnerID, &thread.DefaultPermissions,
			&thread.UserLimit, &thread.Bitrate,
			&thread.Locked, &thread.LockedBy, &thread.LockedAt,
			&thread.Archived, &thread.ReadOnly, &thread.ReadOnlyRoleIDs,
			&thread.DefaultAutoArchiveDuration, &thread.ParentChannelID, &thread.LastActivityAt, &thread.CreatedAt,
		); err != nil {
			return err
		}

		// Link the parent message to the thread.
		if _, err := tx.Exec(r.Context(),
			`UPDATE messages SET thread_id = $1 WHERE id = $2 AND channel_id = $3`,
			threadID, messageID, channelID); err != nil {
			return err
		}

		// Create a system message about thread creation.
		sysMsgID := models.NewULID().String()
		if _, err := tx.Exec(r.Context(),
			`INSERT INTO messages (id, channel_id, author_id, content, message_type, created_at)
			 VALUES ($1, $2, $3, $4, $5, now())`,
			sysMsgID, channelID, userID, req.Name, models.MessageTypeThreadCreated); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to create thread", err)
		return
	}

	threadGuildID := ""
	if guildID != nil {
		threadGuildID = *guildID
	}
	h.EventBus.Publish(r.Context(), events.SubjectChannelCreate, events.Event{
		Type:    "THREAD_CREATE",
		GuildID: threadGuildID,
		Data:    mustMarshal(thread),
	})

	apiutil.WriteJSON(w, http.StatusCreated, thread)
}

// HandleGetThreads lists active threads in a channel.
// GET /api/v1/channels/{channelID}/threads
func (h *Handler) HandleGetThreads(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	// Permission check: ViewChannel.
	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ViewChannel) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need VIEW_CHANNEL permission")
		return
	}

	// Get the guild_id of this channel so we can find threads.
	var guildID *string
	h.Pool.QueryRow(r.Context(),
		`SELECT guild_id FROM channels WHERE id = $1`, channelID).Scan(&guildID)
	if guildID == nil {
		apiutil.WriteError(w, http.StatusNotFound, "channel_not_found", "Channel not found or is not a guild channel")
		return
	}

	// Auto-archive expired threads on fetch: any thread whose last_activity_at is older
	// than its default_auto_archive_duration gets archived automatically.
	h.Pool.Exec(r.Context(),
		`UPDATE channels SET archived = TRUE
		 WHERE parent_channel_id = $1
		   AND archived = FALSE
		   AND default_auto_archive_duration > 0
		   AND COALESCE(last_activity_at, created_at) < now() - make_interval(mins => default_auto_archive_duration)`,
		channelID,
	)

	// Find all threads for this parent channel using parent_channel_id.
	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, guild_id, category_id, channel_type, name, topic,
		        position, slowmode_seconds, nsfw, encrypted, last_message_id,
		        owner_id, default_permissions, user_limit, bitrate, locked, locked_by, locked_at,
		        archived, read_only, read_only_role_ids, default_auto_archive_duration,
		        parent_channel_id, last_activity_at, created_at
		 FROM channels
		 WHERE parent_channel_id = $1
		 ORDER BY last_activity_at DESC NULLS LAST
		 LIMIT 50`,
		channelID,
	)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get threads")
		return
	}
	defer rows.Close()

	threads := make([]models.Channel, 0)
	for rows.Next() {
		var c models.Channel
		if err := rows.Scan(
			&c.ID, &c.GuildID, &c.CategoryID, &c.ChannelType, &c.Name, &c.Topic,
			&c.Position, &c.SlowmodeSeconds, &c.NSFW, &c.Encrypted, &c.LastMessageID,
			&c.OwnerID, &c.DefaultPermissions, &c.UserLimit, &c.Bitrate,
			&c.Locked, &c.LockedBy, &c.LockedAt,
			&c.Archived, &c.ReadOnly, &c.ReadOnlyRoleIDs,
			&c.DefaultAutoArchiveDuration, &c.ParentChannelID, &c.LastActivityAt, &c.CreatedAt,
		); err != nil {
			apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to read threads")
			return
		}
		threads = append(threads, c)
	}

	apiutil.WriteJSON(w, http.StatusOK, threads)
}

// HandleHideThread hides a thread for the current user.
// POST /api/v1/channels/{channelID}/threads/{threadID}/hide
func (h *Handler) HandleHideThread(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	threadID := chi.URLParam(r, "threadID")

	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ViewChannel) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need VIEW_CHANNEL permission")
		return
	}

	var exists bool
	if err := h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM channels WHERE id = $1 AND parent_channel_id = $2)`,
		threadID, channelID,
	).Scan(&exists); err != nil || !exists {
		apiutil.WriteError(w, http.StatusNotFound, "thread_not_found", "Thread not found in this channel")
		return
	}

	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO user_hidden_threads (user_id, thread_id, hidden_at)
		 VALUES ($1, $2, now())
		 ON CONFLICT DO NOTHING`,
		userID, threadID,
	)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to hide thread")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleUnhideThread unhides a thread for the current user.
// DELETE /api/v1/channels/{channelID}/threads/{threadID}/hide
func (h *Handler) HandleUnhideThread(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	threadID := chi.URLParam(r, "threadID")

	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ViewChannel) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need VIEW_CHANNEL permission")
		return
	}

	var exists bool
	if err := h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM channels WHERE id = $1 AND parent_channel_id = $2)`,
		threadID, channelID,
	).Scan(&exists); err != nil || !exists {
		apiutil.WriteError(w, http.StatusNotFound, "thread_not_found", "Thread not found in this channel")
		return
	}

	_, err := h.Pool.Exec(r.Context(),
		`DELETE FROM user_hidden_threads WHERE user_id = $1 AND thread_id = $2`,
		userID, threadID,
	)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to unhide thread")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleGetHiddenThreads returns the list of thread IDs hidden by the current user.
// GET /api/v1/users/@me/hidden-threads
func (h *Handler) HandleGetHiddenThreads(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	rows, err := h.Pool.Query(r.Context(),
		`SELECT thread_id FROM user_hidden_threads WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get hidden threads")
		return
	}
	defer rows.Close()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to read hidden threads")
			return
		}
		ids = append(ids, id)
	}

	apiutil.WriteJSON(w, http.StatusOK, ids)
}

// --- Scheduled Messages ---

// HandleScheduleMessage creates a scheduled message for future delivery.
// POST /api/v1/channels/{channelID}/scheduled-messages
func (h *Handler) HandleScheduleMessage(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	// Permission check: SendMessages.
	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.SendMessages) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need SEND_MESSAGES permission")
		return
	}

	var req scheduleMessageRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	hasContent := req.Content != nil && *req.Content != ""
	hasAttachments := len(req.AttachmentIDs) > 0
	if !hasContent && !hasAttachments {
		apiutil.WriteError(w, http.StatusBadRequest, "empty_content", "Scheduled message content or attachments required")
		return
	}

	if hasContent && len(*req.Content) > 4000 {
		apiutil.WriteError(w, http.StatusBadRequest, "content_too_long", "Message content must be at most 4000 characters")
		return
	}

	scheduledFor, err := time.Parse(time.RFC3339, req.ScheduledFor)
	if err != nil {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_time", "scheduled_for must be a valid RFC3339 timestamp")
		return
	}

	if scheduledFor.Before(time.Now().Add(1 * time.Minute)) {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_time", "Scheduled time must be at least 1 minute in the future")
		return
	}

	msgID := models.NewULID().String()
	var scheduled models.ScheduledMessage
	err = h.Pool.QueryRow(r.Context(),
		`INSERT INTO scheduled_messages (id, channel_id, author_id, content, attachment_ids, scheduled_for, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, now())
		 RETURNING id, channel_id, author_id, content, attachment_ids, scheduled_for, created_at`,
		msgID, channelID, userID, req.Content, req.AttachmentIDs, scheduledFor,
	).Scan(
		&scheduled.ID, &scheduled.ChannelID, &scheduled.AuthorID, &scheduled.Content,
		&scheduled.AttachmentIDs, &scheduled.ScheduledFor, &scheduled.CreatedAt,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to schedule message", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusCreated, scheduled)
}

// HandleGetScheduledMessages lists a user's scheduled messages for a channel.
// GET /api/v1/channels/{channelID}/scheduled-messages
func (h *Handler) HandleGetScheduledMessages(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	// Permission check: ViewChannel.
	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ViewChannel) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need VIEW_CHANNEL permission")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, channel_id, author_id, content, attachment_ids, scheduled_for, created_at
		 FROM scheduled_messages
		 WHERE channel_id = $1 AND author_id = $2 AND scheduled_for > now()
		 ORDER BY scheduled_for ASC`,
		channelID, userID,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get scheduled messages", err)
		return
	}
	defer rows.Close()

	messages := make([]models.ScheduledMessage, 0)
	for rows.Next() {
		var m models.ScheduledMessage
		if err := rows.Scan(
			&m.ID, &m.ChannelID, &m.AuthorID, &m.Content,
			&m.AttachmentIDs, &m.ScheduledFor, &m.CreatedAt,
		); err != nil {
			apiutil.InternalError(w, h.Logger, "Failed to read scheduled messages", err)
			return
		}
		messages = append(messages, m)
	}

	apiutil.WriteJSON(w, http.StatusOK, messages)
}

// HandleDeleteScheduledMessage cancels a scheduled message.
// DELETE /api/v1/channels/{channelID}/scheduled-messages/{messageID}
func (h *Handler) HandleDeleteScheduledMessage(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")

	// Only the author can delete their own scheduled messages.
	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM scheduled_messages WHERE id = $1 AND channel_id = $2 AND author_id = $3`,
		messageID, channelID, userID,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to cancel scheduled message", err)
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Scheduled message not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Internal helpers ---

// HandleGetChannelWebhooks lists all webhooks for a channel.
// GET /api/v1/channels/{channelID}/webhooks
func (h *Handler) HandleGetChannelWebhooks(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	// Permission check: ManageWebhooks.
	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ManageWebhooks) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_WEBHOOKS permission")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, guild_id, channel_id, creator_id, name, avatar_id, token,
		        webhook_type, outgoing_url, created_at
		 FROM webhooks WHERE channel_id = $1
		 ORDER BY created_at DESC`,
		channelID,
	)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get webhooks")
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
			apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to read webhooks")
			return
		}
		webhooks = append(webhooks, wh)
	}

	apiutil.WriteJSON(w, http.StatusOK, webhooks)
}

func (h *Handler) getChannel(ctx context.Context, channelID string) (*models.Channel, error) {
	var c models.Channel
	err := h.Pool.QueryRow(ctx,
		`SELECT id, guild_id, category_id, channel_type, name, topic, position,
		        slowmode_seconds, nsfw, encrypted, last_message_id, owner_id,
		        default_permissions, user_limit, bitrate, locked, locked_by, locked_at,
		        archived, read_only, read_only_role_ids, default_auto_archive_duration,
		        parent_channel_id, last_activity_at, created_at
		 FROM channels WHERE id = $1`,
		channelID,
	).Scan(
		&c.ID, &c.GuildID, &c.CategoryID, &c.ChannelType, &c.Name, &c.Topic,
		&c.Position, &c.SlowmodeSeconds, &c.NSFW, &c.Encrypted, &c.LastMessageID,
		&c.OwnerID, &c.DefaultPermissions, &c.UserLimit, &c.Bitrate,
		&c.Locked, &c.LockedBy, &c.LockedAt,
		&c.Archived, &c.ReadOnly, &c.ReadOnlyRoleIDs,
		&c.DefaultAutoArchiveDuration, &c.ParentChannelID, &c.LastActivityAt, &c.CreatedAt,
	)
	return &c, err
}

func (h *Handler) getMessage(ctx context.Context, channelID, messageID string) (*models.Message, error) {
	var m models.Message
	err := h.Pool.QueryRow(ctx,
		`SELECT id, channel_id, author_id, content, nonce, message_type, edited_at, flags,
		        reply_to_ids, mention_user_ids, mention_role_ids, mention_here,
		        thread_id, masquerade_name, masquerade_avatar, masquerade_color,
		        encrypted, encryption_session_id, created_at
		 FROM messages WHERE id = $1 AND channel_id = $2`,
		messageID, channelID,
	).Scan(
		&m.ID, &m.ChannelID, &m.AuthorID, &m.Content, &m.Nonce, &m.MessageType,
		&m.EditedAt, &m.Flags, &m.ReplyToIDs, &m.MentionUserIDs, &m.MentionRoleIDs,
		&m.MentionHere, &m.ThreadID, &m.MasqueradeName, &m.MasqueradeAvatar,
		&m.MasqueradeColor, &m.Encrypted, &m.EncryptionSessionID, &m.CreatedAt,
	)
	return &m, err
}

func (h *Handler) loadAttachments(ctx context.Context, messageID string) []models.Attachment {
	rows, err := h.Pool.Query(ctx,
		`SELECT id, message_id, uploader_id, filename, content_type, size_bytes,
		        width, height, duration_seconds, s3_bucket, s3_key, blurhash, alt_text, created_at
		 FROM attachments WHERE message_id = $1
		 ORDER BY created_at`,
		messageID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var attachments []models.Attachment
	for rows.Next() {
		var a models.Attachment
		if err := rows.Scan(
			&a.ID, &a.MessageID, &a.UploaderID, &a.Filename, &a.ContentType, &a.SizeBytes,
			&a.Width, &a.Height, &a.DurationSeconds, &a.S3Bucket, &a.S3Key, &a.Blurhash, &a.AltText, &a.CreatedAt,
		); err != nil {
			return nil
		}
		attachments = append(attachments, a)
	}
	return attachments
}

func (h *Handler) loadEmbeds(ctx context.Context, messageID string) []models.Embed {
	rows, err := h.Pool.Query(ctx,
		`SELECT id, message_id, embed_type, url, title, description, site_name,
		        icon_url, color, image_url, image_width, image_height,
		        video_url, special_type, special_id, created_at
		 FROM embeds WHERE message_id = $1
		 ORDER BY created_at`,
		messageID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var embeds []models.Embed
	for rows.Next() {
		var e models.Embed
		if err := rows.Scan(
			&e.ID, &e.MessageID, &e.EmbedType, &e.URL, &e.Title, &e.Description,
			&e.SiteName, &e.IconURL, &e.Color, &e.ImageURL, &e.ImageWidth, &e.ImageHeight,
			&e.VideoURL, &e.SpecialType, &e.SpecialID, &e.CreatedAt,
		); err != nil {
			return nil
		}
		embeds = append(embeds, e)
	}
	return embeds
}

// enrichMessagesWithAuthors fetches author user data for a batch of messages
// and populates the Author field on each message.
func (h *Handler) enrichMessagesWithAuthors(ctx context.Context, messages []models.Message) {
	if len(messages) == 0 {
		return
	}

	// Collect unique author IDs.
	authorIDs := make(map[string]struct{})
	for _, m := range messages {
		authorIDs[m.AuthorID] = struct{}{}
	}

	ids := make([]string, 0, len(authorIDs))
	for id := range authorIDs {
		ids = append(ids, id)
	}

	rows, err := h.Pool.Query(ctx,
		`SELECT id, instance_id, username, display_name, avatar_id,
		        status_text, status_emoji, status_presence, status_expires_at,
		        bio, banner_id, accent_color, pronouns, flags, created_at
		 FROM users WHERE id = ANY($1)`, ids)
	if err != nil {
		return
	}
	defer rows.Close()

	userMap := make(map[string]*models.User)
	for rows.Next() {
		var u models.User
		if err := rows.Scan(
			&u.ID, &u.InstanceID, &u.Username, &u.DisplayName, &u.AvatarID,
			&u.StatusText, &u.StatusEmoji, &u.StatusPresence, &u.StatusExpiresAt,
			&u.Bio, &u.BannerID, &u.AccentColor, &u.Pronouns, &u.Flags, &u.CreatedAt,
		); err != nil {
			continue
		}
		userCopy := u
		userMap[u.ID] = &userCopy
	}

	for i := range messages {
		if u, ok := userMap[messages[i].AuthorID]; ok {
			messages[i].Author = u
		}
	}
}

// enrichMessagesWithAttachments batch-loads attachments for a list of messages
// using a single query to avoid N+1.
func (h *Handler) enrichMessagesWithAttachments(ctx context.Context, messages []models.Message) {
	if len(messages) == 0 {
		return
	}

	msgIDs := make([]string, len(messages))
	for i, m := range messages {
		msgIDs[i] = m.ID
	}

	rows, err := h.Pool.Query(ctx,
		`SELECT id, message_id, uploader_id, filename, content_type, size_bytes,
		        width, height, duration_seconds, s3_bucket, s3_key, blurhash, alt_text, created_at
		 FROM attachments WHERE message_id = ANY($1)
		 ORDER BY created_at`, msgIDs)
	if err != nil {
		return
	}
	defer rows.Close()

	attachMap := make(map[string][]models.Attachment)
	for rows.Next() {
		var a models.Attachment
		if err := rows.Scan(
			&a.ID, &a.MessageID, &a.UploaderID, &a.Filename, &a.ContentType, &a.SizeBytes,
			&a.Width, &a.Height, &a.DurationSeconds, &a.S3Bucket, &a.S3Key, &a.Blurhash, &a.AltText, &a.CreatedAt,
		); err != nil {
			continue
		}
		if a.MessageID != nil {
			attachMap[*a.MessageID] = append(attachMap[*a.MessageID], a)
		}
	}

	for i := range messages {
		if atts, ok := attachMap[messages[i].ID]; ok {
			messages[i].Attachments = atts
		}
	}
}

// enrichMessagesWithEmbeds batch-loads embeds for a list of messages.
func (h *Handler) enrichMessagesWithEmbeds(ctx context.Context, messages []models.Message) {
	if len(messages) == 0 {
		return
	}

	msgIDs := make([]string, len(messages))
	for i, m := range messages {
		msgIDs[i] = m.ID
	}

	rows, err := h.Pool.Query(ctx,
		`SELECT id, message_id, embed_type, url, title, description, site_name,
		        icon_url, color, image_url, image_width, image_height,
		        video_url, special_type, special_id, created_at
		 FROM embeds WHERE message_id = ANY($1)
		 ORDER BY created_at`, msgIDs)
	if err != nil {
		return
	}
	defer rows.Close()

	embedMap := make(map[string][]models.Embed)
	for rows.Next() {
		var e models.Embed
		if err := rows.Scan(
			&e.ID, &e.MessageID, &e.EmbedType, &e.URL, &e.Title, &e.Description,
			&e.SiteName, &e.IconURL, &e.Color, &e.ImageURL, &e.ImageWidth, &e.ImageHeight,
			&e.VideoURL, &e.SpecialType, &e.SpecialID, &e.CreatedAt,
		); err != nil {
			continue
		}
		embedMap[e.MessageID] = append(embedMap[e.MessageID], e)
	}

	for i := range messages {
		if embs, ok := embedMap[messages[i].ID]; ok {
			messages[i].Embeds = embs
		}
	}
}

// enrichMessageWithAuthor fetches author user data for a single message.
// Joins the instances table to populate InstanceDomain for federation badges.
func (h *Handler) enrichMessageWithAuthor(ctx context.Context, msg *models.Message) {
	var u models.User
	var instanceDomain string
	err := h.Pool.QueryRow(ctx,
		`SELECT u.id, u.instance_id, u.username, u.display_name, u.avatar_id,
		        u.status_text, u.status_emoji, u.status_presence, u.status_expires_at,
		        u.bio, u.banner_id, u.accent_color, u.pronouns, u.flags, u.created_at,
		        COALESCE(i.domain, '')
		 FROM users u LEFT JOIN instances i ON i.id = u.instance_id
		 WHERE u.id = $1`, msg.AuthorID).Scan(
		&u.ID, &u.InstanceID, &u.Username, &u.DisplayName, &u.AvatarID,
		&u.StatusText, &u.StatusEmoji, &u.StatusPresence, &u.StatusExpiresAt,
		&u.Bio, &u.BannerID, &u.AccentColor, &u.Pronouns, &u.Flags, &u.CreatedAt,
		&instanceDomain,
	)
	if err == nil {
		if instanceDomain != "" {
			u.InstanceDomain = &instanceDomain
		}
		msg.Author = &u
	}
}

// HandleCrosspostMessage forwards a message to another channel.
// POST /api/v1/channels/{channelID}/messages/{messageID}/crosspost
func (h *Handler) HandleCrosspostMessage(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	sourceChannelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")

	var req struct {
		TargetChannelID string `json:"target_channel_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TargetChannelID == "" {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_body", "target_channel_id is required")
		return
	}

	if req.TargetChannelID == sourceChannelID {
		apiutil.WriteError(w, http.StatusBadRequest, "same_channel", "Cannot crosspost to the same channel")
		return
	}

	// Check permission in target channel's guild.
	if !h.hasChannelPermission(r.Context(), req.TargetChannelID, userID, permissions.SendMessages) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need SEND_MESSAGES permission in the target channel")
		return
	}

	// Fetch the original message.
	var content *string
	var authorID string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT author_id, content FROM messages WHERE id = $1 AND channel_id = $2`,
		messageID, sourceChannelID,
	).Scan(&authorID, &content)
	if err != nil {
		apiutil.WriteError(w, http.StatusNotFound, "message_not_found", "Source message not found")
		return
	}

	// Create the forwarded message in the target channel.
	newMsgID := models.NewULID().String()
	var msg models.Message
	err = h.Pool.QueryRow(r.Context(),
		`INSERT INTO messages (id, channel_id, author_id, content, message_type, flags, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, now())
		 RETURNING id, channel_id, author_id, content, nonce, message_type, edited_at, flags,
		           reply_to_ids, mention_user_ids, mention_role_ids, mention_here,
		           thread_id, masquerade_name, masquerade_avatar, masquerade_color,
		           encrypted, encryption_session_id, created_at`,
		newMsgID, req.TargetChannelID, userID, content, models.MessageTypeDefault, models.MessageFlagCrosspost,
	).Scan(
		&msg.ID, &msg.ChannelID, &msg.AuthorID, &msg.Content, &msg.Nonce, &msg.MessageType,
		&msg.EditedAt, &msg.Flags, &msg.ReplyToIDs, &msg.MentionUserIDs, &msg.MentionRoleIDs,
		&msg.MentionHere, &msg.ThreadID, &msg.MasqueradeName, &msg.MasqueradeAvatar,
		&msg.MasqueradeColor, &msg.Encrypted, &msg.EncryptionSessionID, &msg.CreatedAt,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to crosspost message", err)
		return
	}

	h.Pool.Exec(r.Context(),
		`UPDATE channels SET last_message_id = $1 WHERE id = $2`, newMsgID, req.TargetChannelID)

	h.EventBus.PublishChannelEvent(r.Context(), events.SubjectMessageCreate, "MESSAGE_CREATE", req.TargetChannelID, msg)

	apiutil.WriteJSON(w, http.StatusCreated, msg)
}

// --- Announcement Channel Handlers ---

// HandleFollowChannel subscribes a webhook to an announcement channel so that
// new messages are automatically forwarded to the webhook's target channel.
// POST /api/v1/channels/{channelID}/followers
func (h *Handler) HandleFollowChannel(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	// Permission check: ManageWebhooks in the source channel's guild.
	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ManageWebhooks) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_WEBHOOKS permission")
		return
	}

	// Verify the source channel is an announcement channel.
	var channelType string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT channel_type FROM channels WHERE id = $1`, channelID,
	).Scan(&channelType)
	if err != nil {
		apiutil.WriteError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
		return
	}
	if channelType != models.ChannelTypeAnnouncement {
		apiutil.WriteError(w, http.StatusBadRequest, "not_announcement", "Only announcement channels can be followed")
		return
	}

	var req struct {
		WebhookID string `json:"webhook_id"`
		GuildID   string `json:"guild_id"`
	}
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}
	if req.WebhookID == "" || req.GuildID == "" {
		apiutil.WriteError(w, http.StatusBadRequest, "missing_fields", "webhook_id and guild_id are required")
		return
	}

	// Verify the webhook exists and belongs to the specified guild.
	var webhookExists bool
	h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM webhooks WHERE id = $1 AND guild_id = $2)`,
		req.WebhookID, req.GuildID,
	).Scan(&webhookExists)
	if !webhookExists {
		apiutil.WriteError(w, http.StatusNotFound, "webhook_not_found", "Webhook not found in the specified guild")
		return
	}

	followerID := models.NewULID().String()
	var follower models.ChannelFollower
	err = h.Pool.QueryRow(r.Context(),
		`INSERT INTO channel_followers (id, channel_id, webhook_id, guild_id, created_at)
		 VALUES ($1, $2, $3, $4, now())
		 RETURNING id, channel_id, webhook_id, guild_id, created_at`,
		followerID, channelID, req.WebhookID, req.GuildID,
	).Scan(&follower.ID, &follower.ChannelID, &follower.WebhookID, &follower.GuildID, &follower.CreatedAt)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to follow channel", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusCreated, follower)
}

// HandleGetChannelFollowers returns the list of followers for an announcement channel.
// GET /api/v1/channels/{channelID}/followers
func (h *Handler) HandleGetChannelFollowers(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	// Permission check: ManageWebhooks in the source channel's guild.
	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ManageWebhooks) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_WEBHOOKS permission")
		return
	}

	// Verify the channel is an announcement channel.
	var channelType string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT channel_type FROM channels WHERE id = $1`, channelID,
	).Scan(&channelType)
	if err != nil {
		apiutil.WriteError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
		return
	}
	if channelType != models.ChannelTypeAnnouncement {
		apiutil.WriteError(w, http.StatusBadRequest, "not_announcement", "Only announcement channels have followers")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, channel_id, webhook_id, guild_id, created_at
		 FROM channel_followers WHERE channel_id = $1
		 ORDER BY created_at DESC`,
		channelID,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get followers", err)
		return
	}
	defer rows.Close()

	followers := make([]models.ChannelFollower, 0)
	for rows.Next() {
		var f models.ChannelFollower
		if err := rows.Scan(&f.ID, &f.ChannelID, &f.WebhookID, &f.GuildID, &f.CreatedAt); err != nil {
			apiutil.InternalError(w, h.Logger, "Failed to read followers", err)
			return
		}
		followers = append(followers, f)
	}

	apiutil.WriteJSON(w, http.StatusOK, followers)
}

// HandleUnfollowChannel removes a follower subscription from an announcement channel.
// DELETE /api/v1/channels/{channelID}/followers/{followerID}
func (h *Handler) HandleUnfollowChannel(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	followerID := chi.URLParam(r, "followerID")

	// Permission check: ManageWebhooks in the source channel's guild.
	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ManageWebhooks) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_WEBHOOKS permission")
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM channel_followers WHERE id = $1 AND channel_id = $2`,
		followerID, channelID,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to unfollow channel", err)
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "follower_not_found", "Follower not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandlePublishMessage publishes a message from an announcement channel to all
// followers by creating crosspost messages via each follower's webhook.
// POST /api/v1/channels/{channelID}/messages/{messageID}/crosspost
func (h *Handler) HandlePublishMessage(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")

	// Permission check: SendMessages in the announcement channel.
	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.SendMessages) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need SEND_MESSAGES permission")
		return
	}

	// Verify the source channel is an announcement channel.
	var channelType string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT channel_type FROM channels WHERE id = $1`, channelID,
	).Scan(&channelType)
	if err != nil {
		apiutil.WriteError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
		return
	}
	if channelType != models.ChannelTypeAnnouncement {
		apiutil.WriteError(w, http.StatusBadRequest, "not_announcement", "Only messages in announcement channels can be published")
		return
	}

	// Fetch the source message.
	var content *string
	var authorID string
	var flags int
	err = h.Pool.QueryRow(r.Context(),
		`SELECT author_id, content, flags FROM messages WHERE id = $1 AND channel_id = $2`,
		messageID, channelID,
	).Scan(&authorID, &content, &flags)
	if err != nil {
		apiutil.WriteError(w, http.StatusNotFound, "message_not_found", "Message not found")
		return
	}

	// Check if message is already published (crosspost flag set).
	if flags&models.MessageFlagCrosspost != 0 {
		apiutil.WriteError(w, http.StatusBadRequest, "already_published", "This message has already been published")
		return
	}

	// Get all followers for this announcement channel.
	rows, err := h.Pool.Query(r.Context(),
		`SELECT cf.id, cf.webhook_id, cf.guild_id, w.channel_id, w.name, w.avatar_id
		 FROM channel_followers cf
		 JOIN webhooks w ON cf.webhook_id = w.id
		 WHERE cf.channel_id = $1`,
		channelID,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get followers", err)
		return
	}
	defer rows.Close()

	type followerInfo struct {
		FollowerID       string
		WebhookID        string
		GuildID          string
		TargetChannelID  string
		WebhookName      string
		WebhookAvatarID  *string
	}

	var followers []followerInfo
	for rows.Next() {
		var fi followerInfo
		if err := rows.Scan(&fi.FollowerID, &fi.WebhookID, &fi.GuildID, &fi.TargetChannelID, &fi.WebhookName, &fi.WebhookAvatarID); err != nil {
			h.Logger.Error("failed to scan follower info", slog.String("error", err.Error()))
			continue
		}
		followers = append(followers, fi)
	}

	// Create a crosspost message in each follower's target channel.
	for _, fi := range followers {
		newMsgID := models.NewULID().String()
		_, err := h.Pool.Exec(r.Context(),
			`INSERT INTO messages (id, channel_id, author_id, content, message_type, flags, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, now())`,
			newMsgID, fi.TargetChannelID, authorID, content,
			models.MessageTypeDefault, models.MessageFlagCrosspost,
		)
		if err != nil {
			h.Logger.Error("failed to crosspost to follower",
				slog.String("follower_id", fi.FollowerID),
				slog.String("target_channel_id", fi.TargetChannelID),
				slog.String("error", err.Error()),
			)
			continue
		}

		// Update last_message_id on the target channel.
		h.Pool.Exec(r.Context(),
			`UPDATE channels SET last_message_id = $1 WHERE id = $2`,
			newMsgID, fi.TargetChannelID)

		// Publish a message create event for each crossposted message.
		h.EventBus.PublishChannelEvent(r.Context(), events.SubjectMessageCreate, "MESSAGE_CREATE", fi.TargetChannelID, map[string]interface{}{
			"id":         newMsgID,
			"channel_id": fi.TargetChannelID,
			"author_id":  authorID,
			"content":    content,
			"flags":      models.MessageFlagCrosspost,
		})
	}

	// Mark the original message as published by setting the crosspost flag.
	h.Pool.Exec(r.Context(),
		`UPDATE messages SET flags = flags | $1 WHERE id = $2 AND channel_id = $3`,
		models.MessageFlagCrosspost, messageID, channelID)

	apiutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"message_id":      messageID,
		"channel_id":      channelID,
		"followers_count": len(followers),
	})
}

// --- Channel Templates ---

// HandleCreateChannelTemplate saves a channel configuration as a reusable template.
// POST /api/v1/guilds/{guildID}/channel-templates
func (h *Handler) HandleCreateChannelTemplate(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	// Permission check: ManageChannels required to create templates.
	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageChannels) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_CHANNELS permission")
		return
	}

	var req struct {
		Name                 string          `json:"name"`
		ChannelType          string          `json:"channel_type"`
		Topic                *string         `json:"topic"`
		SlowmodeSeconds      int             `json:"slowmode_seconds"`
		NSFW                 bool            `json:"nsfw"`
		PermissionOverwrites json.RawMessage `json:"permission_overwrites"`
	}
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	if req.Name == "" || len(req.Name) > 100 {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_name", "Template name must be 1-100 characters")
		return
	}

	validTypes := map[string]bool{
		"text": true, "voice": true, "announcement": true, "forum": true, "stage": true,
	}
	if req.ChannelType == "" {
		req.ChannelType = "text"
	}
	if !validTypes[req.ChannelType] {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_type", "Invalid channel type")
		return
	}

	if req.SlowmodeSeconds < 0 || req.SlowmodeSeconds > 21600 {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_slowmode", "Slowmode must be 0-21600 seconds")
		return
	}

	templateID := models.NewULID().String()

	var tmpl models.ChannelTemplate
	err := h.Pool.QueryRow(r.Context(),
		`INSERT INTO channel_templates (id, guild_id, name, channel_type, topic, slowmode_seconds, nsfw, permission_overwrites, created_by, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
		 RETURNING id, guild_id, name, channel_type, topic, slowmode_seconds, nsfw, permission_overwrites, created_by, created_at`,
		templateID, guildID, req.Name, req.ChannelType, req.Topic, req.SlowmodeSeconds,
		req.NSFW, req.PermissionOverwrites, userID,
	).Scan(
		&tmpl.ID, &tmpl.GuildID, &tmpl.Name, &tmpl.ChannelType, &tmpl.Topic,
		&tmpl.SlowmodeSeconds, &tmpl.NSFW, &tmpl.PermissionOverwrites,
		&tmpl.CreatedBy, &tmpl.CreatedAt,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to create template", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusCreated, tmpl)
}

// HandleGetChannelTemplates lists all channel templates for a guild.
// GET /api/v1/guilds/{guildID}/channel-templates
func (h *Handler) HandleGetChannelTemplates(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	// Any guild member can view templates.
	var isMember bool
	h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM guild_members WHERE guild_id = $1 AND user_id = $2)`,
		guildID, userID,
	).Scan(&isMember)
	if !isMember {
		apiutil.WriteError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, guild_id, name, channel_type, topic, slowmode_seconds, nsfw, permission_overwrites, created_by, created_at
		 FROM channel_templates WHERE guild_id = $1
		 ORDER BY created_at DESC`,
		guildID,
	)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get templates")
		return
	}
	defer rows.Close()

	templates := make([]models.ChannelTemplate, 0)
	for rows.Next() {
		var tmpl models.ChannelTemplate
		if err := rows.Scan(
			&tmpl.ID, &tmpl.GuildID, &tmpl.Name, &tmpl.ChannelType, &tmpl.Topic,
			&tmpl.SlowmodeSeconds, &tmpl.NSFW, &tmpl.PermissionOverwrites,
			&tmpl.CreatedBy, &tmpl.CreatedAt,
		); err != nil {
			apiutil.InternalError(w, h.Logger, "Failed to read templates", err)
			return
		}
		templates = append(templates, tmpl)
	}

	apiutil.WriteJSON(w, http.StatusOK, templates)
}

// HandleDeleteChannelTemplate deletes a channel template.
// DELETE /api/v1/guilds/{guildID}/channel-templates/{templateID}
func (h *Handler) HandleDeleteChannelTemplate(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	templateID := chi.URLParam(r, "templateID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageChannels) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_CHANNELS permission")
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM channel_templates WHERE id = $1 AND guild_id = $2`,
		templateID, guildID,
	)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete template")
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "template_not_found", "Template not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleApplyChannelTemplate creates a new channel from a saved template.
// POST /api/v1/guilds/{guildID}/channel-templates/{templateID}/apply
func (h *Handler) HandleApplyChannelTemplate(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	templateID := chi.URLParam(r, "templateID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageChannels) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_CHANNELS permission")
		return
	}

	var req struct {
		Name       string  `json:"name"`
		CategoryID *string `json:"category_id"`
	}
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	if req.Name == "" || len(req.Name) > 100 {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_name", "Channel name must be 1-100 characters")
		return
	}

	// Fetch the template.
	var tmpl models.ChannelTemplate
	err := h.Pool.QueryRow(r.Context(),
		`SELECT id, guild_id, name, channel_type, topic, slowmode_seconds, nsfw, permission_overwrites, created_by, created_at
		 FROM channel_templates WHERE id = $1 AND guild_id = $2`,
		templateID, guildID,
	).Scan(
		&tmpl.ID, &tmpl.GuildID, &tmpl.Name, &tmpl.ChannelType, &tmpl.Topic,
		&tmpl.SlowmodeSeconds, &tmpl.NSFW, &tmpl.PermissionOverwrites,
		&tmpl.CreatedBy, &tmpl.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			apiutil.WriteError(w, http.StatusNotFound, "template_not_found", "Template not found")
			return
		}
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get template")
		return
	}

	channelID := models.NewULID().String()

	var channel models.Channel
	err = apiutil.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		// Create the channel from the template.
		if err := tx.QueryRow(r.Context(),
			`INSERT INTO channels (id, guild_id, category_id, channel_type, name, topic, position, slowmode_seconds, nsfw, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, 0, $7, $8, now())
			 RETURNING id, guild_id, category_id, channel_type, name, topic, position,
			           slowmode_seconds, nsfw, encrypted, last_message_id, owner_id,
			           default_permissions, user_limit, bitrate, locked, locked_by, locked_at,
			           archived, read_only, read_only_role_ids, default_auto_archive_duration, created_at`,
			channelID, guildID, req.CategoryID, tmpl.ChannelType, req.Name, tmpl.Topic,
			tmpl.SlowmodeSeconds, tmpl.NSFW,
		).Scan(
			&channel.ID, &channel.GuildID, &channel.CategoryID, &channel.ChannelType, &channel.Name,
			&channel.Topic, &channel.Position, &channel.SlowmodeSeconds, &channel.NSFW, &channel.Encrypted,
			&channel.LastMessageID, &channel.OwnerID, &channel.DefaultPermissions,
			&channel.UserLimit, &channel.Bitrate,
			&channel.Locked, &channel.LockedBy, &channel.LockedAt,
			&channel.Archived, &channel.ReadOnly, &channel.ReadOnlyRoleIDs,
			&channel.DefaultAutoArchiveDuration, &channel.CreatedAt,
		); err != nil {
			return err
		}

		// Apply permission overwrites from the template if any.
		if len(tmpl.PermissionOverwrites) > 0 {
			var overwrites []struct {
				TargetType       string `json:"target_type"`
				TargetID         string `json:"target_id"`
				PermissionsAllow int64  `json:"permissions_allow"`
				PermissionsDeny  int64  `json:"permissions_deny"`
			}
			if jsonErr := json.Unmarshal(tmpl.PermissionOverwrites, &overwrites); jsonErr == nil {
				for _, ow := range overwrites {
					if _, err := tx.Exec(r.Context(),
						`INSERT INTO channel_permission_overrides (channel_id, target_type, target_id, permissions_allow, permissions_deny)
						 VALUES ($1, $2, $3, $4, $5)
						 ON CONFLICT (channel_id, target_id) DO UPDATE SET
						     permissions_allow = EXCLUDED.permissions_allow,
						     permissions_deny = EXCLUDED.permissions_deny`,
						channelID, ow.TargetType, ow.TargetID, ow.PermissionsAllow, ow.PermissionsDeny,
					); err != nil {
						return err
					}
				}
			}
		}

		return nil
	})
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create channel")
		return
	}

	h.EventBus.Publish(r.Context(), events.SubjectChannelCreate, events.Event{
		Type:    "CHANNEL_CREATE",
		GuildID: guildID,
		Data:    mustMarshal(channel),
	})

	apiutil.WriteJSON(w, http.StatusCreated, channel)
}

// hasGuildPermission checks if a user has a specific permission in a guild.
// Guild owners and instance admins always pass. Then checks default_permissions + role overrides.
func (h *Handler) hasGuildPermission(ctx context.Context, guildID, userID string, perm uint64) bool {
	// Owner has all permissions.
	var ownerID string
	if err := h.Pool.QueryRow(ctx, `SELECT owner_id FROM guilds WHERE id = $1`, guildID).Scan(&ownerID); err != nil {
		return false
	}
	if userID == ownerID {
		return true
	}

	// Admin flag.
	var userFlags int
	h.Pool.QueryRow(ctx, `SELECT flags FROM users WHERE id = $1`, userID).Scan(&userFlags)
	if userFlags&models.UserFlagAdmin != 0 {
		return true
	}

	// Default permissions + role permissions.
	var defaultPerms int64
	h.Pool.QueryRow(ctx, `SELECT default_permissions FROM guilds WHERE id = $1`, guildID).Scan(&defaultPerms)
	computed := uint64(defaultPerms)

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
			computed |= uint64(allow)
			computed &^= uint64(deny)
		}
	}

	if computed&permissions.Administrator != 0 {
		return true
	}
	return computed&perm != 0
}

// hasChannelPermission checks if a user has a specific permission in the guild
// that owns this channel. For DM/group channels (no guild), it checks that the
// user is a participant — DM participants implicitly have all permissions.
func (h *Handler) hasChannelPermission(ctx context.Context, channelID, userID string, perm uint64) bool {
	var guildID *string
	var channelType string
	h.Pool.QueryRow(ctx,
		`SELECT guild_id, channel_type FROM channels WHERE id = $1`, channelID,
	).Scan(&guildID, &channelType)

	if guildID == nil {
		// DM or group channel — check if user is a participant.
		if channelType == "dm" || channelType == "group" {
			var isRecipient bool
			h.Pool.QueryRow(ctx,
				`SELECT EXISTS(SELECT 1 FROM channel_recipients WHERE channel_id = $1 AND user_id = $2)`,
				channelID, userID,
			).Scan(&isRecipient)
			return isRecipient
		}
		return false
	}

	// Owner has all permissions.
	var ownerID string
	if err := h.Pool.QueryRow(ctx, `SELECT owner_id FROM guilds WHERE id = $1`, *guildID).Scan(&ownerID); err != nil {
		return false
	}
	if userID == ownerID {
		return true
	}

	// Admin flag.
	var userFlags int
	h.Pool.QueryRow(ctx, `SELECT flags FROM users WHERE id = $1`, userID).Scan(&userFlags)
	if userFlags&models.UserFlagAdmin != 0 {
		return true
	}

	// Default permissions + role permissions.
	var defaultPerms int64
	h.Pool.QueryRow(ctx, `SELECT default_permissions FROM guilds WHERE id = $1`, *guildID).Scan(&defaultPerms)
	computed := uint64(defaultPerms)

	rows, _ := h.Pool.Query(ctx,
		`SELECT r.permissions_allow, r.permissions_deny
		 FROM roles r
		 JOIN member_roles mr ON r.id = mr.role_id
		 WHERE mr.guild_id = $1 AND mr.user_id = $2
		 ORDER BY r.position DESC`,
		*guildID, userID,
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

	if computed&permissions.Administrator != 0 {
		return true
	}
	return computed&perm != 0
}

// channelCtx holds pre-fetched channel state and computed permissions for the
// message-send hot path. This replaces 20+ sequential queries with 2.
type channelCtx struct {
	GuildID          *string
	ChannelType      string
	Locked           bool
	Archived         bool
	ReadOnly         bool
	ReadOnlyRoleIDs  []string
	Encrypted        bool
	SlowmodeSeconds  int
	OwnerID          string // guild owner, empty for DMs
	UserFlags        int
	ComputedPerms    uint64
	IsOwner          bool
	IsAdmin          bool
	IsDMRecipient    bool
	TimeoutUntil     *time.Time
}

// loadChannelCtx fetches all channel state, guild ownership, and user
// permissions in two queries, eliminating the 20+ sequential queries in the
// message-send hot path.
func (h *Handler) loadChannelCtx(ctx context.Context, channelID, userID string) (*channelCtx, error) {
	c := &channelCtx{}

	// Query 1: Channel + guild state in a single LEFT JOIN.
	err := h.Pool.QueryRow(ctx,
		`SELECT c.guild_id, c.channel_type, c.locked, c.archived, c.read_only,
		        c.read_only_role_ids, c.encrypted, COALESCE(c.slowmode_seconds, 0),
		        COALESCE(g.owner_id, ''), COALESCE(g.default_permissions, 0),
		        COALESCE(u.flags, 0), gm.timeout_until
		 FROM channels c
		 LEFT JOIN guilds g ON g.id = c.guild_id
		 LEFT JOIN users u ON u.id = $2
		 LEFT JOIN guild_members gm ON gm.guild_id = c.guild_id AND gm.user_id = $2
		 WHERE c.id = $1`,
		channelID, userID,
	).Scan(
		&c.GuildID, &c.ChannelType, &c.Locked, &c.Archived, &c.ReadOnly,
		&c.ReadOnlyRoleIDs, &c.Encrypted, &c.SlowmodeSeconds,
		&c.OwnerID, &c.ComputedPerms, &c.UserFlags, &c.TimeoutUntil,
	)
	if err != nil {
		return nil, fmt.Errorf("loading channel context: %w", err)
	}

	c.IsOwner = c.GuildID != nil && userID == c.OwnerID
	c.IsAdmin = c.UserFlags&models.UserFlagAdmin != 0

	// Short-circuit: owners and admins have all permissions.
	if c.IsOwner || c.IsAdmin {
		c.ComputedPerms = ^uint64(0) // all bits set
		return c, nil
	}

	// For DM/group channels, check recipient membership.
	if c.GuildID == nil {
		if c.ChannelType == "dm" || c.ChannelType == "group" {
			var isRecipient bool
			_ = h.Pool.QueryRow(ctx,
				`SELECT EXISTS(SELECT 1 FROM channel_recipients WHERE channel_id = $1 AND user_id = $2)`,
				channelID, userID,
			).Scan(&isRecipient)
			c.IsDMRecipient = isRecipient
			if isRecipient {
				c.ComputedPerms = ^uint64(0) // DM participants have all permissions
			}
		}
		return c, nil
	}

	// Query 2: Role permissions for guild channels.
	rows, err := h.Pool.Query(ctx,
		`SELECT r.permissions_allow, r.permissions_deny
		 FROM roles r
		 JOIN member_roles mr ON r.id = mr.role_id
		 WHERE mr.guild_id = $1 AND mr.user_id = $2
		 ORDER BY r.position DESC`,
		*c.GuildID, userID,
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var allow, deny int64
			rows.Scan(&allow, &deny)
			c.ComputedPerms |= uint64(allow)
			c.ComputedPerms &^= uint64(deny)
		}
	}

	// Administrator bit grants all permissions.
	if c.ComputedPerms&permissions.Administrator != 0 {
		c.ComputedPerms = ^uint64(0)
	}

	return c, nil
}

// hasPerm checks if the pre-computed permissions include a specific permission.
func (c *channelCtx) hasPerm(perm uint64) bool {
	return c.ComputedPerms&perm != 0
}

func mustMarshal(v interface{}) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// HandleBatchDecryptMessages accepts decrypted content for encrypted messages,
// storing the plaintext and clearing the encrypted flag.
// POST /api/v1/channels/{channelID}/decrypt-messages
func (h *Handler) HandleBatchDecryptMessages(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ManageChannels) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_CHANNELS permission")
		return
	}

	var req struct {
		Messages []struct {
			ID      string `json:"id"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	if len(req.Messages) == 0 {
		apiutil.WriteError(w, http.StatusBadRequest, "empty_messages", "No messages provided")
		return
	}
	if len(req.Messages) > 100 {
		apiutil.WriteError(w, http.StatusBadRequest, "too_many_messages", "Maximum 100 messages per request")
		return
	}

	for _, msg := range req.Messages {
		if len(msg.Content) > 4000 {
			apiutil.WriteError(w, http.StatusBadRequest, "content_too_long", "Message content must be at most 4000 characters")
			return
		}
	}

	ctx := r.Context()
	updated := 0
	for _, msg := range req.Messages {
		tag, err := h.Pool.Exec(ctx,
			`UPDATE messages SET content = $1, encrypted = false, encryption_session_id = NULL
			 WHERE id = $2 AND channel_id = $3 AND encrypted = true`,
			msg.Content, msg.ID, channelID,
		)
		if err != nil {
			h.Logger.Error("failed to decrypt message", slog.String("message_id", msg.ID), slog.Any("error", err))
			continue
		}
		updated += int(tag.RowsAffected())
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]int{"updated": updated})
}

// HandleAddGroupDMRecipient adds a user to a group DM channel.
// PUT /api/v1/channels/{channelID}/recipients/{userID}
func (h *Handler) HandleAddGroupDMRecipient(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	targetUserID := chi.URLParam(r, "userID")

	if !apiutil.RequireNonEmpty(w, "User ID", targetUserID) {
		return
	}

	// Verify the channel exists and is a group DM.
	var channelType string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT channel_type FROM channels WHERE id = $1`, channelID,
	).Scan(&channelType)
	if err != nil {
		apiutil.WriteError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
		return
	}
	if channelType != "group" {
		apiutil.WriteError(w, http.StatusBadRequest, "not_group_dm", "This channel is not a group DM")
		return
	}

	// Verify the requesting user is a current member of the group DM.
	var isMember bool
	h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM channel_recipients WHERE channel_id = $1 AND user_id = $2)`,
		channelID, userID,
	).Scan(&isMember)
	if !isMember {
		apiutil.WriteError(w, http.StatusForbidden, "not_member", "You are not a member of this group DM")
		return
	}

	// Verify the target user exists.
	var targetExists bool
	h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`, targetUserID,
	).Scan(&targetExists)
	if !targetExists {
		apiutil.WriteError(w, http.StatusNotFound, "user_not_found", "User not found")
		return
	}

	// Check if the target is already a member.
	var alreadyMember bool
	h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM channel_recipients WHERE channel_id = $1 AND user_id = $2)`,
		channelID, targetUserID,
	).Scan(&alreadyMember)
	if alreadyMember {
		apiutil.WriteError(w, http.StatusConflict, "already_member", "User is already a member of this group DM")
		return
	}

	// Check recipient count (max 10 members in a group DM).
	var recipientCount int
	h.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM channel_recipients WHERE channel_id = $1`, channelID,
	).Scan(&recipientCount)
	if recipientCount >= 10 {
		apiutil.WriteError(w, http.StatusBadRequest, "group_full", "Group DM cannot have more than 10 members")
		return
	}

	// Add the recipient.
	_, err = h.Pool.Exec(r.Context(),
		`INSERT INTO channel_recipients (channel_id, user_id, joined_at) VALUES ($1, $2, now())`,
		channelID, targetUserID,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to add recipient", err)
		return
	}

	channel, err := h.getChannel(r.Context(), channelID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get updated channel", err)
		return
	}

	h.EventBus.PublishChannelEvent(r.Context(), events.SubjectChannelUpdate, "CHANNEL_UPDATE", channelID, channel)

	apiutil.WriteJSON(w, http.StatusOK, channel)
}

// HandleRemoveGroupDMRecipient removes a user from a group DM channel.
// If removing self, acts as a leave. If removing another user, requires channel ownership.
// DELETE /api/v1/channels/{channelID}/recipients/{userID}
func (h *Handler) HandleRemoveGroupDMRecipient(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	targetUserID := chi.URLParam(r, "userID")

	if !apiutil.RequireNonEmpty(w, "User ID", targetUserID) {
		return
	}

	// Verify the channel exists and is a group DM.
	var channelType string
	var ownerID *string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT channel_type, owner_id FROM channels WHERE id = $1`, channelID,
	).Scan(&channelType, &ownerID)
	if err != nil {
		apiutil.WriteError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
		return
	}
	if channelType != "group" {
		apiutil.WriteError(w, http.StatusBadRequest, "not_group_dm", "This channel is not a group DM")
		return
	}

	// Verify the requesting user is a current member.
	var isMember bool
	h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM channel_recipients WHERE channel_id = $1 AND user_id = $2)`,
		channelID, userID,
	).Scan(&isMember)
	if !isMember {
		apiutil.WriteError(w, http.StatusForbidden, "not_member", "You are not a member of this group DM")
		return
	}

	// If removing someone else, must be the channel owner.
	if targetUserID != userID {
		if ownerID == nil || *ownerID != userID {
			apiutil.WriteError(w, http.StatusForbidden, "not_owner", "Only the group DM owner can remove other members")
			return
		}
	}

	// Verify the target is actually a member.
	var targetIsMember bool
	h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM channel_recipients WHERE channel_id = $1 AND user_id = $2)`,
		channelID, targetUserID,
	).Scan(&targetIsMember)
	if !targetIsMember {
		apiutil.WriteError(w, http.StatusNotFound, "not_member", "User is not a member of this group DM")
		return
	}

	// Remove the recipient.
	_, err = h.Pool.Exec(r.Context(),
		`DELETE FROM channel_recipients WHERE channel_id = $1 AND user_id = $2`,
		channelID, targetUserID,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to remove recipient", err)
		return
	}

	channel, err := h.getChannel(r.Context(), channelID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get updated channel", err)
		return
	}

	h.EventBus.PublishChannelEvent(r.Context(), events.SubjectChannelUpdate, "CHANNEL_UPDATE", channelID, channel)

	w.WriteHeader(http.StatusNoContent)
}

// HandleGetChannelGallery returns media attachments (images/videos) for a channel.
// GET /api/v1/channels/{channelID}/gallery
func (h *Handler) HandleGetChannelGallery(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	// Look up the guild that owns this channel and verify membership.
	var guildID *string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT guild_id FROM channels WHERE id = $1`, channelID).Scan(&guildID)
	if err != nil {
		apiutil.WriteError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
		return
	}
	if guildID != nil {
		var isMember bool
		h.Pool.QueryRow(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM guild_members WHERE guild_id = $1 AND user_id = $2)`,
			*guildID, userID).Scan(&isMember)
		if !isMember {
			apiutil.WriteError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
			return
		}
	} else {
		// For DM/group DM channels, verify the user is a participant.
		var isRecipient bool
		h.Pool.QueryRow(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM dm_recipients WHERE channel_id = $1 AND user_id = $2)`,
			channelID, userID).Scan(&isRecipient)
		if !isRecipient {
			apiutil.WriteError(w, http.StatusForbidden, "not_recipient", "You are not a member of this conversation")
			return
		}
	}

	// Build query with optional filters.
	baseSQL := `SELECT a.id, a.message_id, a.uploader_id, a.filename, a.content_type, a.size_bytes,
	            a.width, a.height, a.duration_seconds, a.s3_bucket, a.s3_key, a.blurhash,
	            a.alt_text, a.nsfw, a.description, a.created_at
	     FROM attachments a
	     JOIN messages m ON m.id = a.message_id
	     WHERE m.channel_id = $1`
	args := []interface{}{channelID}
	argIdx := 2

	// Filter by media type.
	mediaType := r.URL.Query().Get("type")
	switch mediaType {
	case "image":
		baseSQL += ` AND a.content_type LIKE 'image/%'`
	case "video":
		baseSQL += ` AND a.content_type LIKE 'video/%'`
	default:
		baseSQL += ` AND (a.content_type LIKE 'image/%' OR a.content_type LIKE 'video/%')`
	}

	// Cursor-based pagination.
	if before := r.URL.Query().Get("before"); before != "" {
		baseSQL += fmt.Sprintf(` AND a.id < $%d`, argIdx)
		args = append(args, before)
		argIdx++
	}

	_ = argIdx // suppress unused warning
	baseSQL += ` ORDER BY a.id DESC LIMIT 50`

	rows, err := h.Pool.Query(r.Context(), baseSQL, args...)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to query gallery")
		return
	}
	defer rows.Close()

	attachments := make([]models.Attachment, 0)
	for rows.Next() {
		var a models.Attachment
		if err := rows.Scan(
			&a.ID, &a.MessageID, &a.UploaderID, &a.Filename, &a.ContentType, &a.SizeBytes,
			&a.Width, &a.Height, &a.DurationSeconds, &a.S3Bucket, &a.S3Key, &a.Blurhash,
			&a.AltText, &a.NSFW, &a.Description, &a.CreatedAt,
		); err != nil {
			apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to read gallery data")
			return
		}
		attachments = append(attachments, a)
	}

	apiutil.WriteJSON(w, http.StatusOK, attachments)
}


