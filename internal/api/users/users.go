// Package users implements REST API handlers for user operations including
// fetching user profiles, updating settings, managing relationships (friends,
// blocks), and DM creation. Mounted under /api/v1/users.
package users

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/amityvox/amityvox/internal/api/apiutil"
	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/models"
)

// sessionDisplayID returns a non-reversible display identifier for a session
// token. It uses the first 16 hex characters of the SHA-256 hash so the raw
// token is never exposed via the API.
func sessionDisplayID(rawToken string) string {
	h := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(h[:8]) // 8 bytes = 16 hex chars
}

// FederationDMNotifier is called when a DM is created with a remote user.
// The users handler calls this to notify the remote instance of the new DM.
// Parameters: ctx, remoteDomain, localChannelID, channelType, creatorID, recipientIDs, groupName.
type FederationDMNotifier func(ctx context.Context, remoteDomain, localChannelID, channelType, creatorID string, recipientIDs []string, groupName *string) error

// Handler implements user-related REST API endpoints.
type Handler struct {
	Pool           *pgxpool.Pool
	EventBus       *events.Bus
	InstanceID     string
	InstanceDomain string
	Logger         *slog.Logger
	NotifyFederatedDM FederationDMNotifier // optional — nil if federation disabled
}

// updateSelfRequest is the JSON body for PATCH /users/@me.
type updateSelfRequest struct {
	DisplayName     *string `json:"display_name"`
	AvatarID        *string `json:"avatar_id"`
	StatusText      *string `json:"status_text"`
	StatusEmoji     *string `json:"status_emoji"`
	StatusPresence  *string `json:"status_presence"`
	StatusExpiresAt *string `json:"status_expires_at"` // RFC3339 or empty to clear
	Bio             *string `json:"bio"`
	BannerID        *string `json:"banner_id"`
	AccentColor     *string `json:"accent_color"`
	Pronouns        *string `json:"pronouns"`
}

// HandleGetSelf returns the authenticated user's profile.
// GET /api/v1/users/@me
func (h *Handler) HandleGetSelf(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	user, err := h.getUser(r.Context(), userID)
	if err != nil {
		if err == pgx.ErrNoRows {
			apiutil.WriteError(w, http.StatusNotFound, "user_not_found", "User not found")
			return
		}
		apiutil.InternalError(w, h.Logger, "Failed to get user", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, user.ToSelf())
}

// HandleUpdateSelf updates the authenticated user's profile fields.
// PATCH /api/v1/users/@me
func (h *Handler) HandleUpdateSelf(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var req updateSelfRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	// Validate field lengths.
	if req.DisplayName != nil && len(*req.DisplayName) > 32 {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_display_name", "Display name must be at most 32 characters")
		return
	}
	if req.StatusText != nil && len(*req.StatusText) > 128 {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_status", "Status text must be at most 128 characters")
		return
	}
	if req.Bio != nil && len(*req.Bio) > 2000 {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_bio", "Bio must be at most 2000 characters")
		return
	}
	if req.StatusPresence != nil {
		valid := map[string]bool{"online": true, "idle": true, "focus": true, "busy": true, "dnd": true, "invisible": true, "offline": true}
		if !valid[*req.StatusPresence] {
			apiutil.WriteError(w, http.StatusBadRequest, "invalid_presence", "Invalid status presence value")
			return
		}
	}
	if req.Pronouns != nil && len(*req.Pronouns) > 40 {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_pronouns", "Pronouns must be at most 40 characters")
		return
	}
	if req.AccentColor != nil && len(*req.AccentColor) > 7 {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_accent_color", "Accent color must be a hex color (e.g. #FF5500)")
		return
	}

	// Parse status expiry if provided.
	var statusExpiresAt *time.Time
	if req.StatusExpiresAt != nil {
		if *req.StatusExpiresAt == "" {
			// Clear expiry.
			statusExpiresAt = nil
		} else {
			t, err := time.Parse(time.RFC3339, *req.StatusExpiresAt)
			if err != nil {
				apiutil.WriteError(w, http.StatusBadRequest, "invalid_status_expires", "status_expires_at must be RFC3339 format")
				return
			}
			statusExpiresAt = &t
		}
	}

	user, err := h.updateUser(r.Context(), userID, req, statusExpiresAt)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to update user", err)
		return
	}

	// Publish user data (email is excluded via json:"-" on the User struct).
	userData, _ := json.Marshal(user)
	h.EventBus.Publish(r.Context(), events.SubjectUserUpdate, events.Event{
		Type:   "USER_UPDATE",
		UserID: userID,
		Data:   userData,
	})

	apiutil.WriteJSON(w, http.StatusOK, user.ToSelf())
}

// HandleGetUser returns a user's public profile by ID.
// GET /api/v1/users/{userID}
func (h *Handler) HandleGetUser(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "userID")
	if !apiutil.RequireNonEmpty(w, "User ID", targetID) {
		return
	}

	user, err := h.getUser(r.Context(), targetID)
	if err != nil {
		if err == pgx.ErrNoRows {
			apiutil.WriteError(w, http.StatusNotFound, "user_not_found", "User not found")
			return
		}
		apiutil.InternalError(w, h.Logger, "Failed to get user", err)
		return
	}

	// Strip private fields for non-self lookups.
	user.Email = nil
	h.computeHandle(r.Context(), user)

	apiutil.WriteJSON(w, http.StatusOK, user)
}

// HandleGetSelfGuilds returns the guilds the authenticated user is a member of.
// GET /api/v1/users/@me/guilds
func (h *Handler) HandleGetSelfGuilds(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	rows, err := h.Pool.Query(r.Context(),
		`SELECT g.id, g.instance_id, COALESCE(i.domain, ''), g.owner_id, g.name, g.description, g.icon_id,
		        g.banner_id, g.default_permissions, g.flags, g.nsfw, g.discoverable,
		        g.preferred_locale, g.max_members, g.vanity_url,
		        g.verification_level, g.afk_channel_id, g.afk_timeout,
		        g.tags, g.member_count, g.created_at
		 FROM guilds g
		 JOIN guild_members gm ON g.id = gm.guild_id
		 LEFT JOIN instances i ON i.id = g.instance_id
		 WHERE gm.user_id = $1
		 ORDER BY g.name`,
		userID,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get guilds", err)
		return
	}
	defer rows.Close()

	guilds := make([]models.Guild, 0)
	for rows.Next() {
		var g models.Guild
		if err := rows.Scan(
			&g.ID, &g.InstanceID, &g.InstanceDomain, &g.OwnerID, &g.Name, &g.Description, &g.IconID,
			&g.BannerID, &g.DefaultPermissions, &g.Flags, &g.NSFW, &g.Discoverable,
			&g.PreferredLocale, &g.MaxMembers, &g.VanityURL,
			&g.VerificationLevel, &g.AFKChannelID, &g.AFKTimeout,
			&g.Tags, &g.MemberCount, &g.CreatedAt,
		); err != nil {
			apiutil.InternalError(w, h.Logger, "Failed to read guilds", err)
			return
		}
		guilds = append(guilds, g)
	}

	apiutil.WriteJSON(w, http.StatusOK, guilds)
}

