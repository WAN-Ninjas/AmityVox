package guilds

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/oklog/ulid/v2"

	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/permissions"
)

// RetentionPolicy represents a data retention policy scoped to a guild or channel.
type RetentionPolicy struct {
	ID                string     `json:"id"`
	ChannelID         *string    `json:"channel_id,omitempty"`
	GuildID           *string    `json:"guild_id,omitempty"`
	MaxAgeDays        int        `json:"max_age_days"`
	DeleteAttachments bool       `json:"delete_attachments"`
	DeletePins        bool       `json:"delete_pins"`
	Enabled           bool       `json:"enabled"`
	LastRunAt         *time.Time `json:"last_run_at,omitempty"`
	NextRunAt         *time.Time `json:"next_run_at,omitempty"`
	MessagesDeleted   int64      `json:"messages_deleted"`
	CreatedBy         string     `json:"created_by"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// HandleGetGuildRetentionPolicies lists retention policies for a guild.
// GET /api/v1/guilds/{guildID}/retention
func (h *Handler) HandleGetGuildRetentionPolicies(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageGuild) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_GUILD permission")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, channel_id, guild_id, max_age_days, delete_attachments, delete_pins,
		        enabled, last_run_at, next_run_at, messages_deleted, created_by, created_at, updated_at
		 FROM data_retention_policies
		 WHERE guild_id = $1
		 ORDER BY created_at DESC`, guildID)
	if err != nil {
		h.Logger.Error("failed to list retention policies", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list retention policies")
		return
	}
	defer rows.Close()

	var policies []RetentionPolicy
	for rows.Next() {
		var p RetentionPolicy
		if err := rows.Scan(&p.ID, &p.ChannelID, &p.GuildID, &p.MaxAgeDays, &p.DeleteAttachments,
			&p.DeletePins, &p.Enabled, &p.LastRunAt, &p.NextRunAt, &p.MessagesDeleted,
			&p.CreatedBy, &p.CreatedAt, &p.UpdatedAt); err != nil {
			continue
		}
		policies = append(policies, p)
	}
	if policies == nil {
		policies = []RetentionPolicy{}
	}

	writeJSON(w, http.StatusOK, policies)
}

