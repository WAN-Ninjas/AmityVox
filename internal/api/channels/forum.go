package channels

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/oklog/ulid/v2"

	"github.com/amityvox/amityvox/internal/api/apiutil"
	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/models"
	"github.com/amityvox/amityvox/internal/permissions"
)

// =============================================================================
// Forum Tag CRUD
// =============================================================================

// HandleGetForumTags lists tags for a forum channel.
// GET /api/v1/channels/{channelID}/tags
func (h *Handler) HandleGetForumTags(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ViewChannel) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need VIEW_CHANNEL permission")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, channel_id, name, emoji, color, position, created_at
		 FROM forum_tags WHERE channel_id = $1 ORDER BY position`, channelID)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list tags")
		return
	}
	defer rows.Close()

	var tags []models.ForumTag
	for rows.Next() {
		var t models.ForumTag
		if err := rows.Scan(&t.ID, &t.ChannelID, &t.Name, &t.Emoji, &t.Color, &t.Position, &t.CreatedAt); err != nil {
			continue
		}
		tags = append(tags, t)
	}
	if tags == nil {
		tags = []models.ForumTag{}
	}

	apiutil.WriteJSON(w, http.StatusOK, tags)
}

// HandleCreateForumTag creates a new tag for a forum channel.
// POST /api/v1/channels/{channelID}/tags
func (h *Handler) HandleCreateForumTag(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ManageChannels) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_CHANNELS permission")
		return
	}

	// Verify it's a forum channel.
	var channelType string
	if err := h.Pool.QueryRow(r.Context(), `SELECT channel_type FROM channels WHERE id = $1`, channelID).Scan(&channelType); err != nil {
		apiutil.WriteError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
		return
	}
	if channelType != models.ChannelTypeForum {
		apiutil.WriteError(w, http.StatusBadRequest, "not_forum", "Tags can only be created on forum channels")
		return
	}

	var req struct {
		Name  string  `json:"name"`
		Emoji *string `json:"emoji"`
		Color *string `json:"color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if req.Name == "" {
		apiutil.WriteError(w, http.StatusBadRequest, "missing_name", "Tag name is required")
		return
	}

	// Get next position.
	var maxPos int
	h.Pool.QueryRow(r.Context(),
		`SELECT COALESCE(MAX(position), -1) FROM forum_tags WHERE channel_id = $1`, channelID).Scan(&maxPos)

	id := ulid.Make().String()
	var tag models.ForumTag
	err := h.Pool.QueryRow(r.Context(),
		`INSERT INTO forum_tags (id, channel_id, name, emoji, color, position, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, now())
		 RETURNING id, channel_id, name, emoji, color, position, created_at`,
		id, channelID, req.Name, req.Emoji, req.Color, maxPos+1,
	).Scan(&tag.ID, &tag.ChannelID, &tag.Name, &tag.Emoji, &tag.Color, &tag.Position, &tag.CreatedAt)
	if err != nil {
		h.Logger.Error("failed to create forum tag", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create tag")
		return
	}

	apiutil.WriteJSON(w, http.StatusCreated, tag)
}

// HandleUpdateForumTag updates a forum tag.
// PATCH /api/v1/channels/{channelID}/tags/{tagID}
func (h *Handler) HandleUpdateForumTag(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	tagID := chi.URLParam(r, "tagID")

	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ManageChannels) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_CHANNELS permission")
		return
	}

	var req struct {
		Name  *string `json:"name"`
		Emoji *string `json:"emoji"`
		Color *string `json:"color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	setClauses := []string{"channel_id = channel_id"} // no-op base
	args := []interface{}{}
	argIdx := 1

	if req.Name != nil {
		setClauses = append(setClauses, "name = $"+strconv.Itoa(argIdx))
		args = append(args, *req.Name)
		argIdx++
	}
	if req.Emoji != nil {
		setClauses = append(setClauses, "emoji = $"+strconv.Itoa(argIdx))
		args = append(args, *req.Emoji)
		argIdx++
	}
	if req.Color != nil {
		setClauses = append(setClauses, "color = $"+strconv.Itoa(argIdx))
		args = append(args, *req.Color)
		argIdx++
	}

	args = append(args, tagID, channelID)
	query := "UPDATE forum_tags SET " + joinStrings(setClauses, ", ") +
		" WHERE id = $" + strconv.Itoa(argIdx) +
		" AND channel_id = $" + strconv.Itoa(argIdx+1) +
		" RETURNING id, channel_id, name, emoji, color, position, created_at"

	var tag models.ForumTag
	err := h.Pool.QueryRow(r.Context(), query, args...).Scan(
		&tag.ID, &tag.ChannelID, &tag.Name, &tag.Emoji, &tag.Color, &tag.Position, &tag.CreatedAt)
	if err == pgx.ErrNoRows {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Tag not found")
		return
	}
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update tag")
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, tag)
}

// HandleDeleteForumTag deletes a forum tag.
// DELETE /api/v1/channels/{channelID}/tags/{tagID}
func (h *Handler) HandleDeleteForumTag(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	tagID := chi.URLParam(r, "tagID")

	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ManageChannels) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_CHANNELS permission")
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM forum_tags WHERE id = $1 AND channel_id = $2`, tagID, channelID)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete tag")
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Tag not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// =============================================================================
// Forum Post Operations
// =============================================================================