// HandleGetSelfDMs returns the DM and group channels the authenticated user
// is a participant in.
// GET /api/v1/users/@me/dms
func (h *Handler) HandleGetSelfDMs(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	rows, err := h.Pool.Query(r.Context(),
		`SELECT c.id, c.guild_id, c.category_id, c.channel_type, c.name, c.topic,
		        c.position, c.slowmode_seconds, c.nsfw, c.encrypted, c.last_message_id,
		        c.owner_id, c.default_permissions, c.user_limit, c.bitrate,
		        c.locked, c.locked_by, c.locked_at, c.archived,
		        c.parent_channel_id, c.last_activity_at, c.created_at
		 FROM channels c
		 JOIN channel_recipients cr ON c.id = cr.channel_id
		 WHERE cr.user_id = $1 AND c.channel_type IN ('dm', 'group')
		 ORDER BY c.created_at DESC`,
		userID,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get DMs", err)
		return
	}
	defer rows.Close()

	channels := make([]models.Channel, 0)
	for rows.Next() {
		var c models.Channel
		if err := rows.Scan(
			&c.ID, &c.GuildID, &c.CategoryID, &c.ChannelType, &c.Name, &c.Topic,
			&c.Position, &c.SlowmodeSeconds, &c.NSFW, &c.Encrypted, &c.LastMessageID,
			&c.OwnerID, &c.DefaultPermissions, &c.UserLimit, &c.Bitrate,
			&c.Locked, &c.LockedBy, &c.LockedAt, &c.Archived,
			&c.ParentChannelID, &c.LastActivityAt, &c.CreatedAt,
		); err != nil {
			apiutil.InternalError(w, h.Logger, "Failed to read DMs", err)
			return
		}
		channels = append(channels, c)
	}

	// Batch-load recipients for all DM/group channels.
	channelIDs := make([]string, len(channels))
	for i, c := range channels {
		channelIDs[i] = c.ID
	}
	recipients, err := h.loadChannelRecipients(r.Context(), channelIDs)
	if err != nil {
		h.Logger.Error("failed to load DM recipients", slog.String("error", err.Error()))
	} else {
		for i := range channels {
			if r, ok := recipients[channels[i].ID]; ok {
				channels[i].Recipients = r
			}
		}
	}

	apiutil.WriteJSON(w, http.StatusOK, channels)
}

// HandleCreateDM creates a DM channel with another user or returns an existing one.
// POST /api/v1/users/{userID}/dm
func (h *Handler) HandleCreateDM(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	targetID := chi.URLParam(r, "userID")

	if !apiutil.RequireNonEmpty(w, "Target user ID", targetID) {
		return
	}
	if targetID == userID {
		apiutil.WriteError(w, http.StatusBadRequest, "self_dm", "Cannot create a DM with yourself")
		return
	}

	// Check if target user exists.
	var exists bool
	err := h.Pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`, targetID).Scan(&exists)
	if err != nil || !exists {
		apiutil.WriteError(w, http.StatusNotFound, "user_not_found", "Target user not found")
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
			apiutil.InternalError(w, h.Logger, "Failed to get DM", err)
			return
		}
		apiutil.WriteJSON(w, http.StatusOK, channel)
		return
	}

	// Create new DM channel.
	newID := models.NewULID().String()
	now := time.Now()

	err = apiutil.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(r.Context(),
			`INSERT INTO channels (id, channel_type, created_at) VALUES ($1, 'dm', $2)`,
			newID, now,
		); err != nil {
			return err
		}
		_, err := tx.Exec(r.Context(),
			`INSERT INTO channel_recipients (channel_id, user_id, joined_at) VALUES ($1, $2, $3), ($1, $4, $3)`,
			newID, userID, now, targetID,
		)
		return err
	})
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to create DM", err)
		return
	}

	channel, _ := h.getChannel(r.Context(), newID)

	h.EventBus.PublishUserEvent(r.Context(), events.SubjectChannelCreate, "CHANNEL_CREATE", targetID, channel)

	// If the target user is from a remote instance, notify their instance.
	if h.NotifyFederatedDM != nil {
		h.notifyRemoteInstancesAsync(r.Context(), newID, "dm", userID, []string{targetID}, nil)
	}

	apiutil.WriteJSON(w, http.StatusCreated, channel)
}

// HandleAddFriend sends or accepts a friend request.
// PUT /api/v1/users/{userID}/friend
func (h *Handler) HandleAddFriend(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	targetID := chi.URLParam(r, "userID")

	if targetID == "" || targetID == userID {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_target", "Invalid target user")
		return
	}

	// Check if blocked.
	var blocked bool
	_ = h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM user_relationships WHERE user_id = $1 AND target_id = $2 AND status = 'blocked')`,
		targetID, userID,
	).Scan(&blocked)
	if blocked {
		apiutil.WriteError(w, http.StatusForbidden, "blocked", "Cannot send friend request to this user")
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
			apiutil.WriteError(w, http.StatusConflict, "already_friends", "Already friends")
			return
		case models.RelationshipBlocked:
			apiutil.WriteError(w, http.StatusConflict, "blocked_user", "You have blocked this user")
			return
		case models.RelationshipPendingOutgoing:
			apiutil.WriteError(w, http.StatusConflict, "already_pending", "Friend request already sent")
			return
		case models.RelationshipPendingIncoming:
			// Accept the friend request — update both sides.
			if err := apiutil.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
				if _, err := tx.Exec(r.Context(),
					`UPDATE user_relationships SET status = 'friend' WHERE user_id = $1 AND target_id = $2`,
					userID, targetID); err != nil {
					return err
				}
				_, err := tx.Exec(r.Context(),
					`UPDATE user_relationships SET status = 'friend' WHERE user_id = $1 AND target_id = $2`,
					targetID, userID)
				return err
			}); err != nil {
				apiutil.InternalError(w, h.Logger, "Failed to accept friend request", err)
				return
			}

			// Emit RELATIONSHIP_UPDATE to both users.
			selfUser, _ := h.getUser(r.Context(), userID)
			targetUser, _ := h.getUser(r.Context(), targetID)
			selfRel, _ := json.Marshal(map[string]interface{}{
				"user_id": userID, "target_id": targetID, "type": models.RelationshipFriend, "user": targetUser,
			})
			targetRel, _ := json.Marshal(map[string]interface{}{
				"user_id": targetID, "target_id": userID, "type": models.RelationshipFriend, "user": selfUser,
			})
			h.EventBus.Publish(r.Context(), events.SubjectRelationshipUpdate, events.Event{
				Type: "RELATIONSHIP_UPDATE", UserID: userID, Data: selfRel,
			})
			h.EventBus.Publish(r.Context(), events.SubjectRelationshipUpdate, events.Event{
				Type: "RELATIONSHIP_UPDATE", UserID: targetID, Data: targetRel,
			})

			apiutil.WriteJSON(w, http.StatusOK, map[string]string{
				"user_id":   userID,
				"target_id": targetID,
				"status":    models.RelationshipFriend,
			})
			return
		}
	}

	// Create new friend request.
	err = apiutil.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(r.Context(),
			`INSERT INTO user_relationships (user_id, target_id, status, created_at)
			 VALUES ($1, $2, 'pending_outgoing', now())
			 ON CONFLICT (user_id, target_id) DO UPDATE SET status = 'pending_outgoing'`,
			userID, targetID); err != nil {
			return err
		}
		_, err := tx.Exec(r.Context(),
			`INSERT INTO user_relationships (user_id, target_id, status, created_at)
			 VALUES ($1, $2, 'pending_incoming', now())
			 ON CONFLICT (user_id, target_id) DO UPDATE SET status = 'pending_incoming'`,
			targetID, userID)
		return err
	})
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to send friend request", err)
		return
	}

	// Emit RELATIONSHIP_ADD to both users.
	selfUser, _ := h.getUser(r.Context(), userID)
	targetUser, _ := h.getUser(r.Context(), targetID)
	selfRel, _ := json.Marshal(map[string]interface{}{
		"user_id": userID, "target_id": targetID, "type": models.RelationshipPendingOutgoing, "user": targetUser,
	})
	targetRel, _ := json.Marshal(map[string]interface{}{
		"user_id": targetID, "target_id": userID, "type": models.RelationshipPendingIncoming, "user": selfUser,
	})
	h.EventBus.Publish(r.Context(), events.SubjectRelationshipAdd, events.Event{
		Type: "RELATIONSHIP_ADD", UserID: userID, Data: selfRel,
	})
	h.EventBus.Publish(r.Context(), events.SubjectRelationshipAdd, events.Event{
		Type: "RELATIONSHIP_ADD", UserID: targetID, Data: targetRel,
	})

	apiutil.WriteJSON(w, http.StatusOK, map[string]string{
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
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_target", "Invalid target user")
		return
	}

	err := apiutil.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(r.Context(),
			`DELETE FROM user_relationships WHERE user_id = $1 AND target_id = $2 AND status IN ('friend', 'pending_outgoing', 'pending_incoming')`,
			userID, targetID); err != nil {
			return err
		}
		_, err := tx.Exec(r.Context(),
			`DELETE FROM user_relationships WHERE user_id = $1 AND target_id = $2 AND status IN ('friend', 'pending_incoming', 'pending_outgoing')`,
			targetID, userID)
		return err
	})
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to remove friend", err)
		return
	}

	// Emit RELATIONSHIP_REMOVE to both users.
	selfRel, _ := json.Marshal(map[string]string{"user_id": userID, "target_id": targetID, "type": "none"})
	targetRel, _ := json.Marshal(map[string]string{"user_id": targetID, "target_id": userID, "type": "none"})
	h.EventBus.Publish(r.Context(), events.SubjectRelationshipRemove, events.Event{
		Type: "RELATIONSHIP_REMOVE", UserID: userID, Data: selfRel,
	})
	h.EventBus.Publish(r.Context(), events.SubjectRelationshipRemove, events.Event{
		Type: "RELATIONSHIP_REMOVE", UserID: targetID, Data: targetRel,
	})

	w.WriteHeader(http.StatusNoContent)
}

