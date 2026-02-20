// Package invites implements REST API handlers for invite operations including
// looking up invites by code, accepting invites to join guilds, and deleting
// invites. Mounted under /api/v1/invites.
package invites

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/amityvox/amityvox/internal/api/apiutil"
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

// parseRemoteInvite checks if an invite code contains a remote domain.
// Supported formats: "CODE@domain.com" or "domain.com/CODE".
// Returns (localCode, domain, isRemote).
func parseRemoteInvite(code string) (string, string, bool) {
	// Format: CODE@domain.com
	if idx := strings.LastIndex(code, "@"); idx > 0 && idx < len(code)-1 {
		localCode := code[:idx]
		domain := code[idx+1:]
		if strings.Contains(domain, ".") {
			return localCode, domain, true
		}
	}
	// Format: domain.com/CODE (domain must contain a dot, code must not)
	if idx := strings.Index(code, "/"); idx > 0 && idx < len(code)-1 {
		domain := code[:idx]
		localCode := code[idx+1:]
		if strings.Contains(domain, ".") && !strings.Contains(localCode, "/") {
			return localCode, domain, true
		}
	}
	return code, "", false
}

// HandleGetInvite handles GET /api/v1/invites/{code}.
// Returns the invite info including guild name and member count.
// Supports remote invite formats: CODE@domain.com or domain.com/CODE.
func (h *Handler) HandleGetInvite(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	localCode, domain, isRemote := parseRemoteInvite(code)

	if isRemote {
		h.handleGetRemoteInvite(w, r, localCode, domain)
		return
	}

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
		apiutil.WriteError(w, http.StatusNotFound, "invite_not_found", "Invite not found or has expired")
		return
	}
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get invite", err)
		return
	}

	if inv.IsExpired() {
		apiutil.WriteError(w, http.StatusNotFound, "invite_expired", "This invite has expired")
		return
	}
	if inv.MaxUses != nil && inv.Uses >= *inv.MaxUses {
		apiutil.WriteError(w, http.StatusNotFound, "invite_exhausted", "This invite has reached its maximum uses")
		return
	}

	// Enrich with guild info.
	var guildName string
	var memberCount int
	err = h.Pool.QueryRow(r.Context(),
		`SELECT g.name, g.member_count
		 FROM guilds g WHERE g.id = $1`, inv.GuildID).Scan(&guildName, &memberCount)
	if err != nil {
		guildName = "Unknown"
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"invite":       inv,
		"guild_name":   guildName,
		"member_count": memberCount,
	})
}

// handleGetRemoteInvite proxies an invite lookup to a remote federated instance.
func (h *Handler) handleGetRemoteInvite(w http.ResponseWriter, r *http.Request, code, domain string) {
	ctx := r.Context()

	// Check if we're federated with this domain.
	var peerID string
	var peerStatus string
	err := h.Pool.QueryRow(ctx,
		`SELECT fp.peer_id, fp.status FROM federation_peers fp
		 JOIN instances i ON i.id = fp.peer_id
		 WHERE fp.instance_id = $1 AND i.domain = $2`,
		h.InstanceID, domain,
	).Scan(&peerID, &peerStatus)

	if err != nil || peerStatus != "active" {
		apiutil.WriteJSON(w, http.StatusForbidden, map[string]interface{}{
			"error": map[string]interface{}{
				"code":    "not_federated",
				"message": "This instance is not federated with " + domain,
				"domain":  domain,
			},
		})
		return
	}

	// We're federated — return a response indicating this is a remote invite.
	// The frontend will use joinFederatedGuild(domain, null, code) to accept.
	apiutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"federated":       true,
		"instance_domain": domain,
		"invite_code":     code,
		"guild_name":      "Server on " + domain,
		"member_count":    0,
	})
}

