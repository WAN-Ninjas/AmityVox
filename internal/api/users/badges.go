// Package users — badge system for user profiles.
// Badges are derived from the user's flags bitfield and returned as
// human-readable objects for the frontend.
package users

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// Badge constants derived from the user flags bitfield.
// These values correspond to bit positions in the users.flags column.
const (
	UserFlagAdmin          = 4   // 1 << 2
	UserBadgeEarlySupporter = 8   // 1 << 3
	UserBadgeServerOwner   = 16  // 1 << 4
	UserBadgeModerator     = 32  // 1 << 5
	UserBadgeBot           = 64  // 1 << 6
	UserBadgeVerified      = 128 // 1 << 7
)

// badge represents a displayable badge on a user profile.
type badge struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Icon string `json:"icon"`
}

// badgeDefinitions maps flag values to their badge display info.
var badgeDefinitions = []struct {
	flag  int
	badge badge
}{
	{UserFlagAdmin, badge{ID: "admin", Name: "Admin", Icon: "shield"}},
	{UserBadgeEarlySupporter, badge{ID: "early_supporter", Name: "Early Supporter", Icon: "heart"}},
	{UserBadgeServerOwner, badge{ID: "server_owner", Name: "Server Owner", Icon: "crown"}},
	{UserBadgeModerator, badge{ID: "moderator", Name: "Moderator", Icon: "hammer"}},
	{UserBadgeBot, badge{ID: "bot", Name: "Bot", Icon: "robot"}},
	{UserBadgeVerified, badge{ID: "verified", Name: "Verified", Icon: "check"}},
}

// HandleGetUserBadges returns the badges for a user based on their flags bitfield.
// GET /api/v1/users/{userID}/badges
func (h *Handler) HandleGetUserBadges(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "userID")
	if targetID == "" {
		writeError(w, http.StatusBadRequest, "missing_user_id", "User ID is required")
		return
	}

	var flags int
	err := h.Pool.QueryRow(r.Context(),
		`SELECT flags FROM users WHERE id = $1`, targetID,
	).Scan(&flags)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "user_not_found", "User not found")
			return
		}
		h.Logger.Error("failed to get user flags", "error", err.Error())
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get user badges")
		return
	}

	badges := make([]badge, 0)
	for _, def := range badgeDefinitions {
		if flags&def.flag != 0 {
			badges = append(badges, def.badge)
		}
	}

	writeJSON(w, http.StatusOK, badges)
}
