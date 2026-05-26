// Package users — badge system for user profiles.
// Badges are derived from the user's flags bitfield and returned as
// human-readable objects for the frontend.
package users

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/amityvox/amityvox/internal/api/apiutil"
	"github.com/amityvox/amityvox/internal/models"
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
	{models.UserFlagAdmin, badge{ID: "admin", Name: "Admin", Icon: "shield"}},
	{models.UserFlagGlobalMod, badge{ID: "moderator", Name: "Moderator", Icon: "hammer"}},
	{models.UserFlagBot, badge{ID: "bot", Name: "Bot", Icon: "robot"}},
	{models.UserFlagVerified, badge{ID: "verified", Name: "Verified", Icon: "check"}},
}

// HandleGetUserBadges returns the badges for a user based on their flags bitfield.
// GET /api/v1/users/{userID}/badges
func (h *Handler) HandleGetUserBadges(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "userID")
	if targetID == "" {
		apiutil.WriteError(w, http.StatusBadRequest, "missing_user_id", "User ID is required")
		return
	}

	var flags int
	err := h.Pool.QueryRow(r.Context(),
		`SELECT flags FROM users WHERE id = $1`, targetID,
	).Scan(&flags)
	if err != nil {
		if err == pgx.ErrNoRows {
			apiutil.WriteError(w, http.StatusNotFound, "user_not_found", "User not found")
			return
		}
		apiutil.InternalError(w, h.Logger, "Failed to get user badges", err)
		return
	}

	badges := make([]badge, 0)
	for _, def := range badgeDefinitions {
		if flags&def.flag != 0 {
			badges = append(badges, def.badge)
		}
	}

	apiutil.WriteJSON(w, http.StatusOK, badges)
}
