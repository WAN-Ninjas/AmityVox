// Package channels implements REST API handlers for channel operations including
// fetching, updating, and deleting channels, managing messages, reactions, pins,
// typing indicators, read state acknowledgment, and permission overrides.
// Mounted under /api/v1/channels.
package channels

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
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/models"
	"github.com/amityvox/amityvox/internal/permissions"
)

// Handler implements channel-related REST API endpoints.
type Handler struct {
	Pool     *pgxpool.Pool
	EventBus *events.Bus
	Logger   *slog.Logger
}

type updateChannelRequest struct {
	Name            *string `json:"name"`
	Topic           *string `json:"topic"`
	Position        *int    `json:"position"`
	NSFW            *bool   `json:"nsfw"`
	SlowmodeSeconds *int    `json:"slowmode_seconds"`
}

type createMessageRequest struct {
	Content         *string  `json:"content"`
	Nonce           *string  `json:"nonce"`
	AttachmentIDs   []string `json:"attachment_ids"`
	ReplyToIDs      []string `json:"reply_to_ids"`
	MentionUserIDs  []string `json:"mention_user_ids"`
	MentionRoleIDs  []string `json:"mention_role_ids"`
	MentionEveryone bool     `json:"mention_everyone"`
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
	channelID := chi.URLParam(r, "channelID")

	channel, err := h.getChannel(r.Context(), channelID)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get channel")
		return
	}

	writeJSON(w, http.StatusOK, channel)
}

// HandleUpdateChannel updates a channel's settings.
// PATCH /api/v1/channels/{channelID}
func (h *Handler) HandleUpdateChannel(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")

	var req updateChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	var channel models.Channel
	err := h.Pool.QueryRow(r.Context(),
		`UPDATE channels SET
			name = COALESCE($2, name),
			topic = COALESCE($3, topic),
			position = COALESCE($4, position),
			nsfw = COALESCE($5, nsfw),
			slowmode_seconds = COALESCE($6, slowmode_seconds)
		 WHERE id = $1
		 RETURNING id, guild_id, category_id, channel_type, name, topic, position,
		           slowmode_seconds, nsfw, encrypted, last_message_id, owner_id,
		           default_permissions, created_at`,
		channelID, req.Name, req.Topic, req.Position, req.NSFW, req.SlowmodeSeconds,
	).Scan(
		&channel.ID, &channel.GuildID, &channel.CategoryID, &channel.ChannelType, &channel.Name,
		&channel.Topic, &channel.Position, &channel.SlowmodeSeconds, &channel.NSFW, &channel.Encrypted,
		&channel.LastMessageID, &channel.OwnerID, &channel.DefaultPermissions, &channel.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update channel")
		return
	}

	h.EventBus.PublishJSON(r.Context(), events.SubjectChannelUpdate, "CHANNEL_UPDATE", channel)

	writeJSON(w, http.StatusOK, channel)
}

// HandleDeleteChannel deletes a channel.
// DELETE /api/v1/channels/{channelID}
func (h *Handler) HandleDeleteChannel(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")

	tag, err := h.Pool.Exec(r.Context(), `DELETE FROM channels WHERE id = $1`, channelID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete channel")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
		return
	}

	h.EventBus.PublishJSON(r.Context(), events.SubjectChannelDelete, "CHANNEL_DELETE", map[string]string{
		"id": channelID,
	})

	w.WriteHeader(http.StatusNoContent)
}

