// Guild channel group handlers.
// Channel groups are guild-wide organizational categories managed by admins with
// ManageChannels permission. All guild members see the same groups in the same order.
// Mounted under /api/v1/guilds/{guildID}/channel-groups.
package guilds

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/amityvox/amityvox/internal/api/apiutil"
	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/models"
	"github.com/amityvox/amityvox/internal/permissions"
)

// --- Response types ---

type channelGroup struct {
	ID        string    `json:"id"`
	GuildID   string    `json:"guild_id"`
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

type setGroupChannelsRequest struct {
	ChannelIDs []string `json:"channel_ids"`
}

// HandleGetChannelGroups returns all channel groups for a guild.
// GET /api/v1/guilds/{guildID}/channel-groups
func (h *Handler) HandleGetChannelGroups(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")

	rows, err := h.Pool.Query(r.Context(),
		`SELECT g.id, g.guild_id, g.name, g.position, g.color, g.created_at,
		        COALESCE(array_agg(gi.channel_id ORDER BY gi.position) FILTER (WHERE gi.channel_id IS NOT NULL), '{}')
		 FROM guild_channel_groups g
		 LEFT JOIN guild_channel_group_items gi ON gi.group_id = g.id
		 WHERE g.guild_id = $1
		 GROUP BY g.id
		 ORDER BY g.position ASC, g.created_at ASC`,
		guildID,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to list channel groups", err)
		return
	}
	defer rows.Close()

	groups := make([]channelGroup, 0)
	for rows.Next() {
		var g channelGroup
		if err := rows.Scan(
			&g.ID, &g.GuildID, &g.Name, &g.Position, &g.Color, &g.CreatedAt,
			&g.Channels,
		); err != nil {
			apiutil.InternalError(w, h.Logger, "Failed to scan channel group", err)
			return
		}
		groups = append(groups, g)
	}

	apiutil.WriteJSON(w, http.StatusOK, groups)
}

// HandleCreateChannelGroup creates a new channel group for a guild.
// POST /api/v1/guilds/{guildID}/channel-groups
func (h *Handler) HandleCreateChannelGroup(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageChannels) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "You need ManageChannels permission")
		return
	}

	var req createChannelGroupRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	if !apiutil.RequireNonEmpty(w, "Group name", req.Name) {
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

	// Limit to 25 groups per guild.
	var count int
	h.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM guild_channel_groups WHERE guild_id = $1`, guildID,
	).Scan(&count)
	if count >= 25 {
		apiutil.WriteError(w, http.StatusBadRequest, "group_limit", "A guild can have at most 25 channel groups")
		return
	}

	id := models.NewULID().String()
	position := count
	if req.Position != nil {
		position = *req.Position
	}

	var g channelGroup
	err := h.Pool.QueryRow(r.Context(),
		`INSERT INTO guild_channel_groups (id, guild_id, name, position, color, created_at)
		 VALUES ($1, $2, $3, $4, $5, now())
		 RETURNING id, guild_id, name, position, color, created_at`,
		id, guildID, req.Name, position, req.Color,
	).Scan(&g.ID, &g.GuildID, &g.Name, &g.Position, &g.Color, &g.CreatedAt)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to create channel group", err)
		return
	}

	g.Channels = []string{}

	h.Logger.Info("channel group created",
		slog.String("group_id", g.ID),
		slog.String("guild_id", guildID),
		slog.String("user_id", userID),
		slog.String("name", g.Name),
	)

	h.EventBus.PublishGuildEvent(r.Context(), events.SubjectChannelGroupCreate, "CHANNEL_GROUP_CREATE", guildID, g)

	apiutil.WriteJSON(w, http.StatusCreated, g)
}

// HandleUpdateChannelGroup updates an existing channel group.
// PATCH /api/v1/guilds/{guildID}/channel-groups/{groupID}
func (h *Handler) HandleUpdateChannelGroup(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	groupID := chi.URLParam(r, "groupID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageChannels) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "You need ManageChannels permission")
		return
	}

	var req updateChannelGroupRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	// Verify group belongs to this guild.
	var groupGuildID string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT guild_id FROM guild_channel_groups WHERE id = $1`, groupID,
	).Scan(&groupGuildID)
	if err == pgx.ErrNoRows {
		apiutil.WriteError(w, http.StatusNotFound, "group_not_found", "Channel group not found")
		return
	}
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to update channel group", err)
		return
	}
	if groupGuildID != guildID {
		apiutil.WriteError(w, http.StatusNotFound, "group_not_found", "Channel group not found")
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

	// Apply updates.
	if req.Name != nil {
		h.Pool.Exec(r.Context(), `UPDATE guild_channel_groups SET name = $2 WHERE id = $1`, groupID, *req.Name)
	}
	if req.Color != nil {
		h.Pool.Exec(r.Context(), `UPDATE guild_channel_groups SET color = $2 WHERE id = $1`, groupID, *req.Color)
	}
	if req.Position != nil {
		h.Pool.Exec(r.Context(), `UPDATE guild_channel_groups SET position = $2 WHERE id = $1`, groupID, *req.Position)
	}

	// Fetch updated group with channels.
	var g channelGroup
	err = h.Pool.QueryRow(r.Context(),
		`SELECT g.id, g.guild_id, g.name, g.position, g.color, g.created_at,
		        COALESCE(array_agg(gi.channel_id ORDER BY gi.position) FILTER (WHERE gi.channel_id IS NOT NULL), '{}')
		 FROM guild_channel_groups g
		 LEFT JOIN guild_channel_group_items gi ON gi.group_id = g.id
		 WHERE g.id = $1
		 GROUP BY g.id`,
		groupID,
	).Scan(&g.ID, &g.GuildID, &g.Name, &g.Position, &g.Color, &g.CreatedAt, &g.Channels)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to update channel group", err)
		return
	}

	h.EventBus.PublishGuildEvent(r.Context(), events.SubjectChannelGroupUpdate, "CHANNEL_GROUP_UPDATE", guildID, g)

	apiutil.WriteJSON(w, http.StatusOK, g)
}

