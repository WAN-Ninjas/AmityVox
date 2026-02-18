// Package moderation implements REST API handlers for guild moderation operations
// including member warnings, message reports, channel locking, and raid protection
// configuration. Mounted under /api/v1/guilds and /api/v1/channels.
package moderation

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/amityvox/amityvox/internal/api/apiutil"
	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/models"
)

// Handler implements moderation-related REST API endpoints.
type Handler struct {
	Pool     *pgxpool.Pool
	EventBus *events.Bus
	Logger   *slog.Logger
}

// --- Request types ---

type warnMemberRequest struct {
	Reason string `json:"reason"`
}

type reportMessageRequest struct {
	Reason string `json:"reason"`
}

type resolveReportRequest struct {
	Status string `json:"status"`
}

type updateRaidConfigRequest struct {
	Enabled        *bool `json:"enabled"`
	JoinRateLimit  *int  `json:"join_rate_limit"`
	JoinRateWindow *int  `json:"join_rate_window"`
	MinAccountAge  *int  `json:"min_account_age"`
	LockdownActive *bool `json:"lockdown_active"`
}

// --- Response types ---

type warningResponse struct {
	ID          string         `json:"id"`
	GuildID     string         `json:"guild_id"`
	UserID      string         `json:"user_id"`
	ModeratorID string         `json:"moderator_id"`
	Reason      string         `json:"reason"`
	CreatedAt   time.Time      `json:"created_at"`
	Moderator   *moderatorInfo `json:"moderator,omitempty"`
}

type moderatorInfo struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName *string `json:"display_name,omitempty"`
}

type channelLockResponse struct {
	ID       string     `json:"id"`
	Locked   bool       `json:"locked"`
	LockedBy *string    `json:"locked_by,omitempty"`
	LockedAt *time.Time `json:"locked_at,omitempty"`
}

// --- Permission helper ---

// isGuildAdmin checks whether the user is the guild owner or an instance admin.
func (h *Handler) isGuildAdmin(ctx context.Context, guildID, userID string) bool {
	// Check if guild owner.
	var ownerID string
	if err := h.Pool.QueryRow(ctx, `SELECT owner_id FROM guilds WHERE id = $1`, guildID).Scan(&ownerID); err == nil && ownerID == userID {
		return true
	}
	// Check if instance admin (flags & 4).
	var flags int
	h.Pool.QueryRow(ctx, `SELECT flags FROM users WHERE id = $1`, userID).Scan(&flags)
	return flags&models.UserFlagAdmin != 0
}

// getGuildIDForChannel looks up the guild_id for a given channel. Returns an
// empty string if the channel is not found or has no guild.
func (h *Handler) getGuildIDForChannel(ctx context.Context, channelID string) (string, error) {
	var guildID *string
	err := h.Pool.QueryRow(ctx, `SELECT guild_id FROM channels WHERE id = $1`, channelID).Scan(&guildID)
	if err != nil {
		return "", err
	}
	if guildID == nil {
		return "", pgx.ErrNoRows
	}
	return *guildID, nil
}

// --- Handlers ---

