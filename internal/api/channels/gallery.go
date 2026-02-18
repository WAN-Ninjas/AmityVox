package channels

import (
	"net/http"
	"strconv"
	"strings"
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
// Gallery Tag CRUD
// =============================================================================

// HandleGetGalleryTags lists tags for a gallery channel.
// GET /api/v1/channels/{channelID}/gallery-tags
func (h *Handler) HandleGetGalleryTags(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ViewChannel) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need VIEW_CHANNEL permission")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, channel_id, name, emoji, color, position, created_at
		 FROM gallery_tags WHERE channel_id = $1 ORDER BY position`, channelID)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list tags")
		return
	}
	defer rows.Close()

	var tags []models.GalleryTag
	for rows.Next() {
		var t models.GalleryTag
		if err := rows.Scan(&t.ID, &t.ChannelID, &t.Name, &t.Emoji, &t.Color, &t.Position, &t.CreatedAt); err != nil {
			continue
		}
		tags = append(tags, t)
	}
	if tags == nil {
		tags = []models.GalleryTag{}
	}

	apiutil.WriteJSON(w, http.StatusOK, tags)
}

// HandleCreateGalleryTag creates a new tag for a gallery channel.
// POST /api/v1/channels/{channelID}/gallery-tags
func (h *Handler) HandleCreateGalleryTag(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ManageChannels) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_CHANNELS permission")
		return
	}

	// Verify it's a gallery channel.
	var channelType string
	if err := h.Pool.QueryRow(r.Context(), `SELECT channel_type FROM channels WHERE id = $1`, channelID).Scan(&channelType); err != nil {
		apiutil.WriteError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
		return
	}
	if channelType != models.ChannelTypeGallery {
		apiutil.WriteError(w, http.StatusBadRequest, "not_gallery", "Tags can only be created on gallery channels")
		return
	}

	var req struct {
		Name  string  `json:"name"`
		Emoji *string `json:"emoji"`
		Color *string `json:"color"`
	}
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}
	if !apiutil.RequireNonEmpty(w, "Tag name", req.Name) {
		return
	}

	// Get next position.
	var maxPos int
	h.Pool.QueryRow(r.Context(),
		`SELECT COALESCE(MAX(position), -1) FROM gallery_tags WHERE channel_id = $1`, channelID).Scan(&maxPos)

	id := ulid.Make().String()
	var tag models.GalleryTag
	err := h.Pool.QueryRow(r.Context(),
		`INSERT INTO gallery_tags (id, channel_id, name, emoji, color, position, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, now())
		 RETURNING id, channel_id, name, emoji, color, position, created_at`,
		id, channelID, req.Name, req.Emoji, req.Color, maxPos+1,
	).Scan(&tag.ID, &tag.ChannelID, &tag.Name, &tag.Emoji, &tag.Color, &tag.Position, &tag.CreatedAt)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to create tag", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusCreated, tag)
}

// HandleUpdateGalleryTag updates a gallery tag.
// PATCH /api/v1/channels/{channelID}/gallery-tags/{tagID}
func (h *Handler) HandleUpdateGalleryTag(w http.ResponseWriter, r *http.Request) {
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
	if !apiutil.DecodeJSON(w, r, &req) {
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
	query := "UPDATE gallery_tags SET " + joinStrings(setClauses, ", ") +
		" WHERE id = $" + strconv.Itoa(argIdx) +
		" AND channel_id = $" + strconv.Itoa(argIdx+1) +
		" RETURNING id, channel_id, name, emoji, color, position, created_at"

	var tag models.GalleryTag
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

// HandleDeleteGalleryTag deletes a gallery tag.
// DELETE /api/v1/channels/{channelID}/gallery-tags/{tagID}
func (h *Handler) HandleDeleteGalleryTag(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	tagID := chi.URLParam(r, "tagID")

	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ManageChannels) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_CHANNELS permission")
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM gallery_tags WHERE id = $1 AND channel_id = $2`, tagID, channelID)
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
// Gallery Post Operations
// =============================================================================

