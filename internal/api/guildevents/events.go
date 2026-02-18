// Package guildevents implements REST API handlers for scheduled guild events
// including creating, listing, updating, and deleting events, as well as RSVP
// management. Mounted under /api/v1/guilds/{guildID}/events.
package guildevents

import (
	"context"
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
	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/models"
)

// Handler implements guild event REST API endpoints.
type Handler struct {
	Pool     *pgxpool.Pool
	EventBus *events.Bus
	Logger   *slog.Logger
}

// GuildEvent represents a scheduled event in a guild.
type GuildEvent struct {
	ID                string     `json:"id"`
	GuildID           string     `json:"guild_id"`
	CreatorID         string     `json:"creator_id"`
	Name              string     `json:"name"`
	Description       *string    `json:"description,omitempty"`
	Location          *string    `json:"location,omitempty"`
	ChannelID         *string    `json:"channel_id,omitempty"`
	ImageID           *string    `json:"image_id,omitempty"`
	ScheduledStart    time.Time  `json:"scheduled_start"`
	ScheduledEnd      *time.Time `json:"scheduled_end,omitempty"`
	Status            string     `json:"status"`
	InterestedCount   int        `json:"interested_count"`
	AutoCancelMinutes int        `json:"auto_cancel_minutes"`
	CreatedAt         time.Time  `json:"created_at"`
	Creator           *User      `json:"creator,omitempty"`
	UserRSVP          *string    `json:"user_rsvp,omitempty"`
}

// User is a minimal user representation embedded in event responses.
type User struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName *string `json:"display_name,omitempty"`
	AvatarID    *string `json:"avatar_id,omitempty"`
}

// EventRSVP represents a user's RSVP to a guild event.
type EventRSVP struct {
	EventID   string    `json:"event_id"`
	UserID    string    `json:"user_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	User      *User     `json:"user,omitempty"`
}

type createEventRequest struct {
	Name              string  `json:"name"`
	Description       *string `json:"description"`
	Location          *string `json:"location"`
	ChannelID         *string `json:"channel_id"`
	ImageID           *string `json:"image_id"`
	ScheduledStart    string  `json:"scheduled_start"`
	ScheduledEnd      *string `json:"scheduled_end"`
	AutoCancelMinutes *int    `json:"auto_cancel_minutes"`
}

type updateEventRequest struct {
	Name              *string `json:"name"`
	Description       *string `json:"description"`
	Location          *string `json:"location"`
	ChannelID         *string `json:"channel_id"`
	ImageID           *string `json:"image_id"`
	ScheduledStart    *string `json:"scheduled_start"`
	ScheduledEnd      *string `json:"scheduled_end"`
	Status            *string `json:"status"`
	AutoCancelMinutes *int    `json:"auto_cancel_minutes"`
}

type rsvpRequest struct {
	Status string `json:"status"`
}

// --- Helpers ---

func (h *Handler) isMember(ctx context.Context, guildID, userID string) bool {
	var exists bool
	h.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM guild_members WHERE guild_id = $1 AND user_id = $2)`,
		guildID, userID,
	).Scan(&exists)
	return exists
}

func (h *Handler) isCreatorOrAdmin(ctx context.Context, guildID, userID, creatorID string) bool {
	if userID == creatorID {
		return true
	}

	// Check if guild owner.
	var ownerID string
	if err := h.Pool.QueryRow(ctx,
		`SELECT owner_id FROM guilds WHERE id = $1`, guildID,
	).Scan(&ownerID); err == nil && ownerID == userID {
		return true
	}

	// Check if instance admin.
	var flags int
	h.Pool.QueryRow(ctx, `SELECT flags FROM users WHERE id = $1`, userID).Scan(&flags)
	return flags&models.UserFlagAdmin != 0
}