// HandleWarnMember issues a warning to a guild member.
// POST /api/v1/guilds/{guildID}/members/{memberID}/warn
func (h *Handler) HandleWarnMember(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	memberID := chi.URLParam(r, "memberID")

	if !h.isGuildAdmin(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You must be the guild owner or an instance admin")
		return
	}

	var req warnMemberRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	if !apiutil.RequireNonEmpty(w, "Reason", req.Reason) {
		return
	}

	// Verify the target user is a member of the guild.
	var exists bool
	err := h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM guild_members WHERE guild_id = $1 AND user_id = $2)`,
		guildID, memberID,
	).Scan(&exists)
	if err != nil || !exists {
		apiutil.WriteError(w, http.StatusNotFound, "member_not_found", "Member not found in this guild")
		return
	}

	warningID := models.NewULID().String()
	var warning models.MemberWarning
	err = h.Pool.QueryRow(r.Context(),
		`INSERT INTO member_warnings (id, guild_id, user_id, moderator_id, reason, created_at)
		 VALUES ($1, $2, $3, $4, $5, now())
		 RETURNING id, guild_id, user_id, moderator_id, reason, created_at`,
		warningID, guildID, memberID, userID, req.Reason,
	).Scan(
		&warning.ID, &warning.GuildID, &warning.UserID,
		&warning.ModeratorID, &warning.Reason, &warning.CreatedAt,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to create warning", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusCreated, warning)
}

// HandleGetWarnings lists warnings for a guild member.
// GET /api/v1/guilds/{guildID}/members/{memberID}/warnings
func (h *Handler) HandleGetWarnings(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	memberID := chi.URLParam(r, "memberID")

	if !h.isGuildAdmin(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You must be the guild owner or an instance admin")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT w.id, w.guild_id, w.user_id, w.moderator_id, w.reason, w.created_at,
		        u.id, u.username, u.display_name
		 FROM member_warnings w
		 JOIN users u ON u.id = w.moderator_id
		 WHERE w.guild_id = $1 AND w.user_id = $2
		 ORDER BY w.created_at DESC`,
		guildID, memberID,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to list warnings", err)
		return
	}
	defer rows.Close()

	warnings := make([]warningResponse, 0)
	for rows.Next() {
		var wr warningResponse
		var mod moderatorInfo
		if err := rows.Scan(
			&wr.ID, &wr.GuildID, &wr.UserID, &wr.ModeratorID, &wr.Reason, &wr.CreatedAt,
			&mod.ID, &mod.Username, &mod.DisplayName,
		); err != nil {
			apiutil.InternalError(w, h.Logger, "Failed to list warnings", err)
			return
		}
		wr.Moderator = &mod
		warnings = append(warnings, wr)
	}
	if err := rows.Err(); err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to list warnings", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, warnings)
}

// HandleDeleteWarning deletes a specific warning.
// DELETE /api/v1/guilds/{guildID}/warnings/{warningID}
func (h *Handler) HandleDeleteWarning(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	warningID := chi.URLParam(r, "warningID")

	if !h.isGuildAdmin(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You must be the guild owner or an instance admin")
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM member_warnings WHERE id = $1 AND guild_id = $2`,
		warningID, guildID,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to delete warning", err)
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "warning_not_found", "Warning not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleReportMessage creates a report on a message.
// POST /api/v1/channels/{channelID}/messages/{messageID}/report
func (h *Handler) HandleReportMessage(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")

	var req reportMessageRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	if !apiutil.RequireNonEmpty(w, "Reason", req.Reason) {
		return
	}

	// Look up guild_id from the channel.
	guildID, err := h.getGuildIDForChannel(r.Context(), channelID)
	if err != nil {
		if err == pgx.ErrNoRows {
			apiutil.WriteError(w, http.StatusNotFound, "channel_not_found", "Channel not found or not in a guild")
			return
		}
		apiutil.InternalError(w, h.Logger, "Failed to report message", err)
		return
	}

	// Verify the message exists in the channel.
	var msgExists bool
	err = h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM messages WHERE id = $1 AND channel_id = $2)`,
		messageID, channelID,
	).Scan(&msgExists)
	if err != nil || !msgExists {
		apiutil.WriteError(w, http.StatusNotFound, "message_not_found", "Message not found in this channel")
		return
	}

	reportID := models.NewULID().String()
	var report models.MessageReport
	err = h.Pool.QueryRow(r.Context(),
		`INSERT INTO message_reports (id, guild_id, channel_id, message_id, reporter_id, reason, status, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, 'open', now())
		 RETURNING id, guild_id, channel_id, message_id, reporter_id, reason, status, resolved_by, resolved_at, created_at`,
		reportID, guildID, channelID, messageID, userID, req.Reason,
	).Scan(
		&report.ID, &report.GuildID, &report.ChannelID, &report.MessageID,
		&report.ReporterID, &report.Reason, &report.Status,
		&report.ResolvedBy, &report.ResolvedAt, &report.CreatedAt,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to create report", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusCreated, report)
}

// HandleGetReports lists message reports for a guild.
// GET /api/v1/guilds/{guildID}/reports?status=open
func (h *Handler) HandleGetReports(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isGuildAdmin(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You must be the guild owner or an instance admin")
		return
	}

	status := r.URL.Query().Get("status")

	var rows pgx.Rows
	var err error
	if status != "" {
		rows, err = h.Pool.Query(r.Context(),
			`SELECT id, guild_id, channel_id, message_id, reporter_id, reason, status,
			        resolved_by, resolved_at, created_at
			 FROM message_reports
			 WHERE guild_id = $1 AND status = $2
			 ORDER BY created_at DESC
			 LIMIT 50`,
			guildID, status,
		)
	} else {
		rows, err = h.Pool.Query(r.Context(),
			`SELECT id, guild_id, channel_id, message_id, reporter_id, reason, status,
			        resolved_by, resolved_at, created_at
			 FROM message_reports
			 WHERE guild_id = $1
			 ORDER BY created_at DESC
			 LIMIT 50`,
			guildID,
		)
	}
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to list reports", err)
		return
	}
	defer rows.Close()

	reports := make([]models.MessageReport, 0)
	for rows.Next() {
		var report models.MessageReport
		if err := rows.Scan(
			&report.ID, &report.GuildID, &report.ChannelID, &report.MessageID,
			&report.ReporterID, &report.Reason, &report.Status,
			&report.ResolvedBy, &report.ResolvedAt, &report.CreatedAt,
		); err != nil {
			apiutil.InternalError(w, h.Logger, "Failed to list reports", err)
			return
		}
		reports = append(reports, report)
	}
	if err := rows.Err(); err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to list reports", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, reports)
}