// HandleGetMessages returns paginated messages from a channel.
// GET /api/v1/channels/{channelID}/messages?before=&after=&around=&limit=
func (h *Handler) HandleGetMessages(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")

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
		                reply_to_ids, mention_user_ids, mention_role_ids, mention_everyone,
		                thread_id, masquerade_name, masquerade_avatar, masquerade_color,
		                encrypted, encryption_session_id, created_at
		         FROM messages WHERE channel_id = $1 AND id < $2
		         ORDER BY id DESC LIMIT $3`
		args = []interface{}{channelID, before, limit}
	case after != "":
		query = `SELECT id, channel_id, author_id, content, nonce, message_type, edited_at, flags,
		                reply_to_ids, mention_user_ids, mention_role_ids, mention_everyone,
		                thread_id, masquerade_name, masquerade_avatar, masquerade_color,
		                encrypted, encryption_session_id, created_at
		         FROM messages WHERE channel_id = $1 AND id > $2
		         ORDER BY id ASC LIMIT $3`
		args = []interface{}{channelID, after, limit}
	case around != "":
		halfLimit := limit / 2
		query = `(SELECT id, channel_id, author_id, content, nonce, message_type, edited_at, flags,
		                 reply_to_ids, mention_user_ids, mention_role_ids, mention_everyone,
		                 thread_id, masquerade_name, masquerade_avatar, masquerade_color,
		                 encrypted, encryption_session_id, created_at
		          FROM messages WHERE channel_id = $1 AND id <= $2
		          ORDER BY id DESC LIMIT $3)
		         UNION ALL
		         (SELECT id, channel_id, author_id, content, nonce, message_type, edited_at, flags,
		                 reply_to_ids, mention_user_ids, mention_role_ids, mention_everyone,
		                 thread_id, masquerade_name, masquerade_avatar, masquerade_color,
		                 encrypted, encryption_session_id, created_at
		          FROM messages WHERE channel_id = $1 AND id > $2
		          ORDER BY id ASC LIMIT $4)
		         ORDER BY id DESC`
		args = []interface{}{channelID, around, halfLimit, halfLimit}
	default:
		query = `SELECT id, channel_id, author_id, content, nonce, message_type, edited_at, flags,
		                reply_to_ids, mention_user_ids, mention_role_ids, mention_everyone,
		                thread_id, masquerade_name, masquerade_avatar, masquerade_color,
		                encrypted, encryption_session_id, created_at
		         FROM messages WHERE channel_id = $1
		         ORDER BY id DESC LIMIT $2`
		args = []interface{}{channelID, limit}
	}

	rows, err := h.Pool.Query(r.Context(), query, args...)
	if err != nil {
		h.Logger.Error("failed to get messages", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get messages")
		return
	}
	defer rows.Close()

	messages := make([]models.Message, 0)
	for rows.Next() {
		var m models.Message
		if err := rows.Scan(
			&m.ID, &m.ChannelID, &m.AuthorID, &m.Content, &m.Nonce, &m.MessageType,
			&m.EditedAt, &m.Flags, &m.ReplyToIDs, &m.MentionUserIDs, &m.MentionRoleIDs,
			&m.MentionEveryone, &m.ThreadID, &m.MasqueradeName, &m.MasqueradeAvatar,
			&m.MasqueradeColor, &m.Encrypted, &m.EncryptionSessionID, &m.CreatedAt,
		); err != nil {
			h.Logger.Error("failed to scan message", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read messages")
			return
		}
		messages = append(messages, m)
	}

	writeJSON(w, http.StatusOK, messages)
}

// HandleCreateMessage sends a new message in a channel.
// POST /api/v1/channels/{channelID}/messages
func (h *Handler) HandleCreateMessage(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	var req createMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	hasContent := req.Content != nil && *req.Content != ""
	hasAttachments := len(req.AttachmentIDs) > 0
	if !hasContent && !hasAttachments {
		writeError(w, http.StatusBadRequest, "empty_content", "Message content or attachments required")
		return
	}

	if hasContent && len(*req.Content) > 4000 {
		writeError(w, http.StatusBadRequest, "content_too_long", "Message content must be at most 4000 characters")
		return
	}

	// Enforce slowmode: check if the user posted too recently in this channel.
	var slowmodeSec int
	h.Pool.QueryRow(r.Context(),
		`SELECT COALESCE(slowmode_seconds, 0) FROM channels WHERE id = $1`, channelID).Scan(&slowmodeSec)
	if slowmodeSec > 0 {
		var lastSent *time.Time
		h.Pool.QueryRow(r.Context(),
			`SELECT MAX(created_at) FROM messages WHERE channel_id = $1 AND author_id = $2`,
			channelID, userID).Scan(&lastSent)
		if lastSent != nil {
			elapsed := time.Since(*lastSent)
			if elapsed < time.Duration(slowmodeSec)*time.Second {
				remaining := time.Duration(slowmodeSec)*time.Second - elapsed
				writeError(w, http.StatusTooManyRequests, "slowmode",
					fmt.Sprintf("Slowmode active. Try again in %.0f seconds", remaining.Seconds()))
				return
			}
		}
	}

	// Check if the user is timed out in this guild (communication disabled).
	var guildID *string
	h.Pool.QueryRow(r.Context(), `SELECT guild_id FROM channels WHERE id = $1`, channelID).Scan(&guildID)
	if guildID != nil {
		var timeoutUntil *time.Time
		h.Pool.QueryRow(r.Context(),
			`SELECT timeout_until FROM guild_members WHERE guild_id = $1 AND user_id = $2`,
			*guildID, userID).Scan(&timeoutUntil)
		if timeoutUntil != nil && timeoutUntil.After(time.Now()) {
			writeError(w, http.StatusForbidden, "timed_out", "You are timed out and cannot send messages")
			return
		}
	}

	msgID := models.NewULID().String()
	msgType := models.MessageTypeDefault
	if len(req.ReplyToIDs) > 0 {
		msgType = models.MessageTypeReply
	}

	var msg models.Message
	err := h.Pool.QueryRow(r.Context(),
		`INSERT INTO messages (id, channel_id, author_id, content, nonce, message_type,
		                       reply_to_ids, mention_user_ids, mention_role_ids, mention_everyone, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now())
		 RETURNING id, channel_id, author_id, content, nonce, message_type, edited_at, flags,
		           reply_to_ids, mention_user_ids, mention_role_ids, mention_everyone,
		           thread_id, masquerade_name, masquerade_avatar, masquerade_color,
		           encrypted, encryption_session_id, created_at`,
		msgID, channelID, userID, req.Content, req.Nonce, msgType,
		req.ReplyToIDs, req.MentionUserIDs, req.MentionRoleIDs, req.MentionEveryone,
	).Scan(
		&msg.ID, &msg.ChannelID, &msg.AuthorID, &msg.Content, &msg.Nonce, &msg.MessageType,
		&msg.EditedAt, &msg.Flags, &msg.ReplyToIDs, &msg.MentionUserIDs, &msg.MentionRoleIDs,
		&msg.MentionEveryone, &msg.ThreadID, &msg.MasqueradeName, &msg.MasqueradeAvatar,
		&msg.MasqueradeColor, &msg.Encrypted, &msg.EncryptionSessionID, &msg.CreatedAt,
	)
	if err != nil {
		h.Logger.Error("failed to create message", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to send message")
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

	h.EventBus.PublishJSON(r.Context(), events.SubjectMessageCreate, "MESSAGE_CREATE", msg)

	writeJSON(w, http.StatusCreated, msg)
}

// HandleGetMessage returns a single message by ID.
// GET /api/v1/channels/{channelID}/messages/{messageID}
func (h *Handler) HandleGetMessage(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")

	msg, err := h.getMessage(r.Context(), channelID, messageID)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "message_not_found", "Message not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get message")
		return
	}

	msg.Attachments = h.loadAttachments(r.Context(), messageID)
	msg.Embeds = h.loadEmbeds(r.Context(), messageID)

	writeJSON(w, http.StatusOK, msg)
}

// HandleUpdateMessage edits a message's content. Only the author can edit.
// PATCH /api/v1/channels/{channelID}/messages/{messageID}
func (h *Handler) HandleUpdateMessage(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")

	var req updateMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Content == nil {
		writeError(w, http.StatusBadRequest, "missing_content", "Content is required")
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
		writeError(w, http.StatusNotFound, "message_not_found", "Message not found")
		return
	}
	if authorID != userID {
		writeError(w, http.StatusForbidden, "not_author", "You can only edit your own messages")
		return
	}

	// Save previous content to edit history.
	if currentContent != nil {
		editID := models.NewULID().String()
		h.Pool.Exec(r.Context(),
			`INSERT INTO message_edits (id, message_id, content, edited_at) VALUES ($1, $2, $3, now())`,
			editID, messageID, *currentContent)
	}

	var msg models.Message
	err = h.Pool.QueryRow(r.Context(),
		`UPDATE messages SET content = $3, edited_at = now()
		 WHERE id = $1 AND channel_id = $2
		 RETURNING id, channel_id, author_id, content, nonce, message_type, edited_at, flags,
		           reply_to_ids, mention_user_ids, mention_role_ids, mention_everyone,
		           thread_id, masquerade_name, masquerade_avatar, masquerade_color,
		           encrypted, encryption_session_id, created_at`,
		messageID, channelID, req.Content,
	).Scan(
		&msg.ID, &msg.ChannelID, &msg.AuthorID, &msg.Content, &msg.Nonce, &msg.MessageType,
		&msg.EditedAt, &msg.Flags, &msg.ReplyToIDs, &msg.MentionUserIDs, &msg.MentionRoleIDs,
		&msg.MentionEveryone, &msg.ThreadID, &msg.MasqueradeName, &msg.MasqueradeAvatar,
		&msg.MasqueradeColor, &msg.Encrypted, &msg.EncryptionSessionID, &msg.CreatedAt,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update message")
		return
	}

	h.EventBus.PublishJSON(r.Context(), events.SubjectMessageUpdate, "MESSAGE_UPDATE", msg)

	writeJSON(w, http.StatusOK, msg)
}

// HandleGetMessageEdits returns the edit history for a message.
// GET /api/v1/channels/{channelID}/messages/{messageID}/edits
func (h *Handler) HandleGetMessageEdits(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")

	// Verify the message exists in this channel.
	var exists bool
	h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM messages WHERE id = $1 AND channel_id = $2)`,
		messageID, channelID).Scan(&exists)
	if !exists {
		writeError(w, http.StatusNotFound, "message_not_found", "Message not found")
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
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get edit history")
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
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read edit history")
			return
		}
		edits = append(edits, e)
	}

	writeJSON(w, http.StatusOK, edits)
}

