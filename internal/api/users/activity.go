// Package users — activity status API for users and bots.
// Allows setting a rich presence activity (playing, listening, watching, streaming).
package users

import (
	"net/http"

	"github.com/amityvox/amityvox/internal/api/apiutil"
	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/events"
)

type updateActivityRequest struct {
	ActivityType *string `json:"activity_type"` // "playing", "listening", "watching", "streaming", or null to clear
	ActivityName *string `json:"activity_name"` // Display text for the activity, or null to clear
}

// ActivityResponse represents the user's current activity status.
type ActivityResponse struct {
	UserID       string  `json:"user_id"`
	ActivityType *string `json:"activity_type,omitempty"`
	ActivityName *string `json:"activity_name,omitempty"`
}

// HandleUpdateActivity sets or clears the authenticated user's activity status.
// PUT /api/v1/users/@me/activity
func (h *Handler) HandleUpdateActivity(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var req updateActivityRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	// Validate activity type if provided.
	if req.ActivityType != nil {
		validTypes := map[string]bool{
			"playing":   true,
			"listening": true,
			"watching":  true,
			"streaming": true,
		}
		if *req.ActivityType != "" && !validTypes[*req.ActivityType] {
			apiutil.WriteError(w, http.StatusBadRequest, "invalid_activity_type",
				"Activity type must be 'playing', 'listening', 'watching', or 'streaming'")
			return
		}
		// Allow empty string to clear the activity type.
		if *req.ActivityType == "" {
			req.ActivityType = nil
		}
	}

	if req.ActivityName != nil && len(*req.ActivityName) > 128 {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_activity_name",
			"Activity name must be at most 128 characters")
		return
	}

	// Allow empty string to clear the activity name.
	if req.ActivityName != nil && *req.ActivityName == "" {
		req.ActivityName = nil
	}

	// Update the user's activity in the database.
	var actType, actName *string
	err := h.Pool.QueryRow(r.Context(),
		`UPDATE users SET
			activity_type = $2,
			activity_name = $3
		 WHERE id = $1
		 RETURNING activity_type, activity_name`,
		userID, req.ActivityType, req.ActivityName,
	).Scan(&actType, &actName)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to update activity", err)
		return
	}

	resp := ActivityResponse{
		UserID:       userID,
		ActivityType: actType,
		ActivityName: actName,
	}

	// Publish presence update so connected clients see the change.
	h.EventBus.PublishUserEvent(r.Context(), events.SubjectPresenceUpdate, "PRESENCE_UPDATE", userID, resp)

	apiutil.WriteJSON(w, http.StatusOK, resp)
}

// HandleGetActivity returns the authenticated user's current activity status.
// GET /api/v1/users/@me/activity
func (h *Handler) HandleGetActivity(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var actType, actName *string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT activity_type, activity_name FROM users WHERE id = $1`,
		userID,
	).Scan(&actType, &actName)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get activity", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, ActivityResponse{
		UserID:       userID,
		ActivityType: actType,
		ActivityName: actName,
	})
}