// blockUserRequest is the optional JSON body for PUT /api/v1/users/{userID}/block.
type blockUserRequest struct {
	Reason *string `json:"reason"`
	Level  *string `json:"level"` // "ignore" or "block" (default "block")
}

// updateBlockRequest is the JSON body for PATCH /api/v1/users/{userID}/block.
type updateBlockRequest struct {
	Level string `json:"level"` // "ignore" or "block"
}

// HandleBlockUser blocks another user. This removes any existing friendship or
// pending friend request, inserts into both user_relationships (for fast
// relationship checks) and user_blocks (for richer metadata), and emits a
// RELATIONSHIP_UPDATE event.
// PUT /api/v1/users/{userID}/block
func (h *Handler) HandleBlockUser(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	targetID := chi.URLParam(r, "userID")

	if targetID == "" || targetID == userID {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_target", "Cannot block yourself")
		return
	}

	// Validate that the target user exists.
	var exists bool
	if err := h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`, targetID,
	).Scan(&exists); err != nil || !exists {
		apiutil.WriteError(w, http.StatusNotFound, "user_not_found", "Target user not found")
		return
	}

	// Check if already blocked.
	var alreadyBlocked bool
	_ = h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM user_blocks WHERE user_id = $1 AND target_id = $2)`,
		userID, targetID,
	).Scan(&alreadyBlocked)
	if alreadyBlocked {
		apiutil.WriteError(w, http.StatusConflict, "already_blocked", "User is already blocked")
		return
	}

	// Parse optional reason and level from request body.
	var req blockUserRequest
	if r.Body != nil && r.ContentLength > 0 {
		if !apiutil.DecodeJSON(w, r, &req) {
			return
		}
		if req.Reason != nil && len(*req.Reason) > 256 {
			apiutil.WriteError(w, http.StatusBadRequest, "reason_too_long", "Block reason must be at most 256 characters")
			return
		}
		if req.Level != nil && *req.Level != "ignore" && *req.Level != "block" {
			apiutil.WriteError(w, http.StatusBadRequest, "invalid_level", "Block level must be 'ignore' or 'block'")
			return
		}
	}

	blockLevel := "block"
	if req.Level != nil {
		blockLevel = *req.Level
	}

	blockID := models.NewULID().String()
	now := time.Now()

	err := apiutil.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		// Remove any existing relationships from both sides (friend requests, friendships).
		if _, err := tx.Exec(r.Context(),
			`DELETE FROM user_relationships WHERE user_id = $1 AND target_id = $2`,
			userID, targetID); err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(),
			`DELETE FROM user_relationships WHERE user_id = $1 AND target_id = $2`,
			targetID, userID); err != nil {
			return err
		}
		// Insert blocked status into user_relationships for fast relationship checks.
		if _, err := tx.Exec(r.Context(),
			`INSERT INTO user_relationships (user_id, target_id, status, created_at)
			 VALUES ($1, $2, 'blocked', $3)`,
			userID, targetID, now); err != nil {
			return err
		}
		// Insert into user_blocks for richer metadata.
		_, err := tx.Exec(r.Context(),
			`INSERT INTO user_blocks (id, user_id, target_id, reason, level, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			blockID, userID, targetID, req.Reason, blockLevel, now)
		return err
	})
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to block user", err)
		return
	}

	result := map[string]interface{}{
		"id":        blockID,
		"user_id":   userID,
		"target_id": targetID,
		"status":    models.RelationshipBlocked,
		"level":     blockLevel,
	}
	if req.Reason != nil {
		result["reason"] = *req.Reason
	}

	h.EventBus.PublishUserEvent(r.Context(), events.SubjectRelationshipUpdate, "RELATIONSHIP_UPDATE", targetID, map[string]string{
		"user_id":   userID,
		"target_id": targetID,
		"status":    models.RelationshipBlocked,
		"level":     blockLevel,
	})

	apiutil.WriteJSON(w, http.StatusOK, result)
}

// HandleUnblockUser removes a block on another user. Cleans up both
// user_relationships and user_blocks, and emits a RELATIONSHIP_UPDATE event.
// DELETE /api/v1/users/{userID}/block
func (h *Handler) HandleUnblockUser(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	targetID := chi.URLParam(r, "userID")

	if targetID == "" || targetID == userID {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_target", "Invalid target user")
		return
	}

	var notBlocked bool
	err := apiutil.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		// Remove from user_relationships.
		result, err := tx.Exec(r.Context(),
			`DELETE FROM user_relationships WHERE user_id = $1 AND target_id = $2 AND status = 'blocked'`,
			userID, targetID)
		if err != nil {
			return err
		}
		if result.RowsAffected() == 0 {
			notBlocked = true
			return nil
		}
		// Remove from user_blocks.
		_, err = tx.Exec(r.Context(),
			`DELETE FROM user_blocks WHERE user_id = $1 AND target_id = $2`,
			userID, targetID)
		return err
	})
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to unblock user", err)
		return
	}
	if notBlocked {
		apiutil.WriteError(w, http.StatusNotFound, "not_blocked", "User is not blocked")
		return
	}

	h.EventBus.PublishUserEvent(r.Context(), events.SubjectRelationshipUpdate, "RELATIONSHIP_UPDATE", targetID, map[string]string{
		"user_id":   userID,
		"target_id": targetID,
		"status":    "none",
	})

	w.WriteHeader(http.StatusNoContent)
}

// HandleUpdateBlockLevel updates the block level for an existing block.
// PATCH /api/v1/users/{userID}/block
func (h *Handler) HandleUpdateBlockLevel(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	targetID := chi.URLParam(r, "userID")

	if targetID == "" || targetID == userID {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_target", "Invalid target user")
		return
	}

	var req updateBlockRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}
	if req.Level != "ignore" && req.Level != "block" {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_level", "Block level must be 'ignore' or 'block'")
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`UPDATE user_blocks SET level = $1 WHERE user_id = $2 AND target_id = $3`,
		req.Level, userID, targetID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to update block level", err)
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Block not found")
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]string{"level": req.Level})
}

// HandleGetBlockedUsers returns the list of users blocked by the authenticated
// user, including their profiles and the block reason/timestamp.
// GET /api/v1/users/@me/blocked
func (h *Handler) HandleGetBlockedUsers(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	rows, err := h.Pool.Query(r.Context(),
		`SELECT ub.id, ub.user_id, ub.target_id, ub.reason, ub.level, ub.created_at,
		        u.id, u.instance_id, u.username, u.display_name, u.avatar_id,
		        u.status_text, u.status_emoji, u.status_presence, u.status_expires_at,
		        u.bio, u.banner_id, u.accent_color, u.pronouns,
		        u.bot_owner_id, u.flags, u.created_at
		 FROM user_blocks ub
		 JOIN users u ON u.id = ub.target_id
		 WHERE ub.user_id = $1
		 ORDER BY ub.created_at DESC`,
		userID,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get blocked users", err)
		return
	}
	defer rows.Close()

	blocks := make([]models.UserBlock, 0)
	for rows.Next() {
		var b models.UserBlock
		var u models.User
		if err := rows.Scan(
			&b.ID, &b.UserID, &b.TargetID, &b.Reason, &b.Level, &b.CreatedAt,
			&u.ID, &u.InstanceID, &u.Username, &u.DisplayName, &u.AvatarID,
			&u.StatusText, &u.StatusEmoji, &u.StatusPresence, &u.StatusExpiresAt,
			&u.Bio, &u.BannerID, &u.AccentColor, &u.Pronouns,
			&u.BotOwnerID, &u.Flags, &u.CreatedAt,
		); err != nil {
			apiutil.InternalError(w, h.Logger, "Failed to read blocked users", err)
			return
		}
		b.User = &u
		blocks = append(blocks, b)
	}

	apiutil.WriteJSON(w, http.StatusOK, blocks)
}

// HandleGetSelfSessions lists all active sessions for the authenticated user.
// GET /api/v1/users/@me/sessions
func (h *Handler) HandleGetSelfSessions(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		apiutil.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, user_id, device_name, user_agent, created_at, last_active_at, expires_at
		 FROM user_sessions WHERE user_id = $1
		 ORDER BY last_active_at DESC`,
		userID,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get sessions", err)
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
			apiutil.InternalError(w, h.Logger, "Failed to read sessions", err)
			return
		}
		session := map[string]interface{}{
			"id":             sessionDisplayID(s.ID),
			"device_name":    s.DeviceName,
			"user_agent":     s.UserAgent,
			"created_at":     s.CreatedAt,
			"last_active_at": s.LastActiveAt,
			"expires_at":     s.ExpiresAt,
			"current":        s.ID == currentSessionID,
		}
		sessions = append(sessions, session)
	}

	apiutil.WriteJSON(w, http.StatusOK, sessions)
}