// HandleCreateEvent creates a new scheduled event in a guild.
// POST /api/v1/guilds/{guildID}/events
func (h *Handler) HandleCreateEvent(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isMember(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	var req createEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Name == "" || len(req.Name) > 100 {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_name", "Event name must be 1-100 characters")
		return
	}

	scheduledStart, err := time.Parse(time.RFC3339, req.ScheduledStart)
	if err != nil {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_scheduled_start", "scheduled_start must be a valid RFC3339 timestamp")
		return
	}
	if !scheduledStart.After(time.Now()) {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_scheduled_start", "scheduled_start must be in the future")
		return
	}

	var scheduledEnd *time.Time
	if req.ScheduledEnd != nil {
		t, err := time.Parse(time.RFC3339, *req.ScheduledEnd)
		if err != nil {
			apiutil.WriteError(w, http.StatusBadRequest, "invalid_scheduled_end", "scheduled_end must be a valid RFC3339 timestamp")
			return
		}
		if !t.After(scheduledStart) {
			apiutil.WriteError(w, http.StatusBadRequest, "invalid_scheduled_end", "scheduled_end must be after scheduled_start")
			return
		}
		scheduledEnd = &t
	}

	eventID := models.NewULID().String()

	autoCancelMinutes := 30 // default
	if req.AutoCancelMinutes != nil {
		if *req.AutoCancelMinutes < 0 || *req.AutoCancelMinutes > 1440 {
			apiutil.WriteError(w, http.StatusBadRequest, "invalid_auto_cancel", "auto_cancel_minutes must be between 0 and 1440")
			return
		}
		autoCancelMinutes = *req.AutoCancelMinutes
	}

	var evt GuildEvent
	err = h.Pool.QueryRow(r.Context(),
		`INSERT INTO guild_events (id, guild_id, creator_id, name, description, location, channel_id, image_id,
		                           scheduled_start, scheduled_end, status, interested_count, auto_cancel_minutes, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'scheduled', 0, $11, now())
		 RETURNING id, guild_id, creator_id, name, description, location, channel_id, image_id,
		           scheduled_start, scheduled_end, status, interested_count, auto_cancel_minutes, created_at`,
		eventID, guildID, userID, req.Name, req.Description, req.Location,
		req.ChannelID, req.ImageID, scheduledStart, scheduledEnd, autoCancelMinutes,
	).Scan(
		&evt.ID, &evt.GuildID, &evt.CreatorID, &evt.Name, &evt.Description,
		&evt.Location, &evt.ChannelID, &evt.ImageID, &evt.ScheduledStart,
		&evt.ScheduledEnd, &evt.Status, &evt.InterestedCount, &evt.AutoCancelMinutes, &evt.CreatedAt,
	)
	if err != nil {
		h.Logger.Error("failed to create guild event", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create event")
		return
	}

	h.EventBus.PublishGuildEvent(r.Context(), events.SubjectGuildEventCreate, "GUILD_EVENT_CREATE", evt.GuildID, evt)

	apiutil.WriteJSON(w, http.StatusCreated, evt)
}

// autoCancelOverdueEvents checks for events in a guild that are past their
// scheduled start time plus the auto_cancel_minutes grace window and marks
// them as cancelled. This is a check-on-fetch approach that runs each time
// events are listed or fetched for a guild, avoiding the need for a separate
// background worker.
func (h *Handler) autoCancelOverdueEvents(ctx context.Context, guildID string) {
	// Find all scheduled events that are past their start time + grace window.
	tag, err := h.Pool.Exec(ctx,
		`UPDATE guild_events
		 SET status = 'cancelled'
		 WHERE guild_id = $1
		   AND status = 'scheduled'
		   AND scheduled_start + (auto_cancel_minutes * INTERVAL '1 minute') < now()`,
		guildID,
	)
	if err != nil {
		h.Logger.Error("failed to auto-cancel overdue events",
			slog.String("guild_id", guildID),
			slog.String("error", err.Error()),
		)
		return
	}
	if tag.RowsAffected() > 0 {
		h.Logger.Info("auto-cancelled overdue events",
			slog.String("guild_id", guildID),
			slog.Int64("count", tag.RowsAffected()),
		)
		// Publish event update for each cancelled event so gateway clients
		// see the status change in real time.
		rows, err := h.Pool.Query(ctx,
			`SELECT id, guild_id, creator_id, name, status FROM guild_events
			 WHERE guild_id = $1 AND status = 'cancelled'
			   AND scheduled_start + (auto_cancel_minutes * INTERVAL '1 minute') < now()
			   AND scheduled_start + (auto_cancel_minutes * INTERVAL '1 minute') > now() - INTERVAL '1 minute'`,
			guildID,
		)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var id, gID, creatorID, name, status string
				if rows.Scan(&id, &gID, &creatorID, &name, &status) == nil {
					h.EventBus.PublishGuildEvent(ctx, events.SubjectGuildEventUpdate, "GUILD_EVENT_UPDATE", gID, map[string]string{
						"id":       id,
						"guild_id": gID,
						"status":   status,
					})
				}
			}
		}
	}
}