// HandleDeleteMessage deletes a message. Author or users with MANAGE_MESSAGES can delete.
// DELETE /api/v1/channels/{channelID}/messages/{messageID}
func (h *Handler) HandleDeleteMessage(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")

	// Check authorship (permission-based deletion requires guild context, simplified here).
	var authorID string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT author_id FROM messages WHERE id = $1 AND channel_id = $2`,
		messageID, channelID,
	).Scan(&authorID)
	if err != nil {
		writeError(w, http.StatusNotFound, "message_not_found", "Message not found")
		return
	}

	if authorID != userID {
		// Non-authors need MANAGE_MESSAGES permission in the guild.
		if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ManageMessages) {
			writeError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_MESSAGES permission to delete others' messages")
			return
		}
	}

	_, err = h.Pool.Exec(r.Context(),
		`DELETE FROM messages WHERE id = $1 AND channel_id = $2`, messageID, channelID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete message")
		return
	}

	h.EventBus.PublishJSON(r.Context(), events.SubjectMessageDelete, "MESSAGE_DELETE", map[string]string{
		"id": messageID, "channel_id": channelID,
	})

	w.WriteHeader(http.StatusNoContent)
}

// HandleBulkDeleteMessages deletes multiple messages in a channel at once.
// POST /api/v1/channels/{channelID}/messages/bulk-delete
func (h *Handler) HandleBulkDeleteMessages(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")

	var req struct {
		MessageIDs []string `json:"message_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if len(req.MessageIDs) == 0 {
		writeError(w, http.StatusBadRequest, "empty_ids", "At least one message ID is required")
		return
	}
	if len(req.MessageIDs) > 100 {
		writeError(w, http.StatusBadRequest, "too_many_ids", "Cannot bulk delete more than 100 messages at once")
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM messages WHERE channel_id = $1 AND id = ANY($2)`,
		channelID, req.MessageIDs,
	)
	if err != nil {
		h.Logger.Error("failed to bulk delete messages", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete messages")
		return
	}

	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "no_messages_found", "No matching messages found in this channel")
		return
	}

	h.EventBus.PublishJSON(r.Context(), events.SubjectMessageDeleteBulk, "MESSAGE_DELETE_BULK", map[string]interface{}{
		"channel_id":  channelID,
		"message_ids": req.MessageIDs,
	})

	w.WriteHeader(http.StatusNoContent)
}

// HandleGetReactions returns aggregated reaction counts and the reacting users for a message.
// GET /api/v1/channels/{channelID}/messages/{messageID}/reactions
func (h *Handler) HandleGetReactions(w http.ResponseWriter, r *http.Request) {
	messageID := chi.URLParam(r, "messageID")

	rows, err := h.Pool.Query(r.Context(),
		`SELECT emoji, COUNT(*) as count, array_agg(user_id ORDER BY created_at) as users
		 FROM reactions WHERE message_id = $1
		 GROUP BY emoji
		 ORDER BY count DESC`,
		messageID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get reactions")
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
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read reactions")
			return
		}
		reactions = append(reactions, rg)
	}

	writeJSON(w, http.StatusOK, reactions)
}

// HandleAddReaction adds an emoji reaction to a message.
// PUT /api/v1/channels/{channelID}/messages/{messageID}/reactions/{emoji}
func (h *Handler) HandleAddReaction(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")
	emoji := chi.URLParam(r, "emoji")

	// Verify message exists in channel.
	var exists bool
	h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM messages WHERE id = $1 AND channel_id = $2)`,
		messageID, channelID,
	).Scan(&exists)
	if !exists {
		writeError(w, http.StatusNotFound, "message_not_found", "Message not found")
		return
	}

	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO reactions (message_id, user_id, emoji, created_at)
		 VALUES ($1, $2, $3, now())
		 ON CONFLICT (message_id, user_id, emoji) DO NOTHING`,
		messageID, userID, emoji,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to add reaction")
		return
	}

	h.EventBus.PublishJSON(r.Context(), events.SubjectMessageReactionAdd, "MESSAGE_REACTION_ADD", map[string]string{
		"message_id": messageID, "channel_id": channelID, "user_id": userID, "emoji": emoji,
	})

	w.WriteHeader(http.StatusNoContent)
}

// HandleRemoveReaction removes an emoji reaction from a message.
// DELETE /api/v1/channels/{channelID}/messages/{messageID}/reactions/{emoji}
func (h *Handler) HandleRemoveReaction(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	messageID := chi.URLParam(r, "messageID")
	emoji := chi.URLParam(r, "emoji")

	_, err := h.Pool.Exec(r.Context(),
		`DELETE FROM reactions WHERE message_id = $1 AND user_id = $2 AND emoji = $3`,
		messageID, userID, emoji,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to remove reaction")
		return
	}

	h.EventBus.PublishJSON(r.Context(), events.SubjectMessageReactionDel, "MESSAGE_REACTION_REMOVE", map[string]string{
		"message_id": messageID, "user_id": userID, "emoji": emoji,
	})

	w.WriteHeader(http.StatusNoContent)
}

// HandleGetPins returns pinned messages in a channel.
// GET /api/v1/channels/{channelID}/pins
func (h *Handler) HandleGetPins(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")

	rows, err := h.Pool.Query(r.Context(),
		`SELECT m.id, m.channel_id, m.author_id, m.content, m.nonce, m.message_type,
		        m.edited_at, m.flags, m.reply_to_ids, m.mention_user_ids, m.mention_role_ids,
		        m.mention_everyone, m.thread_id, m.masquerade_name, m.masquerade_avatar,
		        m.masquerade_color, m.encrypted, m.encryption_session_id, m.created_at
		 FROM messages m
		 JOIN pins p ON m.id = p.message_id
		 WHERE p.channel_id = $1
		 ORDER BY p.pinned_at DESC`,
		channelID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get pins")
		return
	}
	defer rows.Close()

	messages := make([]models.Message, 0)
	for rows.Next() {
		var m models.Message
		if err := rows.Scan(
			&m.ID, &m.ChannelID, &m.AuthorID, &m.Content, &m.Nonce, &m.MessageType,
			&m.EditedAt, &m.Flags, &m.ReplyToIDs, &m.MentionUserIDs, &m.MentionRoleIDs,
			&m.MentionEveryone, &m.ThreadID, &m.MasqueradeName, &m.MasqueradeAvatar,
			&m.MasqueradeColor, &m.Encrypted, &m.EncryptionSessionID, &m.CreatedAt,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read pins")
			return
		}
		messages = append(messages, m)
	}

	writeJSON(w, http.StatusOK, messages)
}

// HandlePinMessage pins a message in a channel.
// PUT /api/v1/channels/{channelID}/pins/{messageID}
func (h *Handler) HandlePinMessage(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")

	// Verify message exists.
	var exists bool
	h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM messages WHERE id = $1 AND channel_id = $2)`,
		messageID, channelID,
	).Scan(&exists)
	if !exists {
		writeError(w, http.StatusNotFound, "message_not_found", "Message not found")
		return
	}

	// Enforce pin limit (50 per channel).
	var pinCount int
	h.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM pins WHERE channel_id = $1`, channelID).Scan(&pinCount)
	if pinCount >= 50 {
		writeError(w, http.StatusBadRequest, "pin_limit", "Channel has reached the maximum of 50 pinned messages")
		return
	}

	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO pins (channel_id, message_id, pinned_by, pinned_at)
		 VALUES ($1, $2, $3, now())
		 ON CONFLICT (channel_id, message_id) DO NOTHING`,
		channelID, messageID, userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to pin message")
		return
	}

	h.EventBus.PublishJSON(r.Context(), events.SubjectChannelPinsUpdate, "CHANNEL_PINS_UPDATE", map[string]string{
		"channel_id": channelID,
	})

	w.WriteHeader(http.StatusNoContent)
}