// HandleResolveReport updates a report's status to resolved or dismissed.
// PATCH /api/v1/guilds/{guildID}/reports/{reportID}
func (h *Handler) HandleResolveReport(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	reportID := chi.URLParam(r, "reportID")

	if !h.isGuildAdmin(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You must be the guild owner or an instance admin")
		return
	}

	var req resolveReportRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	if req.Status != "resolved" && req.Status != "dismissed" {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_status", "Status must be 'resolved' or 'dismissed'")
		return
	}

	var report models.MessageReport
	err := h.Pool.QueryRow(r.Context(),
		`UPDATE message_reports
		 SET status = $3, resolved_by = $4, resolved_at = now()
		 WHERE id = $1 AND guild_id = $2
		 RETURNING id, guild_id, channel_id, message_id, reporter_id, reason, status,
		           resolved_by, resolved_at, created_at`,
		reportID, guildID, req.Status, userID,
	).Scan(
		&report.ID, &report.GuildID, &report.ChannelID, &report.MessageID,
		&report.ReporterID, &report.Reason, &report.Status,
		&report.ResolvedBy, &report.ResolvedAt, &report.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			apiutil.WriteError(w, http.StatusNotFound, "report_not_found", "Report not found")
			return
		}
		apiutil.InternalError(w, h.Logger, "Failed to resolve report", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, report)
}

// HandleLockChannel locks a channel, preventing non-admin messages.
// POST /api/v1/channels/{channelID}/lock
func (h *Handler) HandleLockChannel(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	guildID, err := h.getGuildIDForChannel(r.Context(), channelID)
	if err != nil {
		if err == pgx.ErrNoRows {
			apiutil.WriteError(w, http.StatusNotFound, "channel_not_found", "Channel not found or not in a guild")
			return
		}
		apiutil.InternalError(w, h.Logger, "Failed to lock channel", err)
		return
	}

	if !h.isGuildAdmin(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You must be the guild owner or an instance admin")
		return
	}

	var resp channelLockResponse
	err = h.Pool.QueryRow(r.Context(),
		`UPDATE channels
		 SET locked = true, locked_by = $2, locked_at = now()
		 WHERE id = $1
		 RETURNING id, locked, locked_by, locked_at`,
		channelID, userID,
	).Scan(&resp.ID, &resp.Locked, &resp.LockedBy, &resp.LockedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			apiutil.WriteError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
			return
		}
		apiutil.InternalError(w, h.Logger, "Failed to lock channel", err)
		return
	}

	h.EventBus.PublishChannelEvent(r.Context(), events.SubjectChannelUpdate, "CHANNEL_UPDATE", channelID, resp)

	apiutil.WriteJSON(w, http.StatusOK, resp)
}

