// Package channels — channel-specific emoji handlers.
// Channel emoji are emoji scoped to a single channel within a guild, as opposed
// to guild-wide custom emoji. Mounted under /api/v1/channels/{channelID}/emoji.
package channels

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/models"
)

// EmojiHandler implements channel-specific emoji REST API endpoints.
type EmojiHandler struct {
	Pool     *pgxpool.Pool
	EventBus *events.Bus
	Logger   *slog.Logger
}

// --- Response types ---

type channelEmoji struct {
	ID        string    `json:"id"`
	ChannelID string    `json:"channel_id"`
	GuildID   string    `json:"guild_id"`
	Name      string    `json:"name"`
	CreatorID string    `json:"creator_id"`
	Animated  bool      `json:"animated"`
	S3Key     string    `json:"s3_key"`
	CreatedAt time.Time `json:"created_at"`
}

// --- Request types ---

type createChannelEmojiRequest struct {
	Name     string `json:"name"`
	FileID   string `json:"file_id"`
	Animated bool   `json:"animated"`
}

// HandleGetChannelEmoji returns all emoji for a specific channel.
// GET /api/v1/channels/{channelID}/emoji
func (h *EmojiHandler) HandleGetChannelEmoji(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	if channelID == "" {
		writeError(w, http.StatusBadRequest, "missing_channel_id", "Channel ID is required")
		return
	}

	// Verify the user has access to this channel (is a member of the guild).
	guildID, err := h.getChannelGuild(r, channelID)
	if err != nil {
		writeError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
		return
	}
	if !h.isMember(r, guildID, userID) {
		writeError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, channel_id, guild_id, name, creator_id, animated, s3_key, created_at
		 FROM channel_emoji
		 WHERE channel_id = $1
		 ORDER BY created_at ASC`,
		channelID,
	)
	if err != nil {
		h.Logger.Error("failed to list channel emoji", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list emoji")
		return
	}
	defer rows.Close()

	emoji := make([]channelEmoji, 0)
	for rows.Next() {
		var e channelEmoji
		if err := rows.Scan(
			&e.ID, &e.ChannelID, &e.GuildID, &e.Name,
			&e.CreatorID, &e.Animated, &e.S3Key, &e.CreatedAt,
		); err != nil {
			h.Logger.Error("failed to scan channel emoji", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list emoji")
			return
		}
		emoji = append(emoji, e)
	}

	writeJSON(w, http.StatusOK, emoji)
}

// HandleCreateChannelEmoji adds a new emoji to a channel.
// POST /api/v1/channels/{channelID}/emoji
func (h *EmojiHandler) HandleCreateChannelEmoji(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	if channelID == "" {
		writeError(w, http.StatusBadRequest, "missing_channel_id", "Channel ID is required")
		return
	}

	var req createChannelEmojiRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "missing_name", "Emoji name is required")
		return
	}
	if len(req.Name) > 32 {
		writeError(w, http.StatusBadRequest, "name_too_long", "Emoji name must be at most 32 characters")
		return
	}
	if req.FileID == "" {
		writeError(w, http.StatusBadRequest, "missing_file_id", "File ID (S3 key) is required")
		return
	}

	// Verify channel access and get guild ID.
	guildID, err := h.getChannelGuild(r, channelID)
	if err != nil {
		writeError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
		return
	}
	if !h.isMember(r, guildID, userID) {
		writeError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	// Check for duplicate name in this channel.
	var duplicate bool
	h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM channel_emoji WHERE channel_id = $1 AND name = $2)`,
		channelID, req.Name,
	).Scan(&duplicate)
	if duplicate {
		writeError(w, http.StatusConflict, "emoji_exists", "An emoji with this name already exists in the channel")
		return
	}

	// Limit to 50 emoji per channel.
	var count int
	h.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM channel_emoji WHERE channel_id = $1`, channelID,
	).Scan(&count)
	if count >= 50 {
		writeError(w, http.StatusBadRequest, "emoji_limit", "This channel has reached the maximum of 50 emoji")
		return
	}

	id := models.NewULID().String()
	var e channelEmoji
	err = h.Pool.QueryRow(r.Context(),
		`INSERT INTO channel_emoji (id, channel_id, guild_id, name, creator_id, animated, s3_key, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		 RETURNING id, channel_id, guild_id, name, creator_id, animated, s3_key, created_at`,
		id, channelID, guildID, req.Name, userID, req.Animated, req.FileID,
	).Scan(
		&e.ID, &e.ChannelID, &e.GuildID, &e.Name,
		&e.CreatorID, &e.Animated, &e.S3Key, &e.CreatedAt,
	)
	if err != nil {
		h.Logger.Error("failed to create channel emoji", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create emoji")
		return
	}

	h.Logger.Info("channel emoji created",
		slog.String("emoji_id", e.ID),
		slog.String("channel_id", channelID),
		slog.String("name", e.Name),
	)

	writeJSON(w, http.StatusCreated, e)
}

// HandleDeleteChannelEmoji removes an emoji from a channel.
// DELETE /api/v1/channels/{channelID}/emoji/{emojiID}
func (h *EmojiHandler) HandleDeleteChannelEmoji(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	emojiID := chi.URLParam(r, "emojiID")

	if channelID == "" || emojiID == "" {
		writeError(w, http.StatusBadRequest, "missing_params", "Channel ID and Emoji ID are required")
		return
	}

	// Verify channel access.
	guildID, err := h.getChannelGuild(r, channelID)
	if err != nil {
		writeError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
		return
	}
	if !h.isMember(r, guildID, userID) {
		writeError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	// Check ownership: the creator or guild owner can delete.
	var creatorID string
	err = h.Pool.QueryRow(r.Context(),
		`SELECT creator_id FROM channel_emoji WHERE id = $1 AND channel_id = $2`,
		emojiID, channelID,
	).Scan(&creatorID)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "emoji_not_found", "Emoji not found")
		return
	}
	if err != nil {
		h.Logger.Error("failed to check emoji ownership", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete emoji")
		return
	}

	if creatorID != userID {
		// Check if user is guild owner.
		var ownerID string
		h.Pool.QueryRow(r.Context(),
			`SELECT owner_id FROM guilds WHERE id = $1`, guildID,
		).Scan(&ownerID)
		if ownerID != userID {
			writeError(w, http.StatusForbidden, "forbidden", "You can only delete emoji you created")
			return
		}
	}

	_, err = h.Pool.Exec(r.Context(),
		`DELETE FROM channel_emoji WHERE id = $1 AND channel_id = $2`,
		emojiID, channelID,
	)
	if err != nil {
		h.Logger.Error("failed to delete channel emoji", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete emoji")
		return
	}

	h.Logger.Info("channel emoji deleted",
		slog.String("emoji_id", emojiID),
		slog.String("channel_id", channelID),
		slog.String("deleted_by", userID),
	)

	w.WriteHeader(http.StatusNoContent)
}

// --- Internal helpers ---

func (h *EmojiHandler) getChannelGuild(r *http.Request, channelID string) (string, error) {
	var guildID string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT guild_id FROM channels WHERE id = $1`, channelID,
	).Scan(&guildID)
	return guildID, err
}

func (h *EmojiHandler) isMember(r *http.Request, guildID, userID string) bool {
	var exists bool
	h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM guild_members WHERE guild_id = $1 AND user_id = $2)`,
		guildID, userID,
	).Scan(&exists)
	return exists
}
