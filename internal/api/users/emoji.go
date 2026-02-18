// Package users — user-level custom emoji management.
// Users can upload up to 10 personal emoji that are available in all guilds.
package users

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/amityvox/amityvox/internal/api/apiutil"
	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/models"
)

// UserEmoji represents a personal custom emoji owned by a user.
type UserEmoji struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	FileID    string    `json:"file_id"`
	Animated  bool      `json:"animated"`
	CreatedAt time.Time `json:"created_at"`
}

const maxUserEmoji = 10

type createUserEmojiRequest struct {
	Name     string `json:"name"`
	FileID   string `json:"file_id"`
	Animated bool   `json:"animated"`
}

// HandleGetUserEmoji returns all personal emoji for the authenticated user.
// GET /api/v1/users/@me/emoji
func (h *Handler) HandleGetUserEmoji(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, user_id, name, file_id, animated, created_at
		 FROM user_emoji
		 WHERE user_id = $1
		 ORDER BY created_at ASC`,
		userID,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get emoji", err)
		return
	}
	defer rows.Close()

	emoji := make([]UserEmoji, 0)
	for rows.Next() {
		var e UserEmoji
		if err := rows.Scan(&e.ID, &e.UserID, &e.Name, &e.FileID, &e.Animated, &e.CreatedAt); err != nil {
			apiutil.InternalError(w, h.Logger, "Failed to read emoji", err)
			return
		}
		emoji = append(emoji, e)
	}
	if err := rows.Err(); err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to read emoji", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, emoji)
}

// HandleCreateUserEmoji adds a new personal emoji for the authenticated user.
// Maximum 10 emoji per user.
// POST /api/v1/users/@me/emoji
func (h *Handler) HandleCreateUserEmoji(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var req createUserEmojiRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	if req.Name == "" || len(req.Name) > 32 {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_name", "Emoji name must be 1-32 characters")
		return
	}

	if !apiutil.RequireNonEmpty(w, "file_id", req.FileID) {
		return
	}

	// Check emoji count limit.
	var count int
	err := h.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM user_emoji WHERE user_id = $1`,
		userID,
	).Scan(&count)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to create emoji", err)
		return
	}
	if count >= maxUserEmoji {
		apiutil.WriteError(w, http.StatusBadRequest, "emoji_limit",
			"You have reached the maximum of 10 personal emoji")
		return
	}

	// Check for duplicate name.
	var nameExists bool
	h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM user_emoji WHERE user_id = $1 AND name = $2)`,
		userID, req.Name,
	).Scan(&nameExists)
	if nameExists {
		apiutil.WriteError(w, http.StatusConflict, "duplicate_name", "You already have an emoji with this name")
		return
	}

	emojiID := models.NewULID().String()

	var emoji UserEmoji
	err = h.Pool.QueryRow(r.Context(),
		`INSERT INTO user_emoji (id, user_id, name, file_id, animated, created_at)
		 VALUES ($1, $2, $3, $4, $5, now())
		 RETURNING id, user_id, name, file_id, animated, created_at`,
		emojiID, userID, req.Name, req.FileID, req.Animated,
	).Scan(&emoji.ID, &emoji.UserID, &emoji.Name, &emoji.FileID, &emoji.Animated, &emoji.CreatedAt)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to create emoji", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusCreated, emoji)
}

// HandleDeleteUserEmoji deletes a personal emoji belonging to the authenticated user.
// DELETE /api/v1/users/@me/emoji/{emojiID}
func (h *Handler) HandleDeleteUserEmoji(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	emojiID := chi.URLParam(r, "emojiID")

	// Verify ownership before deleting.
	var ownerID string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT user_id FROM user_emoji WHERE id = $1`,
		emojiID,
	).Scan(&ownerID)
	if err == pgx.ErrNoRows {
		apiutil.WriteError(w, http.StatusNotFound, "emoji_not_found", "Emoji not found")
		return
	}
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to delete emoji", err)
		return
	}
	if ownerID != userID {
		apiutil.WriteError(w, http.StatusForbidden, "not_owner", "You can only delete your own emoji")
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM user_emoji WHERE id = $1 AND user_id = $2`,
		emojiID, userID,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to delete emoji", err)
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "emoji_not_found", "Emoji not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