// HandleDeleteChannelGroup removes a channel group and all its items.
// DELETE /api/v1/guilds/{guildID}/channel-groups/{groupID}
func (h *Handler) HandleDeleteChannelGroup(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	groupID := chi.URLParam(r, "groupID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageChannels) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "You need ManageChannels permission")
		return
	}

	// Verify group belongs to this guild.
	var groupGuildID string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT guild_id FROM guild_channel_groups WHERE id = $1`, groupID,
	).Scan(&groupGuildID)
	if err == pgx.ErrNoRows {
		apiutil.WriteError(w, http.StatusNotFound, "group_not_found", "Channel group not found")
		return
	}
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to delete channel group", err)
		return
	}
	if groupGuildID != guildID {
		apiutil.WriteError(w, http.StatusNotFound, "group_not_found", "Channel group not found")
		return
	}

	_, err = h.Pool.Exec(r.Context(),
		`DELETE FROM guild_channel_groups WHERE id = $1`, groupID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to delete channel group", err)
		return
	}

	h.Logger.Info("channel group deleted",
		slog.String("group_id", groupID),
		slog.String("guild_id", guildID),
		slog.String("user_id", userID),
	)

	h.EventBus.PublishGuildEvent(r.Context(), events.SubjectChannelGroupDelete, "CHANNEL_GROUP_DELETE", guildID, map[string]string{
		"id":       groupID,
		"guild_id": guildID,
	})

	w.WriteHeader(http.StatusNoContent)
}

// HandleSetGroupChannels replaces all channels in a group with the provided set.
// PUT /api/v1/guilds/{guildID}/channel-groups/{groupID}/channels
func (h *Handler) HandleSetGroupChannels(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	groupID := chi.URLParam(r, "groupID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageChannels) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "You need ManageChannels permission")
		return
	}

	var req setGroupChannelsRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	if len(req.ChannelIDs) > 100 {
		apiutil.WriteError(w, http.StatusBadRequest, "channel_limit", "A channel group can have at most 100 channels")
		return
	}

	// Verify group belongs to this guild.
	var groupGuildID string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT guild_id FROM guild_channel_groups WHERE id = $1`, groupID,
	).Scan(&groupGuildID)
	if err == pgx.ErrNoRows {
		apiutil.WriteError(w, http.StatusNotFound, "group_not_found", "Channel group not found")
		return
	}
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to set group channels", err)
		return
	}
	if groupGuildID != guildID {
		apiutil.WriteError(w, http.StatusNotFound, "group_not_found", "Channel group not found")
		return
	}

	// Replace all items in a transaction.
	if err := apiutil.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(r.Context(),
			`DELETE FROM guild_channel_group_items WHERE group_id = $1`, groupID); err != nil {
			return err
		}
		for i, chID := range req.ChannelIDs {
			if chID == "" {
				continue
			}
			if _, err := tx.Exec(r.Context(),
				`INSERT INTO guild_channel_group_items (group_id, channel_id, position) VALUES ($1, $2, $3)
				 ON CONFLICT (group_id, channel_id) DO UPDATE SET position = $3`,
				groupID, chID, i); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to set group channels", err)
		return
	}

	h.EventBus.PublishGuildEvent(r.Context(), events.SubjectChannelGroupItemsUpdate, "CHANNEL_GROUP_ITEMS_UPDATE", guildID, map[string]interface{}{
		"group_id":    groupID,
		"guild_id":    guildID,
		"channel_ids": req.ChannelIDs,
	})

	w.WriteHeader(http.StatusNoContent)
}

// HandleRemoveChannelFromGroup removes a channel from a guild channel group.
// DELETE /api/v1/guilds/{guildID}/channel-groups/{groupID}/channels/{channelID}
func (h *Handler) HandleRemoveChannelFromGroup(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	groupID := chi.URLParam(r, "groupID")
	channelID := chi.URLParam(r, "channelID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageChannels) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "You need ManageChannels permission")
		return
	}

	// Verify group belongs to this guild.
	var groupGuildID string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT guild_id FROM guild_channel_groups WHERE id = $1`, groupID,
	).Scan(&groupGuildID)
	if err == pgx.ErrNoRows {
		apiutil.WriteError(w, http.StatusNotFound, "group_not_found", "Channel group not found")
		return
	}
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to remove channel from group", err)
		return
	}
	if groupGuildID != guildID {
		apiutil.WriteError(w, http.StatusNotFound, "group_not_found", "Channel group not found")
		return
	}

	_, err = h.Pool.Exec(r.Context(),
		`DELETE FROM guild_channel_group_items WHERE group_id = $1 AND channel_id = $2`,
		groupID, channelID,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to remove channel from group", err)
		return
	}

	h.EventBus.PublishGuildEvent(r.Context(), events.SubjectChannelGroupItemsUpdate, "CHANNEL_GROUP_ITEMS_UPDATE", guildID, map[string]string{
		"group_id":   groupID,
		"guild_id":   guildID,
		"channel_id": channelID,
	})

	w.WriteHeader(http.StatusNoContent)
}