// HandleUnlockChannel unlocks a previously locked channel.
// POST /api/v1/channels/{channelID}/unlock
func (h *Handler) HandleUnlockChannel(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	guildID, err := h.getGuildIDForChannel(r.Context(), channelID)
	if err != nil {
		if err == pgx.ErrNoRows {
			apiutil.WriteError(w, http.StatusNotFound, "channel_not_found", "Channel not found or not in a guild")
			return
		}
		apiutil.InternalError(w, h.Logger, "Failed to unlock channel", err)
		return
	}

	if !h.isGuildAdmin(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You must be the guild owner or an instance admin")
		return
	}

	var resp channelLockResponse
	err = h.Pool.QueryRow(r.Context(),
		`UPDATE channels
		 SET locked = false, locked_by = NULL, locked_at = NULL
		 WHERE id = $1
		 RETURNING id, locked, locked_by, locked_at`,
		channelID,
	).Scan(&resp.ID, &resp.Locked, &resp.LockedBy, &resp.LockedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			apiutil.WriteError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
			return
		}
		apiutil.InternalError(w, h.Logger, "Failed to unlock channel", err)
		return
	}

	h.EventBus.PublishChannelEvent(r.Context(), events.SubjectChannelUpdate, "CHANNEL_UPDATE", channelID, resp)

	apiutil.WriteJSON(w, http.StatusOK, resp)
}

// HandleGetRaidConfig returns the raid protection configuration for a guild.
// GET /api/v1/guilds/{guildID}/raid-config
func (h *Handler) HandleGetRaidConfig(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isGuildAdmin(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You must be the guild owner or an instance admin")
		return
	}

	var config models.GuildRaidConfig
	err := h.Pool.QueryRow(r.Context(),
		`SELECT guild_id, enabled, join_rate_limit, join_rate_window, min_account_age,
		        lockdown_active, lockdown_started_at, updated_at
		 FROM guild_raid_config
		 WHERE guild_id = $1`,
		guildID,
	).Scan(
		&config.GuildID, &config.Enabled, &config.JoinRateLimit, &config.JoinRateWindow,
		&config.MinAccountAge, &config.LockdownActive, &config.LockdownStartedAt, &config.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			// Upsert a default config if none exists.
			err = h.Pool.QueryRow(r.Context(),
				`INSERT INTO guild_raid_config (guild_id, enabled, join_rate_limit, join_rate_window,
				             min_account_age, lockdown_active, updated_at)
				 VALUES ($1, false, 10, 60, 0, false, now())
				 ON CONFLICT (guild_id) DO NOTHING
				 RETURNING guild_id, enabled, join_rate_limit, join_rate_window, min_account_age,
				           lockdown_active, lockdown_started_at, updated_at`,
				guildID,
			).Scan(
				&config.GuildID, &config.Enabled, &config.JoinRateLimit, &config.JoinRateWindow,
				&config.MinAccountAge, &config.LockdownActive, &config.LockdownStartedAt, &config.UpdatedAt,
			)
			if err != nil {
				apiutil.InternalError(w, h.Logger, "Failed to get raid config", err)
				return
			}
		} else {
			apiutil.InternalError(w, h.Logger, "Failed to get raid config", err)
			return
		}
	}

	apiutil.WriteJSON(w, http.StatusOK, config)
}

// HandleUpdateRaidConfig updates the raid protection configuration for a guild.
// PATCH /api/v1/guilds/{guildID}/raid-config
func (h *Handler) HandleUpdateRaidConfig(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isGuildAdmin(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You must be the guild owner or an instance admin")
		return
	}

	var req updateRaidConfigRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	// Check the previous lockdown state before updating, to detect transitions.
	var prevLockdownActive bool
	_ = h.Pool.QueryRow(r.Context(),
		`SELECT COALESCE(lockdown_active, false) FROM guild_raid_config WHERE guild_id = $1`, guildID,
	).Scan(&prevLockdownActive)

	var config models.GuildRaidConfig
	err := h.Pool.QueryRow(r.Context(),
		`INSERT INTO guild_raid_config (guild_id, enabled, join_rate_limit, join_rate_window,
		             min_account_age, lockdown_active, updated_at)
		 VALUES ($1, COALESCE($2, false), COALESCE($3, 10), COALESCE($4, 60),
		         COALESCE($5, 0), COALESCE($6, false), now())
		 ON CONFLICT (guild_id) DO UPDATE SET
		     enabled = COALESCE($2, guild_raid_config.enabled),
		     join_rate_limit = COALESCE($3, guild_raid_config.join_rate_limit),
		     join_rate_window = COALESCE($4, guild_raid_config.join_rate_window),
		     min_account_age = COALESCE($5, guild_raid_config.min_account_age),
		     lockdown_active = COALESCE($6, guild_raid_config.lockdown_active),
		     lockdown_started_at = CASE
		         WHEN COALESCE($6, guild_raid_config.lockdown_active) = true
		              AND guild_raid_config.lockdown_active = false THEN now()
		         WHEN COALESCE($6, guild_raid_config.lockdown_active) = false THEN NULL
		         ELSE guild_raid_config.lockdown_started_at
		     END,
		     updated_at = now()
		 RETURNING guild_id, enabled, join_rate_limit, join_rate_window, min_account_age,
		           lockdown_active, lockdown_started_at, updated_at`,
		guildID, req.Enabled, req.JoinRateLimit, req.JoinRateWindow,
		req.MinAccountAge, req.LockdownActive,
	).Scan(
		&config.GuildID, &config.Enabled, &config.JoinRateLimit, &config.JoinRateWindow,
		&config.MinAccountAge, &config.LockdownActive, &config.LockdownStartedAt, &config.UpdatedAt,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to update raid config", err)
		return
	}

	// If lockdown just activated, create a system message and publish an event.
	if config.LockdownActive && !prevLockdownActive {
		h.publishLockdownAlert(r.Context(), guildID, config)
	}

	apiutil.WriteJSON(w, http.StatusOK, config)
}

