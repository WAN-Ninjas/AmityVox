// Package invites implements REST API handlers for invite operations including
// looking up invites by code, accepting invites to join guilds, and deleting
// invites. Mounted under /api/v1/invites.
package invites

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
	"github.com/amityvox/amityvox/internal/permissions"
)

// Handler implements invite-related REST API endpoints.
type Handler struct {
	Pool       *pgxpool.Pool
	EventBus   *events.Bus
	InstanceID string
	Logger     *slog.Logger
}

// --- Helpers ---

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

// HandleGetInvite handles GET /api/v1/invites/{code}.
// Returns the invite info including guild name and member count.
func (h *Handler) HandleGetInvite(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	var inv models.Invite
	err := h.Pool.QueryRow(r.Context(),
		`SELECT code, guild_id, channel_id, creator_id, max_uses, uses,
		        max_age_seconds, temporary, created_at, expires_at
		 FROM invites WHERE code = $1`, code).Scan(
		&inv.Code, &inv.GuildID, &inv.ChannelID, &inv.CreatorID,
		&inv.MaxUses, &inv.Uses, &inv.MaxAgeSeconds, &inv.Temporary,
		&inv.CreatedAt, &inv.ExpiresAt,
	)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "invite_not_found", "Invite not found or has expired")
		return
	}
	if err != nil {
		h.Logger.Error("failed to get invite", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get invite")
		return
	}

	if inv.IsExpired() {
		writeError(w, http.StatusNotFound, "invite_expired", "This invite has expired")
		return
	}
	if inv.MaxUses != nil && inv.Uses >= *inv.MaxUses {
		writeError(w, http.StatusNotFound, "invite_exhausted", "This invite has reached its maximum uses")
		return
	}

	// Enrich with guild info.
	var guildName string
	var memberCount int
	err = h.Pool.QueryRow(r.Context(),
		`SELECT g.name, (SELECT COUNT(*) FROM guild_members WHERE guild_id = g.id)
		 FROM guilds g WHERE g.id = $1`, inv.GuildID).Scan(&guildName, &memberCount)
	if err != nil {
		guildName = "Unknown"
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"invite":       inv,
		"guild_name":   guildName,
		"member_count": memberCount,
	})
}

// HandleAcceptInvite handles POST /api/v1/invites/{code}.
// Joins the authenticated user to the guild associated with the invite.
func (h *Handler) HandleAcceptInvite(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	userID := auth.UserIDFromContext(r.Context())

	var inv models.Invite
	err := h.Pool.QueryRow(r.Context(),
		`SELECT code, guild_id, channel_id, creator_id, max_uses, uses,
		        max_age_seconds, temporary, created_at, expires_at
		 FROM invites WHERE code = $1`, code).Scan(
		&inv.Code, &inv.GuildID, &inv.ChannelID, &inv.CreatorID,
		&inv.MaxUses, &inv.Uses, &inv.MaxAgeSeconds, &inv.Temporary,
		&inv.CreatedAt, &inv.ExpiresAt,
	)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "invite_not_found", "Invite not found")
		return
	}
	if err != nil {
		h.Logger.Error("failed to get invite", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get invite")
		return
	}

	if inv.IsExpired() {
		writeError(w, http.StatusGone, "invite_expired", "This invite has expired")
		return
	}
	if inv.MaxUses != nil && inv.Uses >= *inv.MaxUses {
		writeError(w, http.StatusGone, "invite_exhausted", "This invite has reached its maximum uses")
		return
	}

	// Check if already a member.
	var exists bool
	err = h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM guild_members WHERE guild_id = $1 AND user_id = $2)`,
		inv.GuildID, userID).Scan(&exists)
	if err != nil {
		h.Logger.Error("failed to check membership", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to check membership")
		return
	}
	if exists {
		writeError(w, http.StatusConflict, "already_member", "You are already a member of this guild")
		return
	}

	now := time.Now().UTC()
	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		h.Logger.Error("failed to begin transaction", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to join guild")
		return
	}
	defer tx.Rollback(r.Context())

	// Add guild member.
	_, err = tx.Exec(r.Context(),
		`INSERT INTO guild_members (guild_id, user_id, nickname, joined_at, deaf, mute)
		 VALUES ($1, $2, NULL, $3, false, false)`,
		inv.GuildID, userID, now)
	if err != nil {
		h.Logger.Error("failed to add member", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to join guild")
		return
	}

	// Increment invite usage.
	_, err = tx.Exec(r.Context(),
		`UPDATE invites SET uses = uses + 1 WHERE code = $1`, code)
	if err != nil {
		h.Logger.Error("failed to increment invite uses", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update invite")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		h.Logger.Error("failed to commit transaction", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to join guild")
		return
	}

	// Publish member add event.
	h.EventBus.PublishJSON(r.Context(), events.SubjectGuildMemberAdd, "GUILD_MEMBER_ADD",
		map[string]interface{}{
			"guild_id":  inv.GuildID,
			"user_id":   userID,
			"joined_at": now,
		})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"guild_id": inv.GuildID,
		"joined":   true,
	})
}

// HandleDeleteInvite handles DELETE /api/v1/invites/{code}.
// Only the invite creator or a guild admin can delete an invite.
func (h *Handler) HandleDeleteInvite(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	userID := auth.UserIDFromContext(r.Context())

	// Get the invite to check permissions.
	var guildID string
	var creatorID *string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT guild_id, creator_id FROM invites WHERE code = $1`, code).Scan(&guildID, &creatorID)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "invite_not_found", "Invite not found")
		return
	}
	if err != nil {
		h.Logger.Error("failed to get invite", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get invite")
		return
	}

	// Check if user is the creator or has MANAGE_GUILD permission.
	isCreator := creatorID != nil && *creatorID == userID
	if !isCreator {
		// Check guild owner or admin.
		var ownerID string
		err = h.Pool.QueryRow(r.Context(),
			`SELECT owner_id FROM guilds WHERE id = $1`, guildID).Scan(&ownerID)
		if err != nil || ownerID != userID {
			// Check MANAGE_GUILD permission via roles.
			var hasManageGuild bool
			err = h.Pool.QueryRow(r.Context(),
				`SELECT EXISTS(
					SELECT 1 FROM guild_member_roles gmr
					JOIN roles r ON r.id = gmr.role_id
					WHERE gmr.guild_id = $1 AND gmr.user_id = $2
					  AND (r.permissions & $3) = $3
				)`, guildID, userID, permissions.ManageGuild).Scan(&hasManageGuild)
			if err != nil || !hasManageGuild {
				writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to delete this invite")
				return
			}
		}
	}

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM invites WHERE code = $1`, code)
	if err != nil {
		h.Logger.Error("failed to delete invite", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete invite")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "invite_not_found", "Invite not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