// HandleAcceptInvite handles POST /api/v1/invites/{code}.
// Joins the authenticated user to the guild associated with the invite.
// For remote invites, returns instructions for the client to use the federation join endpoint.
func (h *Handler) HandleAcceptInvite(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	userID := auth.UserIDFromContext(r.Context())

	localCode, domain, isRemote := parseRemoteInvite(code)

	if isRemote {
		h.handleAcceptRemoteInvite(w, r, userID, localCode, domain)
		return
	}

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
		apiutil.WriteError(w, http.StatusNotFound, "invite_not_found", "Invite not found")
		return
	}
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get invite", err)
		return
	}

	if inv.IsExpired() {
		apiutil.WriteError(w, http.StatusGone, "invite_expired", "This invite has expired")
		return
	}
	if inv.MaxUses != nil && inv.Uses >= *inv.MaxUses {
		apiutil.WriteError(w, http.StatusGone, "invite_exhausted", "This invite has reached its maximum uses")
		return
	}

	// Check if user is banned from this guild.
	var banned bool
	h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM guild_bans WHERE guild_id = $1 AND user_id = $2)`,
		inv.GuildID, userID).Scan(&banned)
	if banned {
		apiutil.WriteError(w, http.StatusForbidden, "banned", "You are banned from this guild")
		return
	}

	// Check if already a member.
	var exists bool
	err = h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM guild_members WHERE guild_id = $1 AND user_id = $2)`,
		inv.GuildID, userID).Scan(&exists)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to check membership", err)
		return
	}
	if exists {
		apiutil.WriteError(w, http.StatusConflict, "already_member", "You are already a member of this guild")
		return
	}

	// Check max members limit.
	var maxMembers, currentMembers int
	h.Pool.QueryRow(r.Context(),
		`SELECT max_members, member_count
		 FROM guilds WHERE id = $1`, inv.GuildID).Scan(&maxMembers, &currentMembers)
	if maxMembers > 0 && currentMembers >= maxMembers {
		apiutil.WriteError(w, http.StatusForbidden, "guild_full", "This guild has reached its maximum member count")
		return
	}

	now := time.Now().UTC()
	err = apiutil.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		// Add guild member.
		_, err := tx.Exec(r.Context(),
			`INSERT INTO guild_members (guild_id, user_id, nickname, joined_at, deaf, mute)
			 VALUES ($1, $2, NULL, $3, false, false)`,
			inv.GuildID, userID, now)
		if err != nil {
			return err
		}

		// Increment invite usage.
		_, err = tx.Exec(r.Context(),
			`UPDATE invites SET uses = uses + 1 WHERE code = $1`, code)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to join guild", err)
		return
	}

	// Publish member add event.
	h.EventBus.PublishGuildEvent(r.Context(), events.SubjectGuildMemberAdd, "GUILD_MEMBER_ADD", inv.GuildID,
		map[string]interface{}{
			"guild_id":  inv.GuildID,
			"user_id":   userID,
			"joined_at": now,
		})

	apiutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"guild_id": inv.GuildID,
		"joined":   true,
	})
}

// handleAcceptRemoteInvite checks federation status for a remote invite and
// returns instructions for the client to proceed via the federation join endpoint.
func (h *Handler) handleAcceptRemoteInvite(w http.ResponseWriter, r *http.Request, userID, code, domain string) {
	ctx := r.Context()

	// Check federation status.
	var peerStatus string
	err := h.Pool.QueryRow(ctx,
		`SELECT fp.status FROM federation_peers fp
		 JOIN instances i ON i.id = fp.peer_id
		 WHERE fp.instance_id = $1 AND i.domain = $2`,
		h.InstanceID, domain,
	).Scan(&peerStatus)

	if err != nil || peerStatus != "active" {
		apiutil.WriteJSON(w, http.StatusForbidden, map[string]interface{}{
			"error": map[string]interface{}{
				"code":    "not_federated",
				"message": "This instance is not federated with " + domain,
				"domain":  domain,
			},
		})
		return
	}

	// Return redirect to federation join endpoint.
	// The client should call POST /api/v1/federation/guilds/join with
	// { instance_domain, invite_code }.
	apiutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"federated":       true,
		"instance_domain": domain,
		"invite_code":     code,
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
		apiutil.WriteError(w, http.StatusNotFound, "invite_not_found", "Invite not found")
		return
	}
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get invite", err)
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
				apiutil.WriteError(w, http.StatusForbidden, "forbidden", "You do not have permission to delete this invite")
				return
			}
		}
	}

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM invites WHERE code = $1`, code)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to delete invite", err)
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "invite_not_found", "Invite not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