// HandleCreateGuildRetentionPolicy creates a new retention policy for a guild.
// POST /api/v1/guilds/{guildID}/retention
func (h *Handler) HandleCreateGuildRetentionPolicy(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageGuild) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_GUILD permission")
		return
	}

	var req struct {
		ChannelID         *string `json:"channel_id"`
		MaxAgeDays        int     `json:"max_age_days"`
		DeleteAttachments *bool   `json:"delete_attachments"`
		DeletePins        *bool   `json:"delete_pins"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.MaxAgeDays < 1 {
		writeError(w, http.StatusBadRequest, "invalid_max_age", "max_age_days must be at least 1")
		return
	}

	// Enforce instance minimum retention.
	var minDaysStr string
	h.Pool.QueryRow(r.Context(),
		`SELECT COALESCE((SELECT value FROM instance_settings WHERE key = 'min_retention_days'), '0')`).Scan(&minDaysStr)
	minDays, _ := strconv.Atoi(minDaysStr)
	if minDays > 0 && req.MaxAgeDays < minDays {
		writeError(w, http.StatusBadRequest, "below_minimum", "max_age_days cannot be less than the instance minimum of "+strconv.Itoa(minDays)+" days")
		return
	}

	// Verify channel belongs to guild if channel-scoped.
	if req.ChannelID != nil {
		var channelGuildID *string
		err := h.Pool.QueryRow(r.Context(),
			`SELECT guild_id FROM channels WHERE id = $1`, *req.ChannelID).Scan(&channelGuildID)
		if err != nil || channelGuildID == nil || *channelGuildID != guildID {
			writeError(w, http.StatusBadRequest, "invalid_channel", "Channel does not belong to this guild")
			return
		}
	}

	deleteAttachments := true
	if req.DeleteAttachments != nil {
		deleteAttachments = *req.DeleteAttachments
	}
	deletePins := false
	if req.DeletePins != nil {
		deletePins = *req.DeletePins
	}

	id := ulid.Make().String()
	var p RetentionPolicy
	err := h.Pool.QueryRow(r.Context(),
		`INSERT INTO data_retention_policies (id, channel_id, guild_id, max_age_days, delete_attachments, delete_pins, enabled, created_by, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, true, $7, now(), now())
		 RETURNING id, channel_id, guild_id, max_age_days, delete_attachments, delete_pins, enabled, last_run_at, next_run_at, messages_deleted, created_by, created_at, updated_at`,
		id, req.ChannelID, guildID, req.MaxAgeDays, deleteAttachments, deletePins, userID,
	).Scan(&p.ID, &p.ChannelID, &p.GuildID, &p.MaxAgeDays, &p.DeleteAttachments,
		&p.DeletePins, &p.Enabled, &p.LastRunAt, &p.NextRunAt, &p.MessagesDeleted,
		&p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		h.Logger.Error("failed to create retention policy", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create retention policy")
		return
	}

	writeJSON(w, http.StatusCreated, p)
}

// HandleUpdateGuildRetentionPolicy updates an existing retention policy.
// PATCH /api/v1/guilds/{guildID}/retention/{policyID}
func (h *Handler) HandleUpdateGuildRetentionPolicy(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	policyID := chi.URLParam(r, "policyID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageGuild) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_GUILD permission")
		return
	}

	// Verify policy belongs to this guild.
	var existingGuildID *string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT guild_id FROM data_retention_policies WHERE id = $1`, policyID).Scan(&existingGuildID)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "not_found", "Retention policy not found")
		return
	}
	if err != nil || existingGuildID == nil || *existingGuildID != guildID {
		writeError(w, http.StatusNotFound, "not_found", "Retention policy not found")
		return
	}

	var req struct {
		MaxAgeDays        *int  `json:"max_age_days"`
		DeleteAttachments *bool `json:"delete_attachments"`
		DeletePins        *bool `json:"delete_pins"`
		Enabled           *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	// Enforce instance minimum retention.
	if req.MaxAgeDays != nil {
		if *req.MaxAgeDays < 1 {
			writeError(w, http.StatusBadRequest, "invalid_max_age", "max_age_days must be at least 1")
			return
		}
		var minDaysStr string
		h.Pool.QueryRow(r.Context(),
			`SELECT COALESCE((SELECT value FROM instance_settings WHERE key = 'min_retention_days'), '0')`).Scan(&minDaysStr)
		minDays, _ := strconv.Atoi(minDaysStr)
		if minDays > 0 && *req.MaxAgeDays < minDays {
			writeError(w, http.StatusBadRequest, "below_minimum", "max_age_days cannot be less than the instance minimum of "+strconv.Itoa(minDays)+" days")
			return
		}
	}

	// Build dynamic update.
	setClauses := []string{"updated_at = now()"}
	args := []interface{}{}
	argIdx := 1

	if req.MaxAgeDays != nil {
		setClauses = append(setClauses, "max_age_days = $"+strconv.Itoa(argIdx))
		args = append(args, *req.MaxAgeDays)
		argIdx++
	}
	if req.DeleteAttachments != nil {
		setClauses = append(setClauses, "delete_attachments = $"+strconv.Itoa(argIdx))
		args = append(args, *req.DeleteAttachments)
		argIdx++
	}
	if req.DeletePins != nil {
		setClauses = append(setClauses, "delete_pins = $"+strconv.Itoa(argIdx))
		args = append(args, *req.DeletePins)
		argIdx++
	}
	if req.Enabled != nil {
		setClauses = append(setClauses, "enabled = $"+strconv.Itoa(argIdx))
		args = append(args, *req.Enabled)
		argIdx++
	}

	args = append(args, policyID)
	query := "UPDATE data_retention_policies SET " + joinStrings(setClauses, ", ") +
		" WHERE id = $" + strconv.Itoa(argIdx) +
		" RETURNING id, channel_id, guild_id, max_age_days, delete_attachments, delete_pins, enabled, last_run_at, next_run_at, messages_deleted, created_by, created_at, updated_at"

	var p RetentionPolicy
	err = h.Pool.QueryRow(r.Context(), query, args...).Scan(
		&p.ID, &p.ChannelID, &p.GuildID, &p.MaxAgeDays, &p.DeleteAttachments,
		&p.DeletePins, &p.Enabled, &p.LastRunAt, &p.NextRunAt, &p.MessagesDeleted,
		&p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		h.Logger.Error("failed to update retention policy", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update retention policy")
		return
	}

	writeJSON(w, http.StatusOK, p)
}

// HandleDeleteGuildRetentionPolicy deletes a retention policy.
// DELETE /api/v1/guilds/{guildID}/retention/{policyID}
func (h *Handler) HandleDeleteGuildRetentionPolicy(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	policyID := chi.URLParam(r, "policyID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageGuild) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_GUILD permission")
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM data_retention_policies WHERE id = $1 AND guild_id = $2`, policyID, guildID)
	if err != nil {
		h.Logger.Error("failed to delete retention policy", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete retention policy")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Retention policy not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// joinStrings joins strings with a separator (avoiding importing strings package for one use).
func joinStrings(parts []string, sep string) string {
	result := ""
	for i, part := range parts {
		if i > 0 {
			result += sep
		}
		result += part
	}
	return result
}