// HandleListEvents lists upcoming events for a guild.
// Before returning results, auto-cancels any overdue events past their grace window.
// GET /api/v1/guilds/{guildID}/events
func (h *Handler) HandleListEvents(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isMember(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	// Auto-cancel any overdue events before returning the list.
	h.autoCancelOverdueEvents(r.Context(), guildID)

	// Parse query parameters.
	statusFilter := r.URL.Query().Get("status")
	limit := 25
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > 100 {
		limit = 100
	}

	var query string
	var args []interface{}

	if statusFilter != "" {
		query = `SELECT e.id, e.guild_id, e.creator_id, e.name, e.description, e.location,
		                e.channel_id, e.image_id, e.scheduled_start, e.scheduled_end,
		                e.status, e.interested_count, e.auto_cancel_minutes, e.created_at,
		                u.id, u.username, u.display_name, u.avatar_id,
		                r.status
		         FROM guild_events e
		         JOIN users u ON u.id = e.creator_id
		         LEFT JOIN event_rsvps r ON r.event_id = e.id AND r.user_id = $3
		         WHERE e.guild_id = $1 AND e.status = $4
		         ORDER BY e.scheduled_start ASC
		         LIMIT $2`
		args = []interface{}{guildID, limit, userID, statusFilter}
	} else {
		query = `SELECT e.id, e.guild_id, e.creator_id, e.name, e.description, e.location,
		                e.channel_id, e.image_id, e.scheduled_start, e.scheduled_end,
		                e.status, e.interested_count, e.auto_cancel_minutes, e.created_at,
		                u.id, u.username, u.display_name, u.avatar_id,
		                r.status
		         FROM guild_events e
		         JOIN users u ON u.id = e.creator_id
		         LEFT JOIN event_rsvps r ON r.event_id = e.id AND r.user_id = $3
		         WHERE e.guild_id = $1
		         ORDER BY e.scheduled_start ASC
		         LIMIT $2`
		args = []interface{}{guildID, limit, userID}
	}

	rows, err := h.Pool.Query(r.Context(), query, args...)
	if err != nil {
		h.Logger.Error("failed to list guild events", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list events")
		return
	}
	defer rows.Close()

	eventsList := make([]GuildEvent, 0)
	for rows.Next() {
		var evt GuildEvent
		var creator User
		err := rows.Scan(
			&evt.ID, &evt.GuildID, &evt.CreatorID, &evt.Name, &evt.Description,
			&evt.Location, &evt.ChannelID, &evt.ImageID, &evt.ScheduledStart,
			&evt.ScheduledEnd, &evt.Status, &evt.InterestedCount, &evt.AutoCancelMinutes, &evt.CreatedAt,
			&creator.ID, &creator.Username, &creator.DisplayName, &creator.AvatarID,
			&evt.UserRSVP,
		)
		if err != nil {
			h.Logger.Error("failed to scan guild event row", slog.String("error", err.Error()))
			apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list events")
			return
		}
		evt.Creator = &creator
		eventsList = append(eventsList, evt)
	}
	if err := rows.Err(); err != nil {
		h.Logger.Error("error iterating guild event rows", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list events")
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, eventsList)
}

// HandleGetEvent returns a single guild event by ID.
// Also auto-cancels overdue events before returning the result.
// GET /api/v1/guilds/{guildID}/events/{eventID}
func (h *Handler) HandleGetEvent(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	eventID := chi.URLParam(r, "eventID")

	// Auto-cancel overdue events before returning.
	h.autoCancelOverdueEvents(r.Context(), guildID)

	if !h.isMember(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	var evt GuildEvent
	var creator User
	err := h.Pool.QueryRow(r.Context(),
		`SELECT e.id, e.guild_id, e.creator_id, e.name, e.description, e.location,
		        e.channel_id, e.image_id, e.scheduled_start, e.scheduled_end,
		        e.status, e.interested_count, e.auto_cancel_minutes, e.created_at,
		        u.id, u.username, u.display_name, u.avatar_id,
		        r.status
		 FROM guild_events e
		 JOIN users u ON u.id = e.creator_id
		 LEFT JOIN event_rsvps r ON r.event_id = e.id AND r.user_id = $3
		 WHERE e.id = $1 AND e.guild_id = $2`,
		eventID, guildID, userID,
	).Scan(
		&evt.ID, &evt.GuildID, &evt.CreatorID, &evt.Name, &evt.Description,
		&evt.Location, &evt.ChannelID, &evt.ImageID, &evt.ScheduledStart,
		&evt.ScheduledEnd, &evt.Status, &evt.InterestedCount, &evt.AutoCancelMinutes, &evt.CreatedAt,
		&creator.ID, &creator.Username, &creator.DisplayName, &creator.AvatarID,
		&evt.UserRSVP,
	)
	if err == pgx.ErrNoRows {
		apiutil.WriteError(w, http.StatusNotFound, "event_not_found", "Event not found")
		return
	}
	if err != nil {
		h.Logger.Error("failed to get guild event", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get event")
		return
	}
	evt.Creator = &creator

	apiutil.WriteJSON(w, http.StatusOK, evt)
}

// HandleUpdateEvent updates a guild event. Only the creator or a guild admin can update.
// PATCH /api/v1/guilds/{guildID}/events/{eventID}
func (h *Handler) HandleUpdateEvent(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	eventID := chi.URLParam(r, "eventID")

	// Fetch event to check ownership.
	var creatorID string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT creator_id FROM guild_events WHERE id = $1 AND guild_id = $2`,
		eventID, guildID,
	).Scan(&creatorID)
	if err == pgx.ErrNoRows {
		apiutil.WriteError(w, http.StatusNotFound, "event_not_found", "Event not found")
		return
	}
	if err != nil {
		h.Logger.Error("failed to get guild event for update", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get event")
		return
	}

	if !h.isCreatorOrAdmin(r.Context(), guildID, userID, creatorID) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "You do not have permission to update this event")
		return
	}

	var req updateEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Name != nil && (len(*req.Name) == 0 || len(*req.Name) > 100) {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_name", "Event name must be 1-100 characters")
		return
	}

	var scheduledStart *time.Time
	if req.ScheduledStart != nil {
		t, err := time.Parse(time.RFC3339, *req.ScheduledStart)
		if err != nil {
			apiutil.WriteError(w, http.StatusBadRequest, "invalid_scheduled_start", "scheduled_start must be a valid RFC3339 timestamp")
			return
		}
		scheduledStart = &t
	}

	var scheduledEnd *time.Time
	if req.ScheduledEnd != nil {
		t, err := time.Parse(time.RFC3339, *req.ScheduledEnd)
		if err != nil {
			apiutil.WriteError(w, http.StatusBadRequest, "invalid_scheduled_end", "scheduled_end must be a valid RFC3339 timestamp")
			return
		}
		scheduledEnd = &t
	}

	var evt GuildEvent
	err = h.Pool.QueryRow(r.Context(),
		`UPDATE guild_events SET
			name = COALESCE($3, name),
			description = COALESCE($4, description),
			location = COALESCE($5, location),
			channel_id = COALESCE($6, channel_id),
			image_id = COALESCE($7, image_id),
			scheduled_start = COALESCE($8, scheduled_start),
			scheduled_end = COALESCE($9, scheduled_end),
			status = COALESCE($10, status),
			auto_cancel_minutes = COALESCE($11, auto_cancel_minutes)
		 WHERE id = $1 AND guild_id = $2
		 RETURNING id, guild_id, creator_id, name, description, location, channel_id, image_id,
		           scheduled_start, scheduled_end, status, interested_count, auto_cancel_minutes, created_at`,
		eventID, guildID, req.Name, req.Description, req.Location,
		req.ChannelID, req.ImageID, scheduledStart, scheduledEnd, req.Status, req.AutoCancelMinutes,
	).Scan(
		&evt.ID, &evt.GuildID, &evt.CreatorID, &evt.Name, &evt.Description,
		&evt.Location, &evt.ChannelID, &evt.ImageID, &evt.ScheduledStart,
		&evt.ScheduledEnd, &evt.Status, &evt.InterestedCount, &evt.AutoCancelMinutes, &evt.CreatedAt,
	)
	if err != nil {
		h.Logger.Error("failed to update guild event", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update event")
		return
	}

	h.EventBus.PublishGuildEvent(r.Context(), events.SubjectGuildEventUpdate, "GUILD_EVENT_UPDATE", guildID, evt)

	apiutil.WriteJSON(w, http.StatusOK, evt)
}

// HandleDeleteEvent deletes a guild event. Only the creator or a guild admin can delete.
// DELETE /api/v1/guilds/{guildID}/events/{eventID}
func (h *Handler) HandleDeleteEvent(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	eventID := chi.URLParam(r, "eventID")

	// Fetch event to check ownership.
	var creatorID string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT creator_id FROM guild_events WHERE id = $1 AND guild_id = $2`,
		eventID, guildID,
	).Scan(&creatorID)
	if err == pgx.ErrNoRows {
		apiutil.WriteError(w, http.StatusNotFound, "event_not_found", "Event not found")
		return
	}
	if err != nil {
		h.Logger.Error("failed to get guild event for delete", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get event")
		return
	}

	if !h.isCreatorOrAdmin(r.Context(), guildID, userID, creatorID) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "You do not have permission to delete this event")
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM guild_events WHERE id = $1 AND guild_id = $2`,
		eventID, guildID,
	)
	if err != nil {
		h.Logger.Error("failed to delete guild event", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete event")
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "event_not_found", "Event not found")
		return
	}

	h.EventBus.PublishGuildEvent(r.Context(), events.SubjectGuildEventDelete, "GUILD_EVENT_DELETE", guildID, map[string]string{
		"id":       eventID,
		"guild_id": guildID,
	})

	w.WriteHeader(http.StatusNoContent)
}

// HandleRSVP creates or updates a user's RSVP for a guild event.
// POST /api/v1/guilds/{guildID}/events/{eventID}/rsvp
func (h *Handler) HandleRSVP(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	eventID := chi.URLParam(r, "eventID")

	if !h.isMember(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	var req rsvpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Status != "interested" && req.Status != "going" {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_status", "RSVP status must be \"interested\" or \"going\"")
		return
	}

	// Verify the event exists in this guild.
	var exists bool
	err := h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM guild_events WHERE id = $1 AND guild_id = $2)`,
		eventID, guildID,
	).Scan(&exists)
	if err != nil || !exists {
		apiutil.WriteError(w, http.StatusNotFound, "event_not_found", "Event not found")
		return
	}

	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		h.Logger.Error("failed to begin transaction", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create RSVP")
		return
	}
	defer tx.Rollback(r.Context())

	// Upsert RSVP.
	var rsvp EventRSVP
	err = tx.QueryRow(r.Context(),
		`INSERT INTO event_rsvps (event_id, user_id, status, created_at)
		 VALUES ($1, $2, $3, now())
		 ON CONFLICT (event_id, user_id)
		 DO UPDATE SET status = EXCLUDED.status
		 RETURNING event_id, user_id, status, created_at`,
		eventID, userID, req.Status,
	).Scan(&rsvp.EventID, &rsvp.UserID, &rsvp.Status, &rsvp.CreatedAt)
	if err != nil {
		h.Logger.Error("failed to upsert RSVP", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create RSVP")
		return
	}

	// Update interested_count on the event.
	_, err = tx.Exec(r.Context(),
		`UPDATE guild_events
		 SET interested_count = (SELECT COUNT(*) FROM event_rsvps WHERE event_id = $1)
		 WHERE id = $1`,
		eventID,
	)
	if err != nil {
		h.Logger.Error("failed to update interested count", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update event")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		h.Logger.Error("failed to commit RSVP transaction", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create RSVP")
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, rsvp)
}

// HandleDeleteRSVP removes a user's RSVP for a guild event.
// DELETE /api/v1/guilds/{guildID}/events/{eventID}/rsvp
func (h *Handler) HandleDeleteRSVP(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	eventID := chi.URLParam(r, "eventID")

	if !h.isMember(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		h.Logger.Error("failed to begin transaction", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete RSVP")
		return
	}
	defer tx.Rollback(r.Context())

	tag, err := tx.Exec(r.Context(),
		`DELETE FROM event_rsvps WHERE event_id = $1 AND user_id = $2`,
		eventID, userID,
	)
	if err != nil {
		h.Logger.Error("failed to delete RSVP", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete RSVP")
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "rsvp_not_found", "You have not RSVPed to this event")
		return
	}

	// Update interested_count on the event.
	_, err = tx.Exec(r.Context(),
		`UPDATE guild_events
		 SET interested_count = (SELECT COUNT(*) FROM event_rsvps WHERE event_id = $1)
		 WHERE id = $1`,
		eventID,
	)
	if err != nil {
		h.Logger.Error("failed to update interested count", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update event")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		h.Logger.Error("failed to commit RSVP deletion", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete RSVP")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleListRSVPs returns all RSVPs for a guild event with user info.
// GET /api/v1/guilds/{guildID}/events/{eventID}/rsvps
func (h *Handler) HandleListRSVPs(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	eventID := chi.URLParam(r, "eventID")

	if !h.isMember(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	// Verify the event exists in this guild.
	var exists bool
	err := h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM guild_events WHERE id = $1 AND guild_id = $2)`,
		eventID, guildID,
	).Scan(&exists)
	if err != nil || !exists {
		apiutil.WriteError(w, http.StatusNotFound, "event_not_found", "Event not found")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT r.event_id, r.user_id, r.status, r.created_at,
		        u.id, u.username, u.display_name, u.avatar_id
		 FROM event_rsvps r
		 JOIN users u ON u.id = r.user_id
		 WHERE r.event_id = $1
		 ORDER BY r.created_at ASC`,
		eventID,
	)
	if err != nil {
		h.Logger.Error("failed to list RSVPs", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list RSVPs")
		return
	}
	defer rows.Close()

	rsvps := make([]EventRSVP, 0)
	for rows.Next() {
		var rsvp EventRSVP
		var user User
		err := rows.Scan(
			&rsvp.EventID, &rsvp.UserID, &rsvp.Status, &rsvp.CreatedAt,
			&user.ID, &user.Username, &user.DisplayName, &user.AvatarID,
		)
		if err != nil {
			h.Logger.Error("failed to scan RSVP row", slog.String("error", err.Error()))
			apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list RSVPs")
			return
		}
		rsvp.User = &user
		rsvps = append(rsvps, rsvp)
	}
	if err := rows.Err(); err != nil {
		h.Logger.Error("error iterating RSVP rows", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list RSVPs")
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, rsvps)
}
