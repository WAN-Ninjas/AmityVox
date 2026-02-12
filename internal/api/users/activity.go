// Package users — activity status API for users and bots.
// Allows setting a rich presence activity (playing, listening, watching, streaming).
package users

import (
	"encoding/json"
	"log/slog"
	"net/http"

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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
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
			writeError(w, http.StatusBadRequest, "invalid_activity_type",
				"Activity type must be 'playing', 'listening', 'watching', or 'streaming'")
			return
		}
		// Allow empty string to clear the activity type.
		if *req.ActivityType == "" {
			req.ActivityType = nil
		}
	}

	if req.ActivityName != nil && len(*req.ActivityName) > 128 {
		writeError(w, http.StatusBadRequest, "invalid_activity_name",
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
		h.Logger.Error("failed to update activity", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update activity")
		return
	}

	resp := ActivityResponse{
		UserID:       userID,
		ActivityType: actType,
		ActivityName: actName,
	}

	// Publish presence update so connected clients see the change.
	h.EventBus.PublishJSON(r.Context(), events.SubjectPresenceUpdate, "PRESENCE_UPDATE", resp)

	writeJSON(w, http.StatusOK, resp)
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
		h.Logger.Error("failed to get activity", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get activity")
		return
	}

	writeJSON(w, http.StatusOK, ActivityResponse{
		UserID:       userID,
		ActivityType: actType,
		ActivityName: actName,
	})
}