// HandleDeleteSelfSession revokes a specific session for the authenticated user.
// The sessionID URL parameter is the display ID (SHA-256 hash prefix), not the
// raw token. We iterate the user's sessions and match by computing the same hash.
// DELETE /api/v1/users/@me/sessions/{sessionID}
func (h *Handler) HandleDeleteSelfSession(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		apiutil.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	displayID := chi.URLParam(r, "sessionID")

	// Fetch all session IDs for this user and find the one whose hash prefix matches.
	rows, err := h.Pool.Query(r.Context(),
		`SELECT id FROM user_sessions WHERE user_id = $1`, userID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to delete session", err)
		return
	}
	defer rows.Close()

	var realSessionID string
	for rows.Next() {
		var rawID string
		if err := rows.Scan(&rawID); err != nil {
			continue
		}
		if sessionDisplayID(rawID) == displayID {
			realSessionID = rawID
			break
		}
	}
	rows.Close()

	if realSessionID == "" {
		apiutil.WriteError(w, http.StatusNotFound, "session_not_found", "Session not found")
		return
	}

	_, err = h.Pool.Exec(r.Context(),
		`DELETE FROM user_sessions WHERE id = $1 AND user_id = $2`, realSessionID, userID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to delete session", err)
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
		apiutil.InternalError(w, h.Logger, "Failed to get read state", err)
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
			apiutil.InternalError(w, h.Logger, "Failed to read state", err)
			return
		}
		states = append(states, rs)
	}

	apiutil.WriteJSON(w, http.StatusOK, states)
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
		apiutil.InternalError(w, h.Logger, "Failed to check guild ownership", err)
		return
	}
	if ownedCount > 0 {
		apiutil.WriteError(w, http.StatusBadRequest, "owns_guilds",
			"Transfer or delete all guilds you own before deleting your account")
		return
	}

	err := apiutil.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		// Soft-delete: set deleted flag, clear personal data.
		if _, err := tx.Exec(r.Context(),
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
		); err != nil {
			return err
		}

		// Remove from all guilds.
		if _, err := tx.Exec(r.Context(), `DELETE FROM guild_members WHERE user_id = $1`, userID); err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `DELETE FROM member_roles WHERE user_id = $1`, userID); err != nil {
			return err
		}

		// Remove all relationships and blocks.
		if _, err := tx.Exec(r.Context(), `DELETE FROM user_relationships WHERE user_id = $1 OR target_id = $1`, userID); err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `DELETE FROM user_blocks WHERE user_id = $1 OR target_id = $1`, userID); err != nil {
			return err
		}

		// Invalidate all sessions.
		_, err := tx.Exec(r.Context(), `DELETE FROM user_sessions WHERE user_id = $1`, userID)
		return err
	})
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to delete account", err)
		return
	}

	h.EventBus.PublishUserEvent(r.Context(), events.SubjectUserUpdate, "USER_UPDATE", userID, map[string]interface{}{
		"id":      userID,
		"deleted": true,
	})

	w.WriteHeader(http.StatusNoContent)
}