// HandleGetGalleryPosts lists posts in a gallery channel with sorting, filtering, and pagination.
// GET /api/v1/channels/{channelID}/gallery-posts
func (h *Handler) HandleGetGalleryPosts(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ViewChannel) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need VIEW_CHANNEL permission")
		return
	}

	// Verify it's a gallery channel.
	var channelType string
	if err := h.Pool.QueryRow(r.Context(), `SELECT channel_type FROM channels WHERE id = $1`, channelID).Scan(&channelType); err != nil {
		apiutil.WriteError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
		return
	}
	if channelType != models.ChannelTypeGallery {
		apiutil.WriteError(w, http.StatusBadRequest, "not_gallery", "This is not a gallery channel")
		return
	}

	sortBy := r.URL.Query().Get("sort")
	if sortBy == "" {
		sortBy = "newest"
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

	// Build query for thread channels whose parent is this gallery.
	query := `SELECT c.id, c.name, c.owner_id, c.pinned, c.locked, c.reply_count,
	                 c.last_activity_at, c.created_at
	          FROM channels c`

	args := []interface{}{channelID}
	argIdx := 2
	whereClause := ` WHERE c.parent_channel_id = $1 AND c.channel_type = 'text'`

	if tagFilter != "" {
		query += ` JOIN gallery_post_tags gpt ON gpt.post_id = c.id`
		whereClause += ` AND gpt.tag_id = $` + strconv.Itoa(argIdx)
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
	case "oldest":
		query += ` ORDER BY c.pinned DESC, c.created_at ASC`
	case "most_comments":
		query += ` ORDER BY c.pinned DESC, c.reply_count DESC`
	default: // newest
		query += ` ORDER BY c.pinned DESC, c.created_at DESC`
	}

	query += ` LIMIT $` + strconv.Itoa(argIdx)
	args = append(args, limit)

	rows, err := h.Pool.Query(r.Context(), query, args...)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to load gallery posts", err)
		return
	}
	defer rows.Close()

	var posts []models.GalleryPost
	var ownerIDs []string
	var postIDs []string
	for rows.Next() {
		var p models.GalleryPost
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
		posts = []models.GalleryPost{}
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
		tagMap := make(map[string][]models.GalleryTag)
		tRows, err := h.Pool.Query(r.Context(),
			`SELECT gpt.post_id, gt.id, gt.channel_id, gt.name, gt.emoji, gt.color, gt.position, gt.created_at
			 FROM gallery_post_tags gpt
			 JOIN gallery_tags gt ON gt.id = gpt.tag_id
			 WHERE gpt.post_id = ANY($1)
			 ORDER BY gt.position`, postIDs)
		if err == nil {
			defer tRows.Close()
			for tRows.Next() {
				var postID string
				var t models.GalleryTag
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
				posts[i].Tags = []models.GalleryTag{}
			}
		}
	}

	// Batch-load thumbnail (first media attachment) and description (first message content) per post.
	if len(postIDs) > 0 {
		// Get first message per thread (the OP). The OP message has thread_id = the post's thread channel ID.
		previewRows, err := h.Pool.Query(r.Context(),
			`SELECT DISTINCT ON (thread_id) thread_id, id, LEFT(content, 500)
			 FROM messages WHERE thread_id = ANY($1)
			 ORDER BY thread_id, created_at ASC`, postIDs)
		if err == nil {
			defer previewRows.Close()
			descMap := make(map[string]string)
			msgIDMap := make(map[string]string) // postID -> messageID
			for previewRows.Next() {
				var cid, msgID string
				var desc *string
				if err := previewRows.Scan(&cid, &msgID, &desc); err == nil {
					msgIDMap[cid] = msgID
					if desc != nil {
						descMap[cid] = *desc
					}
				}
			}
			for i := range posts {
				if d, ok := descMap[posts[i].ID]; ok {
					posts[i].Description = &d
				}
			}

			// Batch-load first image/video attachment for each OP message.
			if len(msgIDMap) > 0 {
				msgIDs := make([]string, 0, len(msgIDMap))
				msgToPost := make(map[string]string) // messageID -> postID
				for postID, msgID := range msgIDMap {
					msgIDs = append(msgIDs, msgID)
					msgToPost[msgID] = postID
				}
				aRows, err := h.Pool.Query(r.Context(),
					`SELECT DISTINCT ON (message_id) message_id, id, filename, content_type, size_bytes,
					        width, height, duration_seconds, s3_bucket, s3_key, blurhash, alt_text, nsfw, description, created_at
					 FROM attachments
					 WHERE message_id = ANY($1)
					   AND (content_type LIKE 'image/%' OR content_type LIKE 'video/%')
					 ORDER BY message_id, created_at ASC`, msgIDs)
				if err == nil {
					defer aRows.Close()
					for aRows.Next() {
						var msgID string
						var a models.Attachment
						if err := aRows.Scan(&msgID, &a.ID, &a.Filename, &a.ContentType,
							&a.SizeBytes, &a.Width, &a.Height, &a.DurationSeconds,
							&a.S3Bucket, &a.S3Key, &a.Blurhash, &a.AltText,
							&a.NSFW, &a.Description, &a.CreatedAt); err == nil {
							if postID, ok := msgToPost[msgID]; ok {
								for j := range posts {
									if posts[j].ID == postID {
										posts[j].Thumbnail = &a
										break
									}
								}
							}
						}
					}
				}
			}
		}
	}

	apiutil.WriteJSON(w, http.StatusOK, posts)
}