// HandleUnpinMessage unpins a message from a channel.
// DELETE /api/v1/channels/{channelID}/pins/{messageID}
func (h *Handler) HandleUnpinMessage(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM pins WHERE channel_id = $1 AND message_id = $2`, channelID, messageID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to unpin message")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "pin_not_found", "Message is not pinned")
		return
	}

	h.EventBus.PublishJSON(r.Context(), events.SubjectChannelPinsUpdate, "CHANNEL_PINS_UPDATE", map[string]string{
		"channel_id": channelID,
	})

	w.WriteHeader(http.StatusNoContent)
}

// HandleTriggerTyping sends a typing indicator event for the channel.
// POST /api/v1/channels/{channelID}/typing
func (h *Handler) HandleTriggerTyping(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	h.EventBus.PublishJSON(r.Context(), events.SubjectTypingStart, "TYPING_START", map[string]string{
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

	h.EventBus.PublishJSON(r.Context(), events.SubjectChannelAck, "CHANNEL_ACK", map[string]string{
		"channel_id": channelID, "user_id": userID,
	})

	w.WriteHeader(http.StatusNoContent)
}

// HandleSetChannelPermission sets a permission override on a channel.
// PUT /api/v1/channels/{channelID}/permissions/{overrideID}
func (h *Handler) HandleSetChannelPermission(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")
	overrideID := chi.URLParam(r, "overrideID")

	var req permissionOverrideRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.TargetType != "role" && req.TargetType != "user" {
		writeError(w, http.StatusBadRequest, "invalid_target_type", "Target type must be 'role' or 'user'")
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
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to set permission override")
		return
	}

	h.EventBus.PublishJSON(r.Context(), events.SubjectChannelUpdate, "CHANNEL_UPDATE", map[string]string{
		"channel_id": channelID,
	})

	writeJSON(w, http.StatusOK, models.ChannelPermissionOverride{
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
	channelID := chi.URLParam(r, "channelID")
	overrideID := chi.URLParam(r, "overrideID")

	_, err := h.Pool.Exec(r.Context(),
		`DELETE FROM channel_permission_overrides WHERE channel_id = $1 AND target_id = $2`,
		channelID, overrideID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete permission override")
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

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if req.Name == "" || len(req.Name) > 100 {
		writeError(w, http.StatusBadRequest, "invalid_name", "Thread name must be 1-100 characters")
		return
	}

	// Verify the parent message exists.
	var exists bool
	h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM messages WHERE id = $1 AND channel_id = $2)`,
		messageID, channelID).Scan(&exists)
	if !exists {
		writeError(w, http.StatusNotFound, "message_not_found", "Message not found")
		return
	}

	// Check the parent channel is in a guild.
	var guildID *string
	h.Pool.QueryRow(r.Context(),
		`SELECT guild_id FROM channels WHERE id = $1`, channelID).Scan(&guildID)
	if guildID == nil {
		writeError(w, http.StatusBadRequest, "invalid_channel", "Threads can only be created in guild channels")
		return
	}

	threadID := models.NewULID().String()

	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create thread")
		return
	}
	defer tx.Rollback(r.Context())

	// Create the thread as a new channel linked to the guild.
	var thread models.Channel
	err = tx.QueryRow(r.Context(),
		`INSERT INTO channels (id, guild_id, category_id, channel_type, name, owner_id, position, created_at)
		 VALUES ($1, $2, NULL, 'text', $3, $4, 0, now())
		 RETURNING id, guild_id, category_id, channel_type, name, topic, position,
		           slowmode_seconds, nsfw, encrypted, last_message_id, owner_id,
		           default_permissions, created_at`,
		threadID, guildID, req.Name, userID,
	).Scan(
		&thread.ID, &thread.GuildID, &thread.CategoryID, &thread.ChannelType, &thread.Name,
		&thread.Topic, &thread.Position, &thread.SlowmodeSeconds, &thread.NSFW, &thread.Encrypted,
		&thread.LastMessageID, &thread.OwnerID, &thread.DefaultPermissions, &thread.CreatedAt,
	)
	if err != nil {
		h.Logger.Error("failed to create thread channel", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create thread")
		return
	}

	// Link the parent message to the thread.
	tx.Exec(r.Context(),
		`UPDATE messages SET thread_id = $1 WHERE id = $2 AND channel_id = $3`,
		threadID, messageID, channelID)

	// Create a system message about thread creation.
	sysMsgID := models.NewULID().String()
	tx.Exec(r.Context(),
		`INSERT INTO messages (id, channel_id, author_id, content, message_type, created_at)
		 VALUES ($1, $2, $3, $4, $5, now())`,
		sysMsgID, channelID, userID, req.Name, models.MessageTypeThreadCreated)

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create thread")
		return
	}

	h.EventBus.PublishJSON(r.Context(), events.SubjectChannelCreate, "THREAD_CREATE", thread)

	writeJSON(w, http.StatusCreated, thread)
}