// HandleGetUserNote returns the authenticated user's personal note about another user.
// GET /api/v1/users/{userID}/note
func (h *Handler) HandleGetUserNote(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	targetID := chi.URLParam(r, "userID")

	var note string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT note FROM user_notes WHERE user_id = $1 AND target_id = $2`,
		userID, targetID,
	).Scan(&note)
	if err == pgx.ErrNoRows {
		apiutil.WriteJSON(w, http.StatusOK, map[string]string{
			"target_id": targetID,
			"note":      "",
		})
		return
	}
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get note", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]string{
		"target_id": targetID,
		"note":      note,
	})
}

// HandleSetUserNote sets a personal note about another user.
// PUT /api/v1/users/{userID}/note
func (h *Handler) HandleSetUserNote(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	targetID := chi.URLParam(r, "userID")

	var req struct {
		Note string `json:"note"`
	}
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	if len(req.Note) > 256 {
		apiutil.WriteError(w, http.StatusBadRequest, "note_too_long", "Note must be at most 256 characters")
		return
	}

	if req.Note == "" {
		h.Pool.Exec(r.Context(),
			`DELETE FROM user_notes WHERE user_id = $1 AND target_id = $2`,
			userID, targetID)
	} else {
		_, err := h.Pool.Exec(r.Context(),
			`INSERT INTO user_notes (user_id, target_id, note, updated_at)
			 VALUES ($1, $2, $3, now())
			 ON CONFLICT (user_id, target_id) DO UPDATE SET note = $3, updated_at = now()`,
			userID, targetID, req.Note)
		if err != nil {
			apiutil.InternalError(w, h.Logger, "Failed to save note", err)
			return
		}
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]string{
		"target_id": targetID,
		"note":      req.Note,
	})
}

// HandleGetUserSettings returns the authenticated user's client settings.
// GET /api/v1/users/@me/settings
func (h *Handler) HandleGetUserSettings(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var settings json.RawMessage
	err := h.Pool.QueryRow(r.Context(),
		`SELECT settings FROM user_settings WHERE user_id = $1`, userID,
	).Scan(&settings)
	if err == pgx.ErrNoRows {
		apiutil.WriteJSON(w, http.StatusOK, map[string]interface{}{})
		return
	}
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get settings", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{"data": settings})
}

// HandleUpdateUserSettings updates the authenticated user's client settings.
// PATCH /api/v1/users/@me/settings
func (h *Handler) HandleUpdateUserSettings(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var settings json.RawMessage
	if !apiutil.DecodeJSON(w, r, &settings) {
		return
	}

	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO user_settings (user_id, settings, updated_at)
		 VALUES ($1, $2, now())
		 ON CONFLICT (user_id) DO UPDATE SET settings = $2, updated_at = now()`,
		userID, settings)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to update settings", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{"data": settings})
}

// HandleGetMutualFriends returns mutual friends between the current user and a target.
// GET /api/v1/users/{userID}/mutual-friends
func (h *Handler) HandleGetMutualFriends(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	targetID := chi.URLParam(r, "userID")

	rows, err := h.Pool.Query(r.Context(),
		`SELECT u.id, u.instance_id, u.username, u.display_name, u.avatar_id,
		        u.status_text, u.status_emoji, u.status_presence, u.status_expires_at,
		        u.bio, u.banner_id, u.accent_color, u.pronouns,
		        u.bot_owner_id, u.flags, u.created_at
		 FROM users u
		 WHERE u.id IN (
			SELECT r1.target_id FROM user_relationships r1
			WHERE r1.user_id = $1 AND r1.status = 'friend'
			INTERSECT
			SELECT r2.target_id FROM user_relationships r2
			WHERE r2.user_id = $2 AND r2.status = 'friend'
		 )`,
		userID, targetID,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get mutual friends", err)
		return
	}
	defer rows.Close()

	friends := make([]models.User, 0)
	for rows.Next() {
		var u models.User
		if err := rows.Scan(
			&u.ID, &u.InstanceID, &u.Username, &u.DisplayName, &u.AvatarID,
			&u.StatusText, &u.StatusEmoji, &u.StatusPresence, &u.StatusExpiresAt,
			&u.Bio, &u.BannerID, &u.AccentColor, &u.Pronouns,
			&u.BotOwnerID, &u.Flags, &u.CreatedAt,
		); err != nil {
			apiutil.InternalError(w, h.Logger, "Failed to read mutual friends", err)
			return
		}
		friends = append(friends, u)
	}

	apiutil.WriteJSON(w, http.StatusOK, friends)
}