// HandleGetForumPosts lists posts in a forum channel with sorting, filtering, and pagination.
// GET /api/v1/channels/{channelID}/posts
func (h *Handler) HandleGetForumPosts(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ViewChannel) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need VIEW_CHANNEL permission")
		return
	}

	// Verify it's a forum channel.
	var channelType string
	if err := h.Pool.QueryRow(r.Context(), `SELECT channel_type FROM channels WHERE id = $1`, channelID).Scan(&channelType); err != nil {
		apiutil.WriteError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
		return
	}
	if channelType != models.ChannelTypeForum {
		apiutil.WriteError(w, http.StatusBadRequest, "not_forum", "This is not a forum channel")
		return
	}

	sortBy := r.URL.Query().Get("sort")
	if sortBy == "" {
		sortBy = "latest_activity"
	}
	tagFilter := r.URL.Query().Get("tag")
	before := r.URL.Query().Get("before")
	limitStr := r.URL.Query().Get("limit")
	limit := 25
	if limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil && v > 0 && v <= 100 {
			limit = v
		}
	}

	// Build query for thread channels whose parent is this forum.
	query := `SELECT c.id, c.name, c.owner_id, c.pinned, c.locked, c.reply_count,
	                 c.last_activity_at, c.created_at
	          FROM channels c`

	args := []interface{}{channelID}
	argIdx := 2
	whereClause := ` WHERE c.parent_channel_id = $1 AND c.channel_type = 'text'`

	if tagFilter != "" {
		query += ` JOIN forum_post_tags fpt ON fpt.post_id = c.id`
		whereClause += ` AND fpt.tag_id = $` + strconv.Itoa(argIdx)
		args = append(args, tagFilter)
		argIdx++
	}

	if before != "" {
		whereClause += ` AND c.created_at < $` + strconv.Itoa(argIdx)
		t, err := time.Parse(time.RFC3339, before)
		if err == nil {
			args = append(args, t)
		} else {
			args = append(args, before)
		}
		argIdx++
	}

	query += whereClause

	// Pinned posts first, then sort by user preference.
	switch sortBy {
	case "creation_date":
		query += ` ORDER BY c.pinned DESC, c.created_at DESC`
	default: // latest_activity
		query += ` ORDER BY c.pinned DESC, COALESCE(c.last_activity_at, c.created_at) DESC`
	}

	query += ` LIMIT $` + strconv.Itoa(argIdx)
	args = append(args, limit)

	rows, err := h.Pool.Query(r.Context(), query, args...)
	if err != nil {
		h.Logger.Error("failed to query forum posts", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to load forum posts")
		return
	}
	defer rows.Close()

	var posts []models.ForumPost
	var ownerIDs []string
	var postIDs []string
	for rows.Next() {
		var p models.ForumPost
		if err := rows.Scan(&p.ID, &p.Name, &p.OwnerID, &p.Pinned, &p.Locked,
			&p.ReplyCount, &p.LastActivityAt, &p.CreatedAt); err != nil {
			continue
		}
		posts = append(posts, p)
		postIDs = append(postIDs, p.ID)
		if p.OwnerID != nil {
			ownerIDs = append(ownerIDs, *p.OwnerID)
		}
	}

	if posts == nil {
		posts = []models.ForumPost{}
	}

	// Batch-load authors.
	if len(ownerIDs) > 0 {
		userMap := make(map[string]*models.User)
		uRows, err := h.Pool.Query(r.Context(),
			`SELECT id, instance_id, username, display_name, avatar_id, flags, created_at
			 FROM users WHERE id = ANY($1)`, ownerIDs)
		if err == nil {
			defer uRows.Close()
			for uRows.Next() {
				var u models.User
				if err := uRows.Scan(&u.ID, &u.InstanceID, &u.Username, &u.DisplayName,
					&u.AvatarID, &u.Flags, &u.CreatedAt); err == nil {
					userMap[u.ID] = &u
				}
			}
		}
		for i := range posts {
			if posts[i].OwnerID != nil {
				posts[i].Author = userMap[*posts[i].OwnerID]
			}
		}
	}

	// Batch-load tags for each post.
	if len(postIDs) > 0 {
		tagMap := make(map[string][]models.ForumTag)
		tRows, err := h.Pool.Query(r.Context(),
			`SELECT fpt.post_id, ft.id, ft.channel_id, ft.name, ft.emoji, ft.color, ft.position, ft.created_at
			 FROM forum_post_tags fpt
			 JOIN forum_tags ft ON ft.id = fpt.tag_id
			 WHERE fpt.post_id = ANY($1)
			 ORDER BY ft.position`, postIDs)
		if err == nil {
			defer tRows.Close()
			for tRows.Next() {
				var postID string
				var t models.ForumTag
				if err := tRows.Scan(&postID, &t.ID, &t.ChannelID, &t.Name, &t.Emoji,
					&t.Color, &t.Position, &t.CreatedAt); err == nil {
					tagMap[postID] = append(tagMap[postID], t)
				}
			}
		}
		for i := range posts {
			if tags, ok := tagMap[posts[i].ID]; ok {
				posts[i].Tags = tags
			} else {
				posts[i].Tags = []models.ForumTag{}
			}
		}
	}

	// Batch-load content preview (first message in each thread, first 200 chars).
	if len(postIDs) > 0 {
		previewRows, err := h.Pool.Query(r.Context(),
			`SELECT DISTINCT ON (channel_id) channel_id, LEFT(content, 200)
			 FROM messages WHERE channel_id = ANY($1)
			 ORDER BY channel_id, created_at ASC`, postIDs)
		if err == nil {
			defer previewRows.Close()
			previewMap := make(map[string]string)
			for previewRows.Next() {
				var cid string
				var preview *string
				if err := previewRows.Scan(&cid, &preview); err == nil && preview != nil {
					previewMap[cid] = *preview
				}
			}
			for i := range posts {
				if p, ok := previewMap[posts[i].ID]; ok {
					posts[i].ContentPreview = &p
				}
			}
		}
	}

	apiutil.WriteJSON(w, http.StatusOK, posts)
}

