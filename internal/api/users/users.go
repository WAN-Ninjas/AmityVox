// Package users implements REST API handlers for user operations including
// fetching user profiles, updating settings, managing relationships (friends,
// blocks), and DM creation. Mounted under /api/v1/users.
package users

import (
	"context"
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

// Handler implements user-related REST API endpoints.
type Handler struct {
	Pool     *pgxpool.Pool
	EventBus *events.Bus
	Logger   *slog.Logger
}

// updateSelfRequest is the JSON body for PATCH /users/@me.
type updateSelfRequest struct {
	DisplayName *string `json:"display_name"`
	AvatarID    *string `json:"avatar_id"`
	StatusText  *string `json:"status_text"`
	Bio         *string `json:"bio"`
}

// HandleGetSelf returns the authenticated user's profile.
// GET /api/v1/users/@me
func (h *Handler) HandleGetSelf(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	user, err := h.getUser(r.Context(), userID)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "user_not_found", "User not found")
			return
		}
		h.Logger.Error("failed to get user", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get user")
		return
	}

	writeJSON(w, http.StatusOK, user)
}

// HandleUpdateSelf updates the authenticated user's profile fields.
// PATCH /api/v1/users/@me
func (h *Handler) HandleUpdateSelf(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var req updateSelfRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	user, err := h.updateUser(r.Context(), userID, req)
	if err != nil {
		h.Logger.Error("failed to update user", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update user")
		return
	}

	h.EventBus.PublishJSON(r.Context(), events.SubjectUserUpdate, "USER_UPDATE", user)

	writeJSON(w, http.StatusOK, user)
}

// HandleGetUser returns a user's public profile by ID.
// GET /api/v1/users/{userID}
func (h *Handler) HandleGetUser(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "userID")
	if targetID == "" {
		writeError(w, http.StatusBadRequest, "missing_user_id", "User ID is required")
		return
	}

	user, err := h.getUser(r.Context(), targetID)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "user_not_found", "User not found")
			return
		}
		h.Logger.Error("failed to get user", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get user")
		return
	}

	// Strip private fields for non-self lookups.
	user.Email = nil

	writeJSON(w, http.StatusOK, user)
}