// HandleGetMutualGuilds returns guilds that both the current user and a target share.
// GET /api/v1/users/{userID}/mutual-guilds
func (h *Handler) HandleGetMutualGuilds(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	targetID := chi.URLParam(r, "userID")

	rows, err := h.Pool.Query(r.Context(),
		`SELECT g.id, g.name, g.icon_id
		 FROM guilds g
		 WHERE g.id IN (
			SELECT gm1.guild_id FROM guild_members gm1
			WHERE gm1.user_id = $1
			INTERSECT
			SELECT gm2.guild_id FROM guild_members gm2
			WHERE gm2.user_id = $2
		 )
		 ORDER BY g.name`,
		userID, targetID,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get mutual guilds", err)
		return
	}
	defer rows.Close()

	type mutualGuild struct {
		ID     string  `json:"id"`
		Name   string  `json:"name"`
		IconID *string `json:"icon_id,omitempty"`
	}

	guilds := make([]mutualGuild, 0)
	for rows.Next() {
		var g mutualGuild
		if err := rows.Scan(&g.ID, &g.Name, &g.IconID); err != nil {
			apiutil.InternalError(w, h.Logger, "Failed to read mutual guilds", err)
			return
		}
		guilds = append(guilds, g)
	}

	apiutil.WriteJSON(w, http.StatusOK, guilds)
}

// HandleGetRelationships returns all relationships (friends, pending, blocked)
// for the authenticated user.
// GET /api/v1/users/@me/relationships
func (h *Handler) HandleGetRelationships(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	rows, err := h.Pool.Query(r.Context(),
		`SELECT ur.user_id || ':' || ur.target_id, ur.user_id, ur.target_id, ur.status, ur.created_at,
		        u.id, u.instance_id, u.username, u.display_name, u.avatar_id,
		        u.status_text, u.status_emoji, u.status_presence, u.status_expires_at,
		        u.bio, u.banner_id, u.accent_color, u.pronouns,
		        u.bot_owner_id, u.flags, u.created_at,
		        i.domain
		 FROM user_relationships ur
		 JOIN users u ON u.id = ur.target_id
		 LEFT JOIN instances i ON i.id = u.instance_id
		 WHERE ur.user_id = $1
		 ORDER BY ur.created_at DESC`,
		userID,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get relationships", err)
		return
	}
	defer rows.Close()

	type relationshipResponse struct {
		ID        string       `json:"id"`
		UserID    string       `json:"user_id"`
		TargetID  string       `json:"target_id"`
		Status    string       `json:"type"`
		CreatedAt time.Time    `json:"created_at"`
		User      *models.User `json:"user,omitempty"`
	}

	relationships := make([]relationshipResponse, 0)
	for rows.Next() {
		var rel relationshipResponse
		var u models.User
		var instanceDomain *string
		if err := rows.Scan(
			&rel.ID, &rel.UserID, &rel.TargetID, &rel.Status, &rel.CreatedAt,
			&u.ID, &u.InstanceID, &u.Username, &u.DisplayName, &u.AvatarID,
			&u.StatusText, &u.StatusEmoji, &u.StatusPresence, &u.StatusExpiresAt,
			&u.Bio, &u.BannerID, &u.AccentColor, &u.Pronouns,
			&u.BotOwnerID, &u.Flags, &u.CreatedAt,
			&instanceDomain,
		); err != nil {
			apiutil.InternalError(w, h.Logger, "Failed to read relationships", err)
			return
		}
		// Compute handle inline to avoid N+1 queries.
		if u.InstanceID == h.InstanceID || instanceDomain == nil || *instanceDomain == "" {
			u.Handle = "@" + u.Username
		} else {
			u.Handle = "@" + u.Username + "@" + *instanceDomain
		}
		rel.User = &u
		relationships = append(relationships, rel)
	}
	if err := rows.Err(); err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to read relationships", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, relationships)
}

// --- Internal helpers ---

// computeHandle sets the Handle field on a User based on whether the user is
// local or remote. Local users get @username, remote users get @username@domain.
// WARNING: This issues a DB query per call. Do not call in a loop — use a JOIN
// to batch-resolve instance domains when handling multiple users (see HandleGetRelationships).
func (h *Handler) computeHandle(ctx context.Context, user *models.User) {
	if user.InstanceID == h.InstanceID {
		user.Handle = "@" + user.Username
		return
	}
	// Remote user — look up the instance domain.
	var domain string
	err := h.Pool.QueryRow(ctx,
		`SELECT domain FROM instances WHERE id = $1`, user.InstanceID).Scan(&domain)
	if err != nil {
		user.Handle = "@" + user.Username
		return
	}
	user.Handle = "@" + user.Username + "@" + domain
}

// loadChannelRecipients batch-loads the recipients for a set of DM/group channels.
// Returns a map of channel ID → slice of User.
func (h *Handler) loadChannelRecipients(ctx context.Context, channelIDs []string) (map[string][]models.User, error) {
	result := make(map[string][]models.User)
	if len(channelIDs) == 0 {
		return result, nil
	}

	rows, err := h.Pool.Query(ctx,
		`SELECT cr.channel_id, u.id, u.instance_id, u.username, u.display_name, u.avatar_id,
		        u.status_text, u.status_emoji, u.status_presence, u.status_expires_at,
		        u.bio, u.banner_id, u.accent_color, u.pronouns,
		        u.bot_owner_id, u.flags, u.created_at
		 FROM channel_recipients cr
		 JOIN users u ON u.id = cr.user_id
		 WHERE cr.channel_id = ANY($1)`,
		channelIDs,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var channelID string
		var u models.User
		if err := rows.Scan(
			&channelID,
			&u.ID, &u.InstanceID, &u.Username, &u.DisplayName, &u.AvatarID,
			&u.StatusText, &u.StatusEmoji, &u.StatusPresence, &u.StatusExpiresAt,
			&u.Bio, &u.BannerID, &u.AccentColor, &u.Pronouns,
			&u.BotOwnerID, &u.Flags, &u.CreatedAt,
		); err != nil {
			return nil, err
		}
		result[channelID] = append(result[channelID], u)
	}

	return result, nil
}

func (h *Handler) getUser(ctx context.Context, userID string) (*models.User, error) {
	var user models.User
	err := h.Pool.QueryRow(ctx,
		`SELECT id, instance_id, username, display_name, avatar_id, status_text,
		        status_emoji, status_presence, status_expires_at, bio,
		        banner_id, accent_color, pronouns,
		        bot_owner_id, email, flags, last_online, created_at
		 FROM users WHERE id = $1`,
		userID,
	).Scan(
		&user.ID, &user.InstanceID, &user.Username, &user.DisplayName,
		&user.AvatarID, &user.StatusText, &user.StatusEmoji, &user.StatusPresence,
		&user.StatusExpiresAt, &user.Bio, &user.BannerID, &user.AccentColor,
		&user.Pronouns, &user.BotOwnerID, &user.Email, &user.Flags, &user.LastOnline, &user.CreatedAt,
	)
	return &user, err
}