// publishLockdownAlert creates a system_lockdown message in the guild's first
// text channel and publishes a NATS event so connected clients see the alert
// in real-time.
func (h *Handler) publishLockdownAlert(ctx context.Context, guildID string, config models.GuildRaidConfig) {
	// Find the guild's first text channel to post the alert in.
	var channelID string
	err := h.Pool.QueryRow(ctx,
		`SELECT id FROM channels
		 WHERE guild_id = $1 AND channel_type = 'text'
		 ORDER BY position ASC, created_at ASC
		 LIMIT 1`,
		guildID,
	).Scan(&channelID)
	if err != nil {
		h.Logger.Warn("no text channel found for lockdown alert",
			slog.String("guild_id", guildID),
			slog.String("error", err.Error()),
		)
		return
	}

	// Build the alert message content.
	alertContent := fmt.Sprintf(
		"Raid protection activated: lockdown engaged (rate limit: %d joins in %d seconds)",
		config.JoinRateLimit, config.JoinRateWindow,
	)

	// Insert the system message.
	msgID := models.NewULID().String()
	var msg models.Message
	err = h.Pool.QueryRow(ctx,
		`INSERT INTO messages (id, channel_id, author_id, content, message_type, flags, created_at)
		 VALUES ($1, $2, '00000000000000000000000000', $3, $4, 0, now())
		 RETURNING id, channel_id, author_id, content, nonce, message_type, edited_at, flags,
		           reply_to_ids, mention_user_ids, mention_role_ids, mention_here,
		           thread_id, masquerade_name, masquerade_avatar, masquerade_color,
		           encrypted, encryption_session_id, created_at`,
		msgID, channelID, alertContent, models.MessageTypeSystemLockdown,
	).Scan(
		&msg.ID, &msg.ChannelID, &msg.AuthorID, &msg.Content, &msg.Nonce, &msg.MessageType,
		&msg.EditedAt, &msg.Flags, &msg.ReplyToIDs, &msg.MentionUserIDs, &msg.MentionRoleIDs,
		&msg.MentionHere, &msg.ThreadID, &msg.MasqueradeName, &msg.MasqueradeAvatar,
		&msg.MasqueradeColor, &msg.Encrypted, &msg.EncryptionSessionID, &msg.CreatedAt,
	)
	if err != nil {
		h.Logger.Error("failed to create lockdown alert message",
			slog.String("guild_id", guildID),
			slog.String("error", err.Error()),
		)
		return
	}

	// Update last_message_id on the channel.
	h.Pool.Exec(ctx,
		`UPDATE channels SET last_message_id = $1 WHERE id = $2`, msgID, channelID)

	// Publish MESSAGE_CREATE event so connected clients see it in real-time.
	msgData, _ := json.Marshal(msg)
	h.EventBus.Publish(ctx, events.SubjectMessageCreate, events.Event{
		Type:      "MESSAGE_CREATE",
		ChannelID: channelID,
		GuildID:   guildID,
		Data:      msgData,
	})

	h.Logger.Info("lockdown alert published",
		slog.String("guild_id", guildID),
		slog.String("channel_id", channelID),
		slog.String("message_id", msgID),
	)
}

