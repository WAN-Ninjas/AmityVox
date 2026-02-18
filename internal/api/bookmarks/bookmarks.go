// Package bookmarks implements REST API handlers for message bookmark (saved
// messages) operations including creating, deleting, and listing bookmarks.
// Mounted under /api/v1/messages and /api/v1/users/@me/bookmarks.
package bookmarks

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/amityvox/amityvox/internal/api/apiutil"
	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/models"
)

// Handler implements bookmark-related REST API endpoints.
type Handler struct {
	Pool   *pgxpool.Pool
	Logger *slog.Logger
}

// --- Request types ---

type createBookmarkRequest struct {
	Note       *string `json:"note"`
	ReminderAt *string `json:"reminder_at"`
}

// HandleCreateBookmark creates or updates a bookmark on a message.
// PUT /api/v1/messages/{messageID}/bookmark
func (h *Handler) HandleCreateBookmark(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	messageID := chi.URLParam(r, "messageID")

	// Parse optional request body.
	var req createBookmarkRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apiutil.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}
	}

	// Validate note length if provided.
	if req.Note != nil && len(*req.Note) > 1000 {
		apiutil.WriteError(w, http.StatusBadRequest, "note_too_long", "Bookmark note must be at most 1000 characters")
		return
	}

	// Parse reminder_at if provided.
	var reminderAt *time.Time
	if req.ReminderAt != nil && *req.ReminderAt != "" {
		t, err := time.Parse(time.RFC3339, *req.ReminderAt)
		if err != nil {
			apiutil.WriteError(w, http.StatusBadRequest, "invalid_reminder_at", "Invalid reminder_at format; use RFC3339")
			return
		}
		if t.Before(time.Now()) {
			apiutil.WriteError(w, http.StatusBadRequest, "invalid_reminder_at", "Reminder time must be in the future")
			return
		}
		reminderAt = &t
	}

	// Verify the message exists.
	var exists bool
	err := h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM messages WHERE id = $1)`,
		messageID,
	).Scan(&exists)
	if err != nil {
		h.Logger.Error("failed to check message existence", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to check message")
		return
	}
	if !exists {
		apiutil.WriteError(w, http.StatusNotFound, "message_not_found", "Message not found")
		return
	}

	// Upsert the bookmark.
	var bookmark models.MessageBookmark
	err = h.Pool.QueryRow(r.Context(),
		`INSERT INTO message_bookmarks (user_id, message_id, note, reminder_at, reminded, created_at)
		 VALUES ($1, $2, $3, $4, false, now())
		 ON CONFLICT (user_id, message_id) DO UPDATE SET note = $3, reminder_at = $4, reminded = false
		 RETURNING user_id, message_id, note, reminder_at, reminded, created_at`,
		userID, messageID, req.Note, reminderAt,
	).Scan(
		&bookmark.UserID, &bookmark.MessageID, &bookmark.Note,
		&bookmark.ReminderAt, &bookmark.Reminded, &bookmark.CreatedAt,
	)
	if err != nil {
		h.Logger.Error("failed to create bookmark",
			slog.String("user_id", userID),
			slog.String("message_id", messageID),
			slog.String("error", err.Error()),
		)
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create bookmark")
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, bookmark)
}

// HandleDeleteBookmark removes a bookmark from a message.
// DELETE /api/v1/messages/{messageID}/bookmark
func (h *Handler) HandleDeleteBookmark(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	messageID := chi.URLParam(r, "messageID")

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM message_bookmarks WHERE user_id = $1 AND message_id = $2`,
		userID, messageID,
	)
	if err != nil {
		h.Logger.Error("failed to delete bookmark",
			slog.String("user_id", userID),
			slog.String("message_id", messageID),
			slog.String("error", err.Error()),
		)
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete bookmark")
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "bookmark_not_found", "Bookmark not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleListBookmarks returns the authenticated user's bookmarked messages.
// GET /api/v1/users/@me/bookmarks?limit=50&before=ULID
func (h *Handler) HandleListBookmarks(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	// Parse limit parameter (default 50, max 100).
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	before := r.URL.Query().Get("before")

	var rows pgx.Rows
	var err error

	if before != "" {
		rows, err = h.Pool.Query(r.Context(),
			`SELECT b.user_id, b.message_id, b.note, b.reminder_at, b.reminded, b.created_at,
			        m.id, m.channel_id, m.author_id, m.content, m.message_type, m.created_at,
			        u.id, u.instance_id, u.username, u.display_name, u.avatar_id,
			        u.status_text, u.status_emoji, u.status_presence, u.bio, u.flags, u.created_at
			 FROM message_bookmarks b
			 JOIN messages m ON m.id = b.message_id
			 JOIN users u ON u.id = m.author_id
			 WHERE b.user_id = $1 AND b.created_at < (
			     SELECT created_at FROM message_bookmarks
			     WHERE user_id = $1 AND message_id = $2
			 )
			 ORDER BY b.created_at DESC
			 LIMIT $3`,
			userID, before, limit,
		)
	} else {
		rows, err = h.Pool.Query(r.Context(),
			`SELECT b.user_id, b.message_id, b.note, b.reminder_at, b.reminded, b.created_at,
			        m.id, m.channel_id, m.author_id, m.content, m.message_type, m.created_at,
			        u.id, u.instance_id, u.username, u.display_name, u.avatar_id,
			        u.status_text, u.status_emoji, u.status_presence, u.bio, u.flags, u.created_at
			 FROM message_bookmarks b
			 JOIN messages m ON m.id = b.message_id
			 JOIN users u ON u.id = m.author_id
			 WHERE b.user_id = $1
			 ORDER BY b.created_at DESC
			 LIMIT $2`,
			userID, limit,
		)
	}
	if err != nil {
		h.Logger.Error("failed to list bookmarks",
			slog.String("user_id", userID),
			slog.String("error", err.Error()),
		)
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list bookmarks")
		return
	}
	defer rows.Close()

	bookmarks := make([]models.MessageBookmark, 0)
	for rows.Next() {
		var b models.MessageBookmark
		var msg models.Message
		var author models.User

		if err := rows.Scan(
			&b.UserID, &b.MessageID, &b.Note, &b.ReminderAt, &b.Reminded, &b.CreatedAt,
			&msg.ID, &msg.ChannelID, &msg.AuthorID, &msg.Content, &msg.MessageType, &msg.CreatedAt,
			&author.ID, &author.InstanceID, &author.Username, &author.DisplayName, &author.AvatarID,
			&author.StatusText, &author.StatusEmoji, &author.StatusPresence, &author.Bio, &author.Flags, &author.CreatedAt,
		); err != nil {
			h.Logger.Error("failed to scan bookmark row", slog.String("error", err.Error()))
			apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to read bookmarks")
			return
		}

		msg.Author = &author
		b.Message = &msg
		bookmarks = append(bookmarks, b)
	}

	apiutil.WriteJSON(w, http.StatusOK, bookmarks)
}