func (h *Handler) updateUser(ctx context.Context, userID string, req updateSelfRequest, statusExpiresAt *time.Time) (*models.User, error) {
	var user models.User
	err := h.Pool.QueryRow(ctx,
		`UPDATE users SET
			display_name = COALESCE($2, display_name),
			avatar_id = COALESCE($3, avatar_id),
			status_text = COALESCE($4, status_text),
			bio = COALESCE($5, bio),
			status_emoji = COALESCE($6, status_emoji),
			status_presence = COALESCE($7, status_presence),
			status_expires_at = COALESCE($8, status_expires_at),
			banner_id = COALESCE($9, banner_id),
			accent_color = COALESCE($10, accent_color),
			pronouns = COALESCE($11, pronouns)
		 WHERE id = $1
		 RETURNING id, instance_id, username, display_name, avatar_id, status_text,
		           status_emoji, status_presence, status_expires_at, bio,
		           banner_id, accent_color, pronouns,
		           bot_owner_id, email, flags, last_online, created_at`,
		userID, req.DisplayName, req.AvatarID, req.StatusText, req.Bio,
		req.StatusEmoji, req.StatusPresence, statusExpiresAt,
		req.BannerID, req.AccentColor, req.Pronouns,
	).Scan(
		&user.ID, &user.InstanceID, &user.Username, &user.DisplayName,
		&user.AvatarID, &user.StatusText, &user.StatusEmoji, &user.StatusPresence,
		&user.StatusExpiresAt, &user.Bio, &user.BannerID, &user.AccentColor,
		&user.Pronouns, &user.BotOwnerID, &user.Email, &user.Flags, &user.LastOnline, &user.CreatedAt,
	)
	return &user, err
}

func (h *Handler) getChannel(ctx context.Context, channelID string) (*models.Channel, error) {
	var c models.Channel
	err := h.Pool.QueryRow(ctx,
		`SELECT id, guild_id, category_id, channel_type, name, topic, position,
		        slowmode_seconds, nsfw, encrypted, last_message_id, owner_id,
		        default_permissions, user_limit, bitrate, locked, locked_by, locked_at, archived, created_at
		 FROM channels WHERE id = $1`,
		channelID,
	).Scan(
		&c.ID, &c.GuildID, &c.CategoryID, &c.ChannelType, &c.Name, &c.Topic,
		&c.Position, &c.SlowmodeSeconds, &c.NSFW, &c.Encrypted, &c.LastMessageID,
		&c.OwnerID, &c.DefaultPermissions, &c.UserLimit, &c.Bitrate,
		&c.Locked, &c.LockedBy, &c.LockedAt, &c.Archived, &c.CreatedAt,
	)
	if err != nil {
		return &c, err
	}

	// Load recipients for DM/group channels.
	if c.ChannelType == models.ChannelTypeDM || c.ChannelType == models.ChannelTypeGroup {
		recipients, err := h.loadChannelRecipients(ctx, []string{channelID})
		if err == nil {
			c.Recipients = recipients[channelID]
		}
	}

	return &c, nil
}

// writeJSON and writeError match the api package envelope format.
// --- Profile Links ---

// HandleGetUserLinks returns the public profile links for a user.
// GET /api/v1/users/{userID}/links
func (h *Handler) HandleGetUserLinks(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "userID")
	links, err := h.getUserLinks(r.Context(), targetID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get user links", err)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, links)
}

// HandleGetMyLinks returns the authenticated user's own links.
// GET /api/v1/users/@me/links
func (h *Handler) HandleGetMyLinks(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	links, err := h.getUserLinks(r.Context(), userID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get links", err)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, links)
}

// HandleCreateLink adds a profile link for the authenticated user.
// POST /api/v1/users/@me/links
func (h *Handler) HandleCreateLink(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var req struct {
		Platform string `json:"platform"`
		Label    string `json:"label"`
		URL      string `json:"url"`
	}
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}
	if req.Platform == "" || req.Label == "" || req.URL == "" {
		apiutil.WriteError(w, http.StatusBadRequest, "missing_fields", "platform, label, and url are required")
		return
	}
	if !isValidLinkURL(req.URL) {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_url", "URL must use http or https scheme")
		return
	}

	id := models.NewULID().String()
	var link models.UserLink
	err := h.Pool.QueryRow(r.Context(),
		`INSERT INTO user_links (id, user_id, platform, label, url, position)
		 VALUES ($1, $2, $3, $4, $5, COALESCE((SELECT MAX(position) + 1 FROM user_links WHERE user_id = $2), 0))
		 RETURNING id, user_id, platform, label, url, position, verified, created_at`,
		id, userID, req.Platform, req.Label, req.URL,
	).Scan(&link.ID, &link.UserID, &link.Platform, &link.Label, &link.URL, &link.Position, &link.Verified, &link.CreatedAt)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to create link", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusCreated, link)
}

// HandleUpdateLink updates a profile link.
// PATCH /api/v1/users/@me/links/{linkID}
func (h *Handler) HandleUpdateLink(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	linkID := chi.URLParam(r, "linkID")

	var req struct {
		Platform *string `json:"platform"`
		Label    *string `json:"label"`
		URL      *string `json:"url"`
		Position *int    `json:"position"`
	}
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	if req.URL != nil && !isValidLinkURL(*req.URL) {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_url", "URL must use http or https scheme")
		return
	}

	var link models.UserLink
	err := h.Pool.QueryRow(r.Context(),
		`UPDATE user_links SET
			platform = COALESCE($3, platform),
			label = COALESCE($4, label),
			url = COALESCE($5, url),
			position = COALESCE($6, position)
		 WHERE id = $1 AND user_id = $2
		 RETURNING id, user_id, platform, label, url, position, verified, created_at`,
		linkID, userID, req.Platform, req.Label, req.URL, req.Position,
	).Scan(&link.ID, &link.UserID, &link.Platform, &link.Label, &link.URL, &link.Position, &link.Verified, &link.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			apiutil.WriteError(w, http.StatusNotFound, "link_not_found", "Link not found")
			return
		}
		apiutil.InternalError(w, h.Logger, "Failed to update link", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, link)
}