// HandleReportToAdmin escalates a message report to instance admins.
// POST /api/v1/channels/{channelID}/messages/{messageID}/report-admin
func (h *Handler) HandleReportToAdmin(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")

	var req reportMessageRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	if !apiutil.RequireNonEmpty(w, "Reason", req.Reason) {
		return
	}

	// Look up guild_id from the channel (may be empty for DMs).
	guildID, _ := h.getGuildIDForChannel(r.Context(), channelID)

	// Verify the message exists.
	var msgAuthorID string
	var msgContent *string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT author_id, content FROM messages WHERE id = $1 AND channel_id = $2`,
		messageID, channelID,
	).Scan(&msgAuthorID, &msgContent)
	if err != nil {
		apiutil.WriteError(w, http.StatusNotFound, "message_not_found", "Message not found in this channel")
		return
	}

	reportID := models.NewULID().String()

	// Store the report in message_reports with a special "admin" target.
	var report models.MessageReport
	err = h.Pool.QueryRow(r.Context(),
		`INSERT INTO message_reports (id, guild_id, channel_id, message_id, reporter_id, reason, status, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, 'admin_pending', now())
		 RETURNING id, guild_id, channel_id, message_id, reporter_id, reason, status,
		           resolved_by, resolved_at, created_at`,
		reportID, guildID, channelID, messageID, userID,
		"[ADMIN REPORT] "+req.Reason,
	).Scan(
		&report.ID, &report.GuildID, &report.ChannelID, &report.MessageID,
		&report.ReporterID, &report.Reason, &report.Status,
		&report.ResolvedBy, &report.ResolvedAt, &report.CreatedAt,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to submit report", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusCreated, report)
}

// HandleGetAdminReports lists reports escalated to instance admins.
// GET /api/v1/admin/reports
func (h *Handler) HandleGetAdminReports(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	// Check if user is instance admin.
	var flags int
	err := h.Pool.QueryRow(r.Context(),
		`SELECT flags FROM users WHERE id = $1`, userID).Scan(&flags)
	if err != nil || flags&models.UserFlagAdmin == 0 {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT mr.id, mr.guild_id, mr.channel_id, mr.message_id, mr.reporter_id,
		        mr.reason, mr.status, mr.resolved_by, mr.resolved_at, mr.created_at,
		        u.username as reporter_name
		 FROM message_reports mr
		 JOIN users u ON u.id = mr.reporter_id
		 WHERE mr.status = 'admin_pending' OR mr.reason LIKE '[ADMIN REPORT]%'
		 ORDER BY mr.created_at DESC
		 LIMIT 50`)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list reports")
		return
	}
	defer rows.Close()

	type adminReport struct {
		models.MessageReport
		ReporterName string `json:"reporter_name"`
	}

	reports := make([]adminReport, 0)
	for rows.Next() {
		var r adminReport
		if err := rows.Scan(
			&r.ID, &r.GuildID, &r.ChannelID, &r.MessageID,
			&r.ReporterID, &r.Reason, &r.Status,
			&r.ResolvedBy, &r.ResolvedAt, &r.CreatedAt,
			&r.ReporterName,
		); err != nil {
			continue
		}
		reports = append(reports, r)
	}

	apiutil.WriteJSON(w, http.StatusOK, reports)
}

// hasGuildPermission checks if a user has a specific permission in a guild.
// Falls back to isGuildAdmin for admin-level override.
func (h *Handler) hasGuildPermission(ctx context.Context, guildID, userID, permName string) bool {
	// Guild admins always have all permissions.
	if h.isGuildAdmin(ctx, guildID, userID) {
		return true
	}

	// For now, check if the user has the relevant role permission.
	// BAN_MEMBERS is the main permission for ban list operations.
	var defaultPerms int64
	h.Pool.QueryRow(ctx, `SELECT default_permissions FROM guilds WHERE id = $1`, guildID).Scan(&defaultPerms)
	computed := uint64(defaultPerms)

	rows, _ := h.Pool.Query(ctx,
		`SELECT r.permissions_allow, r.permissions_deny
		 FROM roles r
		 JOIN member_roles mr ON r.id = mr.role_id
		 WHERE mr.guild_id = $1 AND mr.user_id = $2
		 ORDER BY r.position DESC`,
		guildID, userID,
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

	// Map permission name to bitfield.
	permMap := map[string]uint64{
		"ban_members":    1 << 2,
		"manage_guild":   1 << 5,
		"manage_roles":   1 << 24,
		"administrator":  1 << 3,
	}
	if computed&(1<<3) != 0 { // Administrator
		return true
	}
	if bit, ok := permMap[permName]; ok {
		return computed&bit != 0
	}
	return false
}
