// Package users — user channel group handlers.
// Channel groups allow users to organize channels into custom groups on the
// client side, beyond the server-defined category structure. These groups are
// personal and not visible to other users.
// Mounted under /api/v1/users/@me/channel-groups.
package users

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/amityvox/amityvox/internal/api/apiutil"
	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/models"
)

// ChannelGroupHandler implements user channel group REST API endpoints.
type ChannelGroupHandler struct {
	Pool   *pgxpool.Pool
	Logger *slog.Logger
}

// --- Response types ---

type channelGroup struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	Position  int       `json:"position"`
	Color     string    `json:"color"`
	Channels  []string  `json:"channels"`
	CreatedAt time.Time `json:"created_at"`
}

// --- Request types ---

type createChannelGroupRequest struct {
	Name     string `json:"name"`
	Color    string `json:"color"`
	Position *int   `json:"position"`
}

type updateChannelGroupRequest struct {
	Name     *string `json:"name"`
	Color    *string `json:"color"`
	Position *int    `json:"position"`
}

type addChannelToGroupRequest struct {
	ChannelID string `json:"channel_id"`
}

// HandleGetChannelGroups returns all channel groups for the authenticated user.
// GET /api/v1/users/@me/channel-groups
func (h *ChannelGroupHandler) HandleGetChannelGroups(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	rows, err := h.Pool.Query(r.Context(),
		`SELECT g.id, g.user_id, g.name, g.position, g.color, g.created_at,
		        COALESCE(array_agg(gi.channel_id) FILTER (WHERE gi.channel_id IS NOT NULL), '{}')
		 FROM user_channel_groups g
		 LEFT JOIN user_channel_group_items gi ON gi.group_id = g.id
		 WHERE g.user_id = $1
		 GROUP BY g.id
		 ORDER BY g.position ASC, g.created_at ASC`,
		userID,
	)
	if err != nil {
		h.Logger.Error("failed to list channel groups", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list channel groups")
		return
	}
	defer rows.Close()

	groups := make([]channelGroup, 0)
	for rows.Next() {
		var g channelGroup
		if err := rows.Scan(
			&g.ID, &g.UserID, &g.Name, &g.Position, &g.Color, &g.CreatedAt,
			&g.Channels,
		); err != nil {
			h.Logger.Error("failed to scan channel group", slog.String("error", err.Error()))
			apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list channel groups")
			return
		}
		groups = append(groups, g)
	}

	apiutil.WriteJSON(w, http.StatusOK, groups)
}

// HandleCreateChannelGroup creates a new channel group.
// POST /api/v1/users/@me/channel-groups
func (h *ChannelGroupHandler) HandleCreateChannelGroup(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var req createChannelGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Name == "" {
		apiutil.WriteError(w, http.StatusBadRequest, "missing_name", "Group name is required")
		return
	}
	if len(req.Name) > 64 {
		apiutil.WriteError(w, http.StatusBadRequest, "name_too_long", "Group name must be at most 64 characters")
		return
	}
	if len(req.Color) > 7 {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_color", "Color must be a valid hex color (e.g. #ff0000)")
		return
	}

	// Limit to 25 groups per user.
	var count int
	h.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM user_channel_groups WHERE user_id = $1`, userID,
	).Scan(&count)
	if count >= 25 {
		apiutil.WriteError(w, http.StatusBadRequest, "group_limit", "You can have at most 25 channel groups")
		return
	}

	id := models.NewULID().String()
	position := count // Default to end of list.
	if req.Position != nil {
		position = *req.Position
	}

	var g channelGroup
	err := h.Pool.QueryRow(r.Context(),
		`INSERT INTO user_channel_groups (id, user_id, name, position, color, created_at)
		 VALUES ($1, $2, $3, $4, $5, now())
		 RETURNING id, user_id, name, position, color, created_at`,
		id, userID, req.Name, position, req.Color,
	).Scan(&g.ID, &g.UserID, &g.Name, &g.Position, &g.Color, &g.CreatedAt)
	if err != nil {
		h.Logger.Error("failed to create channel group", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create channel group")
		return
	}

	g.Channels = []string{}

	h.Logger.Info("channel group created",
		slog.String("group_id", g.ID),
		slog.String("user_id", userID),
		slog.String("name", g.Name),
	)

	apiutil.WriteJSON(w, http.StatusCreated, g)
}

// HandleUpdateChannelGroup updates an existing channel group.
// PATCH /api/v1/users/@me/channel-groups/{groupID}
func (h *ChannelGroupHandler) HandleUpdateChannelGroup(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	groupID := chi.URLParam(r, "groupID")

	if groupID == "" {
		apiutil.WriteError(w, http.StatusBadRequest, "missing_group_id", "Group ID is required")
		return
	}

	var req updateChannelGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	// Verify ownership.
	var ownerID string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT user_id FROM user_channel_groups WHERE id = $1`, groupID,
	).Scan(&ownerID)
	if err == pgx.ErrNoRows {
		apiutil.WriteError(w, http.StatusNotFound, "group_not_found", "Channel group not found")
		return
	}
	if err != nil {
		h.Logger.Error("failed to check group ownership", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update channel group")
		return
	}
	if ownerID != userID {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "You can only update your own channel groups")
		return
	}

	// Validate fields.
	if req.Name != nil && len(*req.Name) > 64 {
		apiutil.WriteError(w, http.StatusBadRequest, "name_too_long", "Group name must be at most 64 characters")
		return
	}
	if req.Color != nil && len(*req.Color) > 7 {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_color", "Color must be a valid hex color")
		return
	}

	// Build dynamic update.
	if req.Name != nil {
		h.Pool.Exec(r.Context(), `UPDATE user_channel_groups SET name = $2 WHERE id = $1`, groupID, *req.Name)
	}
	if req.Color != nil {
		h.Pool.Exec(r.Context(), `UPDATE user_channel_groups SET color = $2 WHERE id = $1`, groupID, *req.Color)
	}
	if req.Position != nil {
		h.Pool.Exec(r.Context(), `UPDATE user_channel_groups SET position = $2 WHERE id = $1`, groupID, *req.Position)
	}

	// Fetch updated group with channels.
	var g channelGroup
	err = h.Pool.QueryRow(r.Context(),
		`SELECT g.id, g.user_id, g.name, g.position, g.color, g.created_at,
		        COALESCE(array_agg(gi.channel_id) FILTER (WHERE gi.channel_id IS NOT NULL), '{}')
		 FROM user_channel_groups g
		 LEFT JOIN user_channel_group_items gi ON gi.group_id = g.id
		 WHERE g.id = $1
		 GROUP BY g.id`,
		groupID,
	).Scan(&g.ID, &g.UserID, &g.Name, &g.Position, &g.Color, &g.CreatedAt, &g.Channels)
	if err != nil {
		h.Logger.Error("failed to fetch updated group", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update channel group")
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, g)
}

// HandleDeleteChannelGroup removes a channel group and all its items.
// DELETE /api/v1/users/@me/channel-groups/{groupID}
func (h *ChannelGroupHandler) HandleDeleteChannelGroup(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	groupID := chi.URLParam(r, "groupID")

	if groupID == "" {
		apiutil.WriteError(w, http.StatusBadRequest, "missing_group_id", "Group ID is required")
		return
	}

	// Verify ownership.
	var ownerID string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT user_id FROM user_channel_groups WHERE id = $1`, groupID,
	).Scan(&ownerID)
	if err == pgx.ErrNoRows {
		apiutil.WriteError(w, http.StatusNotFound, "group_not_found", "Channel group not found")
		return
	}
	if err != nil {
		h.Logger.Error("failed to check group ownership", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete channel group")
		return
	}
	if ownerID != userID {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "You can only delete your own channel groups")
		return
	}

	// Items cascade-deleted due to FK constraint.
	_, err = h.Pool.Exec(r.Context(),
		`DELETE FROM user_channel_groups WHERE id = $1`, groupID)
	if err != nil {
		h.Logger.Error("failed to delete channel group", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete channel group")
		return
	}

	h.Logger.Info("channel group deleted",
		slog.String("group_id", groupID),
		slog.String("user_id", userID),
	)

	w.WriteHeader(http.StatusNoContent)
}

// HandleAddChannelToGroup adds a channel to a user's channel group.
// PUT /api/v1/users/@me/channel-groups/{groupID}/channels
func (h *ChannelGroupHandler) HandleAddChannelToGroup(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	groupID := chi.URLParam(r, "groupID")

	if groupID == "" {
		apiutil.WriteError(w, http.StatusBadRequest, "missing_group_id", "Group ID is required")
		return
	}

	var req addChannelToGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if req.ChannelID == "" {
		apiutil.WriteError(w, http.StatusBadRequest, "missing_channel_id", "Channel ID is required")
		return
	}

	// Verify group ownership.
	var ownerID string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT user_id FROM user_channel_groups WHERE id = $1`, groupID,
	).Scan(&ownerID)
	if err == pgx.ErrNoRows {
		apiutil.WriteError(w, http.StatusNotFound, "group_not_found", "Channel group not found")
		return
	}
	if err != nil {
		h.Logger.Error("failed to check group ownership", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to add channel to group")
		return
	}
	if ownerID != userID {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "You can only modify your own channel groups")
		return
	}

	// Limit to 100 channels per group.
	var count int
	h.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM user_channel_group_items WHERE group_id = $1`, groupID,
	).Scan(&count)
	if count >= 100 {
		apiutil.WriteError(w, http.StatusBadRequest, "channel_limit", "A channel group can have at most 100 channels")
		return
	}

	_, err = h.Pool.Exec(r.Context(),
		`INSERT INTO user_channel_group_items (group_id, channel_id)
		 VALUES ($1, $2)
		 ON CONFLICT (group_id, channel_id) DO NOTHING`,
		groupID, req.ChannelID,
	)
	if err != nil {
		h.Logger.Error("failed to add channel to group", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to add channel to group")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleRemoveChannelFromGroup removes a channel from a user's channel group.
// DELETE /api/v1/users/@me/channel-groups/{groupID}/channels/{channelID}
func (h *ChannelGroupHandler) HandleRemoveChannelFromGroup(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	groupID := chi.URLParam(r, "groupID")
	channelID := chi.URLParam(r, "channelID")

	if groupID == "" || channelID == "" {
		apiutil.WriteError(w, http.StatusBadRequest, "missing_params", "Group ID and Channel ID are required")
		return
	}

	// Verify group ownership.
	var ownerID string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT user_id FROM user_channel_groups WHERE id = $1`, groupID,
	).Scan(&ownerID)
	if err == pgx.ErrNoRows {
		apiutil.WriteError(w, http.StatusNotFound, "group_not_found", "Channel group not found")
		return
	}
	if err != nil {
		h.Logger.Error("failed to check group ownership", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to remove channel from group")
		return
	}
	if ownerID != userID {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "You can only modify your own channel groups")
		return
	}

	_, err = h.Pool.Exec(r.Context(),
		`DELETE FROM user_channel_group_items WHERE group_id = $1 AND channel_id = $2`,
		groupID, channelID,
	)
	if err != nil {
		h.Logger.Error("failed to remove channel from group", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to remove channel from group")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