// HandleCreateGalleryPost creates a new post in a gallery channel.
// POST /api/v1/channels/{channelID}/gallery-posts
func (h *Handler) HandleCreateGalleryPost(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.CreateThreads) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need CREATE_THREADS permission")
		return
	}

	// Verify it's a gallery channel and get settings.
	var channelType string
	var requireTags bool
	var guildID *string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT channel_type, COALESCE(gallery_require_tags, false), guild_id
		 FROM channels WHERE id = $1`, channelID).Scan(&channelType, &requireTags, &guildID)
	if err != nil {
		apiutil.WriteError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
		return
	}
	if channelType != models.ChannelTypeGallery {
		apiutil.WriteError(w, http.StatusBadRequest, "not_gallery", "Posts can only be created in gallery channels")
		return
	}

	var req struct {
		Title         string   `json:"title"`
		Description   string   `json:"description"`
		TagIDs        []string `json:"tag_ids"`
		AttachmentIDs []string `json:"attachment_ids"`
	}
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	if len(req.AttachmentIDs) == 0 {
		apiutil.WriteError(w, http.StatusBadRequest, "missing_attachments", "Gallery posts require at least one image or video attachment")
		return
	}
	if requireTags && len(req.TagIDs) == 0 {
		apiutil.WriteError(w, http.StatusBadRequest, "tags_required", "This gallery requires at least one tag per post")
		return
	}

	// Validate all attachments are image/* or video/*.
	var validCount int
	h.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM attachments
		 WHERE id = ANY($1) AND uploader_id = $2 AND message_id IS NULL
		   AND (content_type LIKE 'image/%' OR content_type LIKE 'video/%')`,
		req.AttachmentIDs, userID).Scan(&validCount)
	if validCount != len(req.AttachmentIDs) {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_attachments", "All attachments must be images or videos that you uploaded")
		return
	}

	// Use description as message content, or a default.
	content := req.Description
	if content == "" {
		content = req.Title
		if content == "" {
			content = "Gallery post"
		}
	}

	// Start transaction.
	msgID := ulid.Make().String()
	threadID := ulid.Make().String()
	threadName := req.Title
	if threadName == "" {
		threadName = "Gallery Post"
	}
	var post models.GalleryPost
	err = apiutil.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		// 1. Create the OP message in the gallery channel.
		if _, err := tx.Exec(r.Context(),
			`INSERT INTO messages (id, channel_id, author_id, content, message_type, created_at)
			 VALUES ($1, $2, $3, $4, 'default', now())`,
			msgID, channelID, userID, content); err != nil {
			return err
		}

		// 2. Create thread channel (name=title, parent_channel_id=gallery).
		if err := tx.QueryRow(r.Context(),
			`INSERT INTO channels (id, guild_id, channel_type, name, parent_channel_id, owner_id,
			                       pinned, reply_count, last_activity_at, created_at)
			 VALUES ($1, $2, 'text', $3, $4, $5, false, 0, now(), now())
			 RETURNING id, name, owner_id, pinned, locked, reply_count, last_activity_at, created_at`,
			threadID, guildID, threadName, channelID, userID,
		).Scan(&post.ID, &post.Name, &post.OwnerID, &post.Pinned, &post.Locked,
			&post.ReplyCount, &post.LastActivityAt, &post.CreatedAt); err != nil {
			return err
		}

		// 3. Set message thread_id to the new thread.
		if _, err := tx.Exec(r.Context(),
			`UPDATE messages SET thread_id = $1 WHERE id = $2`, threadID, msgID); err != nil {
			return err
		}

		// 4. Insert gallery_post_tags.
		post.Tags = []models.GalleryTag{}
		for _, tagID := range req.TagIDs {
			if _, err := tx.Exec(r.Context(),
				`INSERT INTO gallery_post_tags (post_id, tag_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
				threadID, tagID); err != nil {
				return err
			}
		}

		// 5. Link attachments.
		if _, err := tx.Exec(r.Context(),
			`UPDATE attachments SET message_id = $1 WHERE id = ANY($2) AND uploader_id = $3 AND message_id IS NULL`,
			msgID, req.AttachmentIDs, userID); err != nil {
			return err
		}

		// 6. Update gallery channel's last_activity_at.
		if _, err := tx.Exec(r.Context(),
			`UPDATE channels SET last_activity_at = now() WHERE id = $1`, channelID); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create post")
		return
	}

	// Load tags for the response.
	if len(req.TagIDs) > 0 {
		tRows, err := h.Pool.Query(r.Context(),
			`SELECT id, channel_id, name, emoji, color, position, created_at
			 FROM gallery_tags WHERE id = ANY($1) ORDER BY position`, req.TagIDs)
		if err == nil {
			defer tRows.Close()
			for tRows.Next() {
				var t models.GalleryTag
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

	// Set description.
	if req.Description != "" {
		post.Description = &req.Description
	}

	// Load thumbnail (first attachment).
	if len(req.AttachmentIDs) > 0 {
		var a models.Attachment
		err := h.Pool.QueryRow(r.Context(),
			`SELECT id, message_id, uploader_id, filename, content_type, size_bytes,
			        width, height, duration_seconds, s3_bucket, s3_key, blurhash, alt_text, nsfw, description, created_at
			 FROM attachments WHERE id = $1`, req.AttachmentIDs[0]).Scan(
			&a.ID, &a.MessageID, &a.UploaderID, &a.Filename, &a.ContentType,
			&a.SizeBytes, &a.Width, &a.Height, &a.DurationSeconds,
			&a.S3Bucket, &a.S3Key, &a.Blurhash, &a.AltText,
			&a.NSFW, &a.Description, &a.CreatedAt)
		if err == nil {
			post.Thumbnail = &a
		}
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
			"name":              threadName,
			"parent_channel_id": channelID,
			"owner_id":          userID,
		}),
	})

	apiutil.WriteJSON(w, http.StatusCreated, post)
}

// HandlePinGalleryPost toggles the pinned status of a gallery post.
// POST /api/v1/channels/{channelID}/gallery-posts/{postID}/pin
func (h *Handler) HandlePinGalleryPost(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	postID := chi.URLParam(r, "postID")

	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ManageThreads) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_THREADS permission")
		return
	}

	// Verify the post belongs to this gallery.
	var parentID *string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT parent_channel_id FROM channels WHERE id = $1`, postID).Scan(&parentID)
	if err != nil || parentID == nil || *parentID != channelID {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Post not found in this gallery")
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

// HandleCloseGalleryPost toggles the locked status of a gallery post.
// POST /api/v1/channels/{channelID}/gallery-posts/{postID}/close
func (h *Handler) HandleCloseGalleryPost(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	postID := chi.URLParam(r, "postID")

	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ManageThreads) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_THREADS permission")
		return
	}

	// Verify the post belongs to this gallery.
	var parentID *string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT parent_channel_id FROM channels WHERE id = $1`, postID).Scan(&parentID)
	if err != nil || parentID == nil || *parentID != channelID {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Post not found in this gallery")
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

// isMediaContentType checks if a content type is an image or video.
func isMediaContentType(ct string) bool {
	return strings.HasPrefix(ct, "image/") || strings.HasPrefix(ct, "video/")
}