// HandleCreateForumPost creates a new post in a forum channel.
// POST /api/v1/channels/{channelID}/posts
func (h *Handler) HandleCreateForumPost(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.CreateThreads) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need CREATE_THREADS permission")
		return
	}

	// Verify it's a forum channel and get forum settings.
	var channelType string
	var requireTags bool
	var guildID *string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT channel_type, COALESCE(forum_require_tags, false), guild_id
		 FROM channels WHERE id = $1`, channelID).Scan(&channelType, &requireTags, &guildID)
	if err != nil {
		apiutil.WriteError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
		return
	}
	if channelType != models.ChannelTypeForum {
		apiutil.WriteError(w, http.StatusBadRequest, "not_forum", "Posts can only be created in forum channels")
		return
	}

	var req struct {
		Title         string   `json:"title"`
		Content       string   `json:"content"`
		TagIDs        []string `json:"tag_ids"`
		AttachmentIDs []string `json:"attachment_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Title == "" {
		apiutil.WriteError(w, http.StatusBadRequest, "missing_title", "Post title is required")
		return
	}
	if req.Content == "" {
		apiutil.WriteError(w, http.StatusBadRequest, "missing_content", "Post content is required")
		return
	}
	if requireTags && len(req.TagIDs) == 0 {
		apiutil.WriteError(w, http.StatusBadRequest, "tags_required", "This forum requires at least one tag per post")
		return
	}

	// Start transaction.
	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to start transaction")
		return
	}
	defer tx.Rollback(r.Context())

	// 1. Create the OP message in the forum channel.
	msgID := ulid.Make().String()
	_, err = tx.Exec(r.Context(),
		`INSERT INTO messages (id, channel_id, author_id, content, message_type, created_at)
		 VALUES ($1, $2, $3, $4, 'default', now())`,
		msgID, channelID, userID, req.Content)
	if err != nil {
		h.Logger.Error("failed to create forum post message", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create post")
		return
	}

	// 2. Create thread channel (name=title, parent_channel_id=forum).
	threadID := ulid.Make().String()
	var post models.ForumPost
	err = tx.QueryRow(r.Context(),
		`INSERT INTO channels (id, guild_id, channel_type, name, parent_channel_id, owner_id,
		                       pinned, reply_count, last_activity_at, created_at)
		 VALUES ($1, $2, 'text', $3, $4, $5, false, 0, now(), now())
		 RETURNING id, name, owner_id, pinned, locked, reply_count, last_activity_at, created_at`,
		threadID, guildID, req.Title, channelID, userID,
	).Scan(&post.ID, &post.Name, &post.OwnerID, &post.Pinned, &post.Locked,
		&post.ReplyCount, &post.LastActivityAt, &post.CreatedAt)
	if err != nil {
		h.Logger.Error("failed to create forum thread", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create post")
		return
	}

	// 3. Set message thread_id to the new thread.
	tx.Exec(r.Context(), `UPDATE messages SET thread_id = $1 WHERE id = $2`, threadID, msgID)

	// 4. Insert forum_post_tags.
	post.Tags = []models.ForumTag{}
	for _, tagID := range req.TagIDs {
		_, err = tx.Exec(r.Context(),
			`INSERT INTO forum_post_tags (post_id, tag_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			threadID, tagID)
		if err != nil {
			h.Logger.Warn("failed to insert forum post tag", slog.String("error", err.Error()))
		}
	}

	// 5. Link attachments if any.
	if len(req.AttachmentIDs) > 0 {
		tx.Exec(r.Context(),
			`UPDATE attachments SET message_id = $1 WHERE id = ANY($2) AND uploader_id = $3 AND message_id IS NULL`,
			msgID, req.AttachmentIDs, userID)
	}

	// 6. Update forum channel's last_activity_at.
	tx.Exec(r.Context(), `UPDATE channels SET last_activity_at = now() WHERE id = $1`, channelID)

	if err := tx.Commit(r.Context()); err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create post")
		return
	}

	// Load tags for the response.
	if len(req.TagIDs) > 0 {
		tRows, err := h.Pool.Query(r.Context(),
			`SELECT id, channel_id, name, emoji, color, position, created_at
			 FROM forum_tags WHERE id = ANY($1) ORDER BY position`, req.TagIDs)
		if err == nil {
			defer tRows.Close()
			for tRows.Next() {
				var t models.ForumTag
				if err := tRows.Scan(&t.ID, &t.ChannelID, &t.Name, &t.Emoji, &t.Color, &t.Position, &t.CreatedAt); err == nil {
					post.Tags = append(post.Tags, t)
				}
			}
		}
	}

	// Load author.
	var author models.User
	if err := h.Pool.QueryRow(r.Context(),
		`SELECT id, instance_id, username, display_name, avatar_id, flags, created_at
		 FROM users WHERE id = $1`, userID).Scan(
		&author.ID, &author.InstanceID, &author.Username, &author.DisplayName,
		&author.AvatarID, &author.Flags, &author.CreatedAt); err == nil {
		post.Author = &author
	}

	// Set content preview.
	if len(req.Content) > 200 {
		p := req.Content[:200]
		post.ContentPreview = &p
	} else {
		post.ContentPreview = &req.Content
	}

	// Publish CHANNEL_CREATE event for the thread.
	h.EventBus.Publish(r.Context(), "amityvox.channel.create", events.Event{
		Type:      "CHANNEL_CREATE",
		ChannelID: channelID,
		UserID:    userID,
		Data: mustMarshal(map[string]interface{}{
			"id":                threadID,
			"guild_id":          guildID,
			"channel_type":      "text",
			"name":              req.Title,
			"parent_channel_id": channelID,
			"owner_id":          userID,
		}),
	})

	apiutil.WriteJSON(w, http.StatusCreated, post)
}

// HandlePinForumPost toggles the pinned status of a forum post.
// POST /api/v1/channels/{channelID}/posts/{postID}/pin
func (h *Handler) HandlePinForumPost(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	postID := chi.URLParam(r, "postID")

	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ManageThreads) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_THREADS permission")
		return
	}

	// Verify the post belongs to this forum.
	var parentID *string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT parent_channel_id FROM channels WHERE id = $1`, postID).Scan(&parentID)
	if err != nil || parentID == nil || *parentID != channelID {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Post not found in this forum")
		return
	}

	_, err = h.Pool.Exec(r.Context(),
		`UPDATE channels SET pinned = NOT pinned WHERE id = $1`, postID)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to toggle pin")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleCloseForumPost toggles the locked status of a forum post.
// POST /api/v1/channels/{channelID}/posts/{postID}/close
func (h *Handler) HandleCloseForumPost(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	postID := chi.URLParam(r, "postID")

	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ManageThreads) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_THREADS permission")
		return
	}

	// Verify the post belongs to this forum.
	var parentID *string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT parent_channel_id FROM channels WHERE id = $1`, postID).Scan(&parentID)
	if err != nil || parentID == nil || *parentID != channelID {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Post not found in this forum")
		return
	}

	_, err = h.Pool.Exec(r.Context(),
		`UPDATE channels SET locked = NOT locked WHERE id = $1`, postID)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to toggle post lock")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// joinStrings joins string slices with a separator.
func joinStrings(parts []string, sep string) string {
	result := ""
	for i, part := range parts {
		if i > 0 {
			result += sep
		}
		result += part
	}
	return result
}