// HandleGetSelfGuilds returns the guilds the authenticated user is a member of.
// GET /api/v1/users/@me/guilds
func (h *Handler) HandleGetSelfGuilds(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	rows, err := h.Pool.Query(r.Context(),
		`SELECT g.id, g.instance_id, g.owner_id, g.name, g.description, g.icon_id,
		        g.banner_id, g.default_permissions, g.flags, g.nsfw, g.discoverable,
		        g.preferred_locale, g.max_members,
		        (SELECT COUNT(*) FROM guild_members gm2 WHERE gm2.guild_id = g.id),
		        g.created_at
		 FROM guilds g
		 JOIN guild_members gm ON g.id = gm.guild_id
		 WHERE gm.user_id = $1
		 ORDER BY g.name`,
		userID,
	)
	if err != nil {
		h.Logger.Error("failed to get guilds", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get guilds")
		return
	}
	defer rows.Close()

	guilds := make([]models.Guild, 0)
	for rows.Next() {
		var g models.Guild
		if err := rows.Scan(
			&g.ID, &g.InstanceID, &g.OwnerID, &g.Name, &g.Description, &g.IconID,
			&g.BannerID, &g.DefaultPermissions, &g.Flags, &g.NSFW, &g.Discoverable,
			&g.PreferredLocale, &g.MaxMembers, &g.MemberCount, &g.CreatedAt,
		); err != nil {
			h.Logger.Error("failed to scan guild", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read guilds")
			return
		}
		guilds = append(guilds, g)
	}

	writeJSON(w, http.StatusOK, guilds)
}

// HandleGetSelfDMs returns the DM and group channels the authenticated user
// is a participant in.
// GET /api/v1/users/@me/dms
func (h *Handler) HandleGetSelfDMs(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	rows, err := h.Pool.Query(r.Context(),
		`SELECT c.id, c.guild_id, c.category_id, c.channel_type, c.name, c.topic,
		        c.position, c.slowmode_seconds, c.nsfw, c.encrypted, c.last_message_id,
		        c.owner_id, c.default_permissions, c.created_at
		 FROM channels c
		 JOIN channel_recipients cr ON c.id = cr.channel_id
		 WHERE cr.user_id = $1 AND c.channel_type IN ('dm', 'group')
		 ORDER BY c.created_at DESC`,
		userID,
	)
	if err != nil {
		h.Logger.Error("failed to get DMs", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get DMs")
		return
	}
	defer rows.Close()

	channels := make([]models.Channel, 0)
	for rows.Next() {
		var c models.Channel
		if err := rows.Scan(
			&c.ID, &c.GuildID, &c.CategoryID, &c.ChannelType, &c.Name, &c.Topic,
			&c.Position, &c.SlowmodeSeconds, &c.NSFW, &c.Encrypted, &c.LastMessageID,
			&c.OwnerID, &c.DefaultPermissions, &c.CreatedAt,
		); err != nil {
			h.Logger.Error("failed to scan channel", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read DMs")
			return
		}
		channels = append(channels, c)
	}

	writeJSON(w, http.StatusOK, channels)
}

// HandleCreateDM creates a DM channel with another user or returns an existing one.
// POST /api/v1/users/{userID}/dm
func (h *Handler) HandleCreateDM(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	targetID := chi.URLParam(r, "userID")

	if targetID == "" {
		writeError(w, http.StatusBadRequest, "missing_user_id", "Target user ID is required")
		return
	}
	if targetID == userID {
		writeError(w, http.StatusBadRequest, "self_dm", "Cannot create a DM with yourself")
		return
	}

	// Check if target user exists.
	var exists bool
	err := h.Pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`, targetID).Scan(&exists)
	if err != nil || !exists {
		writeError(w, http.StatusNotFound, "user_not_found", "Target user not found")
		return
	}

	// Check for existing DM.
	var channelID string
	err = h.Pool.QueryRow(r.Context(),
		`SELECT c.id FROM channels c
		 JOIN channel_recipients cr1 ON c.id = cr1.channel_id AND cr1.user_id = $1
		 JOIN channel_recipients cr2 ON c.id = cr2.channel_id AND cr2.user_id = $2
		 WHERE c.channel_type = 'dm'
		 LIMIT 1`,
		userID, targetID,
	).Scan(&channelID)

	if err == nil {
		// Existing DM found — return it.
		channel, err := h.getChannel(r.Context(), channelID)
		if err != nil {
			h.Logger.Error("failed to get existing DM", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get DM")
			return
		}
		writeJSON(w, http.StatusOK, channel)
		return
	}

	// Create new DM channel.
	newID := models.NewULID().String()
	now := time.Now()

	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		h.Logger.Error("failed to begin tx", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create DM")
		return
	}
	defer tx.Rollback(r.Context())

	_, err = tx.Exec(r.Context(),
		`INSERT INTO channels (id, channel_type, created_at) VALUES ($1, 'dm', $2)`,
		newID, now,
	)
	if err != nil {
		h.Logger.Error("failed to create DM channel", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create DM")
		return
	}

	_, err = tx.Exec(r.Context(),
		`INSERT INTO channel_recipients (channel_id, user_id, joined_at) VALUES ($1, $2, $3), ($1, $4, $3)`,
		newID, userID, now, targetID,
	)
	if err != nil {
		h.Logger.Error("failed to add DM recipients", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create DM")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		h.Logger.Error("failed to commit DM", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create DM")
		return
	}

	channel, _ := h.getChannel(r.Context(), newID)

	h.EventBus.PublishJSON(r.Context(), events.SubjectChannelCreate, "CHANNEL_CREATE", channel)

	writeJSON(w, http.StatusCreated, channel)
}

// HandleAddFriend sends or accepts a friend request.
// PUT /api/v1/users/{userID}/friend
func (h *Handler) HandleAddFriend(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	targetID := chi.URLParam(r, "userID")

	if targetID == "" || targetID == userID {
		writeError(w, http.StatusBadRequest, "invalid_target", "Invalid target user")
		return
	}

	// Check if blocked.
	var blocked bool
	_ = h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM user_relationships WHERE user_id = $1 AND target_id = $2 AND status = 'blocked')`,
		targetID, userID,
	).Scan(&blocked)
	if blocked {
		writeError(w, http.StatusForbidden, "blocked", "Cannot send friend request to this user")
		return
	}

	// Check for existing pending incoming (means we should accept).
	var existingStatus string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT status FROM user_relationships WHERE user_id = $1 AND target_id = $2`,
		userID, targetID,
	).Scan(&existingStatus)

	if err == nil {
		switch existingStatus {
		case models.RelationshipFriend:
			writeError(w, http.StatusConflict, "already_friends", "Already friends")
			return
		case models.RelationshipBlocked:
			writeError(w, http.StatusConflict, "blocked_user", "You have blocked this user")
			return
		case models.RelationshipPendingOutgoing:
			writeError(w, http.StatusConflict, "already_pending", "Friend request already sent")
			return
		case models.RelationshipPendingIncoming:
			// Accept the friend request — update both sides.
			tx, err := h.Pool.Begin(r.Context())
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", "Failed to accept request")
				return
			}
			defer tx.Rollback(r.Context())

			tx.Exec(r.Context(),
				`UPDATE user_relationships SET status = 'friend' WHERE user_id = $1 AND target_id = $2`,
				userID, targetID)
			tx.Exec(r.Context(),
				`UPDATE user_relationships SET status = 'friend' WHERE user_id = $1 AND target_id = $2`,
				targetID, userID)
			tx.Commit(r.Context())

			writeJSON(w, http.StatusOK, map[string]string{
				"user_id":   userID,
				"target_id": targetID,
				"status":    models.RelationshipFriend,
			})
			return
		}
	}

	// Create new friend request.
	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to send friend request")
		return
	}
	defer tx.Rollback(r.Context())

	tx.Exec(r.Context(),
		`INSERT INTO user_relationships (user_id, target_id, status, created_at)
		 VALUES ($1, $2, 'pending_outgoing', now())
		 ON CONFLICT (user_id, target_id) DO UPDATE SET status = 'pending_outgoing'`,
		userID, targetID)
	tx.Exec(r.Context(),
		`INSERT INTO user_relationships (user_id, target_id, status, created_at)
		 VALUES ($1, $2, 'pending_incoming', now())
		 ON CONFLICT (user_id, target_id) DO UPDATE SET status = 'pending_incoming'`,
		targetID, userID)

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to send friend request")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"user_id":   userID,
		"target_id": targetID,
		"status":    models.RelationshipPendingOutgoing,
	})
}

// HandleRemoveFriend removes a friend or cancels a pending request.
// DELETE /api/v1/users/{userID}/friend
func (h *Handler) HandleRemoveFriend(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	targetID := chi.URLParam(r, "userID")

	if targetID == "" || targetID == userID {
		writeError(w, http.StatusBadRequest, "invalid_target", "Invalid target user")
		return
	}

	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to remove friend")
		return
	}
	defer tx.Rollback(r.Context())

	tx.Exec(r.Context(),
		`DELETE FROM user_relationships WHERE user_id = $1 AND target_id = $2 AND status IN ('friend', 'pending_outgoing', 'pending_incoming')`,
		userID, targetID)
	tx.Exec(r.Context(),
		`DELETE FROM user_relationships WHERE user_id = $1 AND target_id = $2 AND status IN ('friend', 'pending_incoming', 'pending_outgoing')`,
		targetID, userID)

	tx.Commit(r.Context())

	w.WriteHeader(http.StatusNoContent)
}

// HandleBlockUser blocks another user.
// PUT /api/v1/users/{userID}/block
func (h *Handler) HandleBlockUser(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	targetID := chi.URLParam(r, "userID")

	if targetID == "" || targetID == userID {
		writeError(w, http.StatusBadRequest, "invalid_target", "Invalid target user")
		return
	}

	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to block user")
		return
	}
	defer tx.Rollback(r.Context())

	// Remove any existing relationship from both sides.
	tx.Exec(r.Context(), `DELETE FROM user_relationships WHERE user_id = $1 AND target_id = $2`, userID, targetID)
	tx.Exec(r.Context(), `DELETE FROM user_relationships WHERE user_id = $1 AND target_id = $2`, targetID, userID)

	// Create block.
	tx.Exec(r.Context(),
		`INSERT INTO user_relationships (user_id, target_id, status, created_at) VALUES ($1, $2, 'blocked', now())`,
		userID, targetID)

	tx.Commit(r.Context())

	writeJSON(w, http.StatusOK, map[string]string{
		"user_id":   userID,
		"target_id": targetID,
		"status":    models.RelationshipBlocked,
	})
}

// HandleUnblockUser removes a block on another user.
// DELETE /api/v1/users/{userID}/block
func (h *Handler) HandleUnblockUser(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	targetID := chi.URLParam(r, "userID")

	if targetID == "" || targetID == userID {
		writeError(w, http.StatusBadRequest, "invalid_target", "Invalid target user")
		return
	}

	_, err := h.Pool.Exec(r.Context(),
		`DELETE FROM user_relationships WHERE user_id = $1 AND target_id = $2 AND status = 'blocked'`,
		userID, targetID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to unblock user")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleGetSelfSessions lists all active sessions for the authenticated user.
// GET /api/v1/users/@me/sessions
func (h *Handler) HandleGetSelfSessions(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, user_id, device_name, user_agent, created_at, last_active_at, expires_at
		 FROM user_sessions WHERE user_id = $1
		 ORDER BY last_active_at DESC`,
		userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get sessions")
		return
	}
	defer rows.Close()

	sessions := make([]map[string]interface{}, 0)
	currentSessionID := auth.SessionIDFromContext(r.Context())
	for rows.Next() {
		var s models.UserSession
		if err := rows.Scan(
			&s.ID, &s.UserID, &s.DeviceName,
			&s.UserAgent, &s.CreatedAt, &s.LastActiveAt, &s.ExpiresAt,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read sessions")
			return
		}
		session := map[string]interface{}{
			"id":             s.ID,
			"device_name":    s.DeviceName,
			"user_agent":     s.UserAgent,
			"created_at":     s.CreatedAt,
			"last_active_at": s.LastActiveAt,
			"expires_at":     s.ExpiresAt,
			"current":        s.ID == currentSessionID,
		}
		sessions = append(sessions, session)
	}

	writeJSON(w, http.StatusOK, sessions)
}

// HandleDeleteSelfSession revokes a specific session for the authenticated user.
// DELETE /api/v1/users/@me/sessions/{sessionID}
func (h *Handler) HandleDeleteSelfSession(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	sessionID := chi.URLParam(r, "sessionID")

	result, err := h.Pool.Exec(r.Context(),
		`DELETE FROM user_sessions WHERE id = $1 AND user_id = $2`, sessionID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete session")
		return
	}

	if result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "session_not_found", "Session not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleGetSelfReadState returns the unread state for all channels the user has.
// GET /api/v1/users/@me/read-state
func (h *Handler) HandleGetSelfReadState(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	rows, err := h.Pool.Query(r.Context(),
		`SELECT rs.channel_id, rs.last_read_id, rs.mention_count
		 FROM read_state rs
		 WHERE rs.user_id = $1`,
		userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get read state")
		return
	}
	defer rows.Close()

	type readState struct {
		ChannelID    string  `json:"channel_id"`
		LastReadID   *string `json:"last_read_id"`
		MentionCount int     `json:"mention_count"`
	}

	states := make([]readState, 0)
	for rows.Next() {
		var rs readState
		if err := rows.Scan(&rs.ChannelID, &rs.LastReadID, &rs.MentionCount); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read state")
			return
		}
		states = append(states, rs)
	}

	writeJSON(w, http.StatusOK, states)
}

// HandleDeleteSelf soft-deletes the authenticated user's account.
// The account is flagged as deleted and personal data is cleared.
// DELETE /api/v1/users/@me
func (h *Handler) HandleDeleteSelf(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	// Check that user is not the owner of any guild.
	var ownedCount int
	if err := h.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM guilds WHERE owner_id = $1`, userID,
	).Scan(&ownedCount); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to check guild ownership")
		return
	}
	if ownedCount > 0 {
		writeError(w, http.StatusBadRequest, "owns_guilds",
			"Transfer or delete all guilds you own before deleting your account")
		return
	}

	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete account")
		return
	}
	defer tx.Rollback(r.Context())

	// Soft-delete: set deleted flag, clear personal data.
	_, err = tx.Exec(r.Context(),
		`UPDATE users SET
			flags = flags | $2,
			display_name = NULL,
			avatar_id = NULL,
			status_text = NULL,
			bio = NULL,
			email = NULL,
			password_hash = NULL
		 WHERE id = $1`,
		userID, models.UserFlagDeleted,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete account")
		return
	}

	// Remove from all guilds.
	tx.Exec(r.Context(), `DELETE FROM guild_members WHERE user_id = $1`, userID)
	tx.Exec(r.Context(), `DELETE FROM member_roles WHERE user_id = $1`, userID)

	// Remove all relationships.
	tx.Exec(r.Context(), `DELETE FROM user_relationships WHERE user_id = $1 OR target_id = $1`, userID)

	// Invalidate all sessions.
	tx.Exec(r.Context(), `DELETE FROM user_sessions WHERE user_id = $1`, userID)

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete account")
		return
	}

	h.EventBus.PublishJSON(r.Context(), events.SubjectUserUpdate, "USER_UPDATE", map[string]interface{}{
		"id":      userID,
		"deleted": true,
	})

	w.WriteHeader(http.StatusNoContent)
}

// --- Internal helpers ---

func (h *Handler) getUser(ctx context.Context, userID string) (*models.User, error) {
	var user models.User
	err := h.Pool.QueryRow(ctx,
		`SELECT id, instance_id, username, display_name, avatar_id, status_text,
		        status_presence, bio, bot_owner_id, email, flags, created_at
		 FROM users WHERE id = $1`,
		userID,
	).Scan(
		&user.ID, &user.InstanceID, &user.Username, &user.DisplayName,
		&user.AvatarID, &user.StatusText, &user.StatusPresence, &user.Bio,
		&user.BotOwnerID, &user.Email, &user.Flags, &user.CreatedAt,
	)
	return &user, err
}

func (h *Handler) updateUser(ctx context.Context, userID string, req updateSelfRequest) (*models.User, error) {
	var user models.User
	err := h.Pool.QueryRow(ctx,
		`UPDATE users SET
			display_name = COALESCE($2, display_name),
			avatar_id = COALESCE($3, avatar_id),
			status_text = COALESCE($4, status_text),
			bio = COALESCE($5, bio)
		 WHERE id = $1
		 RETURNING id, instance_id, username, display_name, avatar_id, status_text,
		           status_presence, bio, bot_owner_id, email, flags, created_at`,
		userID, req.DisplayName, req.AvatarID, req.StatusText, req.Bio,
	).Scan(
		&user.ID, &user.InstanceID, &user.Username, &user.DisplayName,
		&user.AvatarID, &user.StatusText, &user.StatusPresence, &user.Bio,
		&user.BotOwnerID, &user.Email, &user.Flags, &user.CreatedAt,
	)
	return &user, err
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

// writeJSON and writeError match the api package envelope format.
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