// HandleDeleteLink removes a profile link.
// DELETE /api/v1/users/@me/links/{linkID}
func (h *Handler) HandleDeleteLink(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	linkID := chi.URLParam(r, "linkID")

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM user_links WHERE id = $1 AND user_id = $2`, linkID, userID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to delete link", err)
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "link_not_found", "Link not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) getUserLinks(ctx context.Context, userID string) ([]models.UserLink, error) {
	rows, err := h.Pool.Query(ctx,
		`SELECT id, user_id, platform, label, url, position, verified, created_at
		 FROM user_links WHERE user_id = $1 ORDER BY position`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	links := make([]models.UserLink, 0)
	for rows.Next() {
		var l models.UserLink
		if err := rows.Scan(&l.ID, &l.UserID, &l.Platform, &l.Label, &l.URL, &l.Position, &l.Verified, &l.CreatedAt); err != nil {
			return nil, err
		}
		links = append(links, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return links, nil
}

// isValidLinkURL checks that a URL uses http or https scheme.
func isValidLinkURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	return scheme == "http" || scheme == "https"
}

// createGroupDMRequest is the JSON body for POST /users/@me/group-dms.
type createGroupDMRequest struct {
	UserIDs []string `json:"user_ids"`
	Name    *string  `json:"name"`
}

// HandleCreateGroupDM creates a group DM channel with multiple users.
// POST /api/v1/users/@me/group-dms
func (h *Handler) HandleCreateGroupDM(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var req createGroupDMRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	// Validate recipient count: 2-9 other users (plus self makes 3-10 total).
	if len(req.UserIDs) < 2 || len(req.UserIDs) > 9 {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_user_count", "Group DMs require between 2 and 9 other users")
		return
	}

	// Validate name length if provided.
	if req.Name != nil && len(*req.Name) > 100 {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_name", "Group DM name must be at most 100 characters")
		return
	}

	// Ensure self is not included in user_ids.
	for _, uid := range req.UserIDs {
		if uid == userID {
			apiutil.WriteError(w, http.StatusBadRequest, "self_included", "Do not include yourself in user_ids; you are added automatically")
			return
		}
	}

	// Deduplicate user IDs.
	seen := make(map[string]bool)
	unique := make([]string, 0, len(req.UserIDs))
	for _, uid := range req.UserIDs {
		if !seen[uid] {
			seen[uid] = true
			unique = append(unique, uid)
		}
	}
	req.UserIDs = unique

	// Re-validate after dedup: still need 2-9 unique recipients.
	if len(req.UserIDs) < 2 || len(req.UserIDs) > 9 {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_recipients", "Group DMs require 2-9 other users (after deduplication)")
		return
	}

	// Verify all target users exist.
	var existCount int
	err := h.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM users WHERE id = ANY($1)`, req.UserIDs,
	).Scan(&existCount)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to create group DM", err)
		return
	}
	if existCount != len(req.UserIDs) {
		apiutil.WriteError(w, http.StatusBadRequest, "user_not_found", "One or more users not found")
		return
	}

	// Create the group DM channel in a transaction.
	newID := models.NewULID().String()
	now := time.Now()

	err = apiutil.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(r.Context(),
			`INSERT INTO channels (id, channel_type, name, owner_id, created_at) VALUES ($1, 'group', $2, $3, $4)`,
			newID, req.Name, userID, now,
		); err != nil {
			return err
		}
		// Insert self as the first recipient.
		if _, err := tx.Exec(r.Context(),
			`INSERT INTO channel_recipients (channel_id, user_id, joined_at) VALUES ($1, $2, $3)`,
			newID, userID, now,
		); err != nil {
			return err
		}
		// Insert all other recipients.
		for _, uid := range req.UserIDs {
			if _, err := tx.Exec(r.Context(),
				`INSERT INTO channel_recipients (channel_id, user_id, joined_at) VALUES ($1, $2, $3)`,
				newID, uid, now,
			); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to create group DM", err)
		return
	}

	channel, _ := h.getChannel(r.Context(), newID)

	for _, uid := range req.UserIDs {
		h.EventBus.PublishUserEvent(r.Context(), events.SubjectChannelCreate, "CHANNEL_CREATE", uid, channel)
	}

	// Notify remote instances of any remote recipients.
	if h.NotifyFederatedDM != nil {
		h.notifyRemoteInstancesAsync(r.Context(), newID, "group", userID, req.UserIDs, req.Name)
	}

	apiutil.WriteJSON(w, http.StatusCreated, channel)
}

// notifyRemoteInstancesAsync queries for remote instances among the given userIDs
// and asynchronously notifies each one about a new DM/group-DM channel.
func (h *Handler) notifyRemoteInstancesAsync(ctx context.Context, channelID, channelType, creatorID string, userIDs []string, groupName *string) {
	rows, err := h.Pool.Query(ctx,
		`SELECT DISTINCT i.domain FROM users u
		 JOIN instances i ON i.id = u.instance_id
		 WHERE u.id = ANY($1) AND u.instance_id <> $2`,
		userIDs, h.InstanceID,
	)
	if err != nil {
		h.Logger.Warn("failed to query remote instances for federation DM",
			slog.String("channel_id", channelID),
			slog.String("error", err.Error()),
		)
		return
	}
	defer rows.Close()

	var domains []string
	for rows.Next() {
		var domain string
		if err := rows.Scan(&domain); err != nil {
			h.Logger.Warn("failed to scan remote instance domain",
				slog.String("error", err.Error()),
			)
			continue
		}
		domains = append(domains, domain)
	}
	if err := rows.Err(); err != nil {
		h.Logger.Warn("error iterating remote instances",
			slog.String("error", err.Error()),
		)
	}

	for _, domain := range domains {
		domain := domain
		go func() {
			if err := h.NotifyFederatedDM(context.Background(), domain, channelID, channelType, creatorID, userIDs, groupName); err != nil {
				h.Logger.Warn("failed to notify remote instance of DM",
					slog.String("domain", domain),
					slog.String("error", err.Error()),
				)
			}
		}()
	}
}

// HandleUpdateGuildPositions replaces the user's guild ordering.
// PUT /api/v1/users/@me/guild-positions
func (h *Handler) HandleUpdateGuildPositions(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var positions []struct {
		GuildID  string `json:"guild_id"`
		Position int    `json:"position"`
	}
	if !apiutil.DecodeJSON(w, r, &positions) {
		return
	}

	if len(positions) > 200 {
		apiutil.WriteError(w, http.StatusBadRequest, "too_many", "Too many guild positions")
		return
	}

	if err := apiutil.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(r.Context(),
			`DELETE FROM user_guild_positions WHERE user_id = $1`, userID); err != nil {
			return err
		}
		for _, p := range positions {
			if p.GuildID == "" {
				continue
			}
			if _, err := tx.Exec(r.Context(),
				`INSERT INTO user_guild_positions (user_id, guild_id, position)
				 VALUES ($1, $2, $3)
				 ON CONFLICT (user_id, guild_id) DO UPDATE SET position = $3`,
				userID, p.GuildID, p.Position); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to update guild positions", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
