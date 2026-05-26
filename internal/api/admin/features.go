package admin

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/amityvox/amityvox/internal/api/apiutil"
	"github.com/amityvox/amityvox/internal/features"
)

type updateInstanceFeatureRequest struct {
	Enabled      *bool `json:"enabled"`
	HardDisabled *bool `json:"hard_disabled"`
}

// HandleGetFeatureFlags returns instance feature availability settings.
func (h *Handler) HandleGetFeatureFlags(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}
	states, err := features.Resolve(r.Context(), h.Pool, "")
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to load feature flags", err)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, map[string]any{"features": states})
}

// HandleUpdateFeatureFlag updates one instance-level feature flag.
func (h *Handler) HandleUpdateFeatureFlag(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}
	featureKey := chi.URLParam(r, "featureKey")
	if !features.IsKnown(featureKey) {
		apiutil.WriteError(w, http.StatusBadRequest, "unknown_feature", "Unknown feature key")
		return
	}

	var req updateInstanceFeatureRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}
	if req.Enabled == nil && req.HardDisabled == nil {
		apiutil.WriteError(w, http.StatusBadRequest, "empty_update", "No feature flag fields provided")
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	hardDisabled := false
	if req.HardDisabled != nil {
		hardDisabled = *req.HardDisabled
	}

	_, err := h.Pool.Exec(r.Context(), `
		INSERT INTO instance_feature_flags (feature_key, enabled, hard_disabled, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (feature_key) DO UPDATE SET
			enabled = COALESCE($4, instance_feature_flags.enabled),
			hard_disabled = COALESCE($5, instance_feature_flags.hard_disabled),
			updated_at = now()`,
		featureKey, enabled, hardDisabled, req.Enabled, req.HardDisabled)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to update feature flag", err)
		return
	}
	states, err := features.Resolve(r.Context(), h.Pool, "")
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to reload feature flags", err)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, states[featureKey])
}