// HandleGetThreads lists active threads in a channel.
// GET /api/v1/channels/{channelID}/threads
func (h *Handler) HandleGetThreads(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")

	// Get the guild_id of this channel so we can find threads.
	var guildID *string
	h.Pool.QueryRow(r.Context(),
		`SELECT guild_id FROM channels WHERE id = $1`, channelID).Scan(&guildID)
	if guildID == nil {
		writeError(w, http.StatusNotFound, "channel_not_found", "Channel not found or is not a guild channel")
		return
	}

	// Find all threads originating from messages in this channel.
	rows, err := h.Pool.Query(r.Context(),
		`SELECT DISTINCT c.id, c.guild_id, c.category_id, c.channel_type, c.name, c.topic,
		        c.position, c.slowmode_seconds, c.nsfw, c.encrypted, c.last_message_id,
		        c.owner_id, c.default_permissions, c.created_at
		 FROM channels c
		 JOIN messages m ON m.thread_id = c.id
		 WHERE m.channel_id = $1
		 ORDER BY c.created_at DESC
		 LIMIT 50`,
		channelID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get threads")
		return
	}
	defer rows.Close()

	threads := make([]models.Channel, 0)
	for rows.Next() {
		var c models.Channel
		if err := rows.Scan(
			&c.ID, &c.GuildID, &c.CategoryID, &c.ChannelType, &c.Name, &c.Topic,
			&c.Position, &c.SlowmodeSeconds, &c.NSFW, &c.Encrypted, &c.LastMessageID,
			&c.OwnerID, &c.DefaultPermissions, &c.CreatedAt,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read threads")
			return
		}
		threads = append(threads, c)
	}

	writeJSON(w, http.StatusOK, threads)
}

// --- Internal helpers ---

// HandleGetChannelWebhooks lists all webhooks for a channel.
// GET /api/v1/channels/{channelID}/webhooks
func (h *Handler) HandleGetChannelWebhooks(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, guild_id, channel_id, creator_id, name, avatar_id, token,
		        webhook_type, outgoing_url, created_at
		 FROM webhooks WHERE channel_id = $1
		 ORDER BY created_at DESC`,
		channelID,
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

func (h *Handler) getChannel(ctx context.Context, channelID string) (*models.Channel, error) {
	var c models.Channel
	err := h.Pool.QueryRow(ctx,
		`SELECT id, guild_id, category_id, channel_type, name, topic, position,
		        slowmode_seconds, nsfw, encrypted, last_message_id, owner_id,
		        default_permissions, created_at
		 FROM channels WHERE id = $1`,
		channelID,
	).Scan(
		&c.ID, &c.GuildID, &c.CategoryID, &c.ChannelType, &c.Name, &c.Topic,
		&c.Position, &c.SlowmodeSeconds, &c.NSFW, &c.Encrypted, &c.LastMessageID,
		&c.OwnerID, &c.DefaultPermissions, &c.CreatedAt,
	)
	return &c, err
}

func (h *Handler) getMessage(ctx context.Context, channelID, messageID string) (*models.Message, error) {
	var m models.Message
	err := h.Pool.QueryRow(ctx,
		`SELECT id, channel_id, author_id, content, nonce, message_type, edited_at, flags,
		        reply_to_ids, mention_user_ids, mention_role_ids, mention_everyone,
		        thread_id, masquerade_name, masquerade_avatar, masquerade_color,
		        encrypted, encryption_session_id, created_at
		 FROM messages WHERE id = $1 AND channel_id = $2`,
		messageID, channelID,
	).Scan(
		&m.ID, &m.ChannelID, &m.AuthorID, &m.Content, &m.Nonce, &m.MessageType,
		&m.EditedAt, &m.Flags, &m.ReplyToIDs, &m.MentionUserIDs, &m.MentionRoleIDs,
		&m.MentionEveryone, &m.ThreadID, &m.MasqueradeName, &m.MasqueradeAvatar,
		&m.MasqueradeColor, &m.Encrypted, &m.EncryptionSessionID, &m.CreatedAt,
	)
	return &m, err
}

func (h *Handler) loadAttachments(ctx context.Context, messageID string) []models.Attachment {
	rows, err := h.Pool.Query(ctx,
		`SELECT id, message_id, uploader_id, filename, content_type, size_bytes,
		        width, height, duration_seconds, s3_bucket, s3_key, blurhash, created_at
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
			&a.Width, &a.Height, &a.DurationSeconds, &a.S3Bucket, &a.S3Key, &a.Blurhash, &a.CreatedAt,
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
		writeError(w, http.StatusBadRequest, "invalid_body", "target_channel_id is required")
		return
	}

	if req.TargetChannelID == sourceChannelID {
		writeError(w, http.StatusBadRequest, "same_channel", "Cannot crosspost to the same channel")
		return
	}

	// Check permission in target channel's guild.
	if !h.hasChannelPermission(r.Context(), req.TargetChannelID, userID, permissions.SendMessages) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need SEND_MESSAGES permission in the target channel")
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
		writeError(w, http.StatusNotFound, "message_not_found", "Source message not found")
		return
	}

	// Create the forwarded message in the target channel.
	newMsgID := models.NewULID().String()
	var msg models.Message
	err = h.Pool.QueryRow(r.Context(),
		`INSERT INTO messages (id, channel_id, author_id, content, message_type, flags, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, now())
		 RETURNING id, channel_id, author_id, content, nonce, message_type, edited_at, flags,
		           reply_to_ids, mention_user_ids, mention_role_ids, mention_everyone,
		           thread_id, masquerade_name, masquerade_avatar, masquerade_color,
		           encrypted, encryption_session_id, created_at`,
		newMsgID, req.TargetChannelID, userID, content, models.MessageTypeDefault, models.MessageFlagCrosspost,
	).Scan(
		&msg.ID, &msg.ChannelID, &msg.AuthorID, &msg.Content, &msg.Nonce, &msg.MessageType,
		&msg.EditedAt, &msg.Flags, &msg.ReplyToIDs, &msg.MentionUserIDs, &msg.MentionRoleIDs,
		&msg.MentionEveryone, &msg.ThreadID, &msg.MasqueradeName, &msg.MasqueradeAvatar,
		&msg.MasqueradeColor, &msg.Encrypted, &msg.EncryptionSessionID, &msg.CreatedAt,
	)
	if err != nil {
		h.Logger.Error("failed to crosspost message", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to crosspost message")
		return
	}

	h.Pool.Exec(r.Context(),
		`UPDATE channels SET last_message_id = $1 WHERE id = $2`, newMsgID, req.TargetChannelID)

	h.EventBus.PublishJSON(r.Context(), events.SubjectMessageCreate, "MESSAGE_CREATE", msg)

	writeJSON(w, http.StatusCreated, msg)
}

// hasChannelPermission checks if a user has a specific permission in the guild
// that owns this channel. Returns false for DM channels.
func (h *Handler) hasChannelPermission(ctx context.Context, channelID, userID string, perm uint64) bool {
	var guildID *string
	h.Pool.QueryRow(ctx, `SELECT guild_id FROM channels WHERE id = $1`, channelID).Scan(&guildID)
	if guildID == nil {
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
