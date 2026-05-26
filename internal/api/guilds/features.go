package guilds

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/amityvox/amityvox/internal/api/apiutil"
	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/features"
	"github.com/amityvox/amityvox/internal/permissions"
)

type updateGuildFeatureRequest struct {
	Enabled *bool `json:"enabled"`
}

// HandleGetFeatureFlags returns effective feature availability for a guild.
func (h *Handler) HandleGetFeatureFlags(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")
	userID := auth.UserIDFromContext(r.Context())
	if !h.isMember(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}
	states, err := features.Resolve(r.Context(), h.Pool, guildID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to load guild feature flags", err)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, map[string]any{"features": states})
}

// HandleUpdateFeatureFlag updates a guild-level feature flag.
func (h *Handler) HandleUpdateFeatureFlag(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")
	featureKey := chi.URLParam(r, "featureKey")
	userID := auth.UserIDFromContext(r.Context())
	if !h.hasGuildPermission(r.Context(), guildID, userID, permissions.ManageGuild) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_GUILD permission")
		return
	}
	if !features.IsKnown(featureKey) {
		apiutil.WriteError(w, http.StatusBadRequest, "unknown_feature", "Unknown feature key")
		return
	}
	var req updateGuildFeatureRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}
	if req.Enabled == nil {
		apiutil.WriteError(w, http.StatusBadRequest, "empty_update", "No feature flag fields provided")
		return
	}
	_, err := h.Pool.Exec(r.Context(), `
		INSERT INTO guild_feature_flags (guild_id, feature_key, enabled, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (guild_id, feature_key) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			updated_at = now()`,
		guildID, featureKey, *req.Enabled)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to update guild feature flag", err)
		return
	}
	states, err := features.Resolve(r.Context(), h.Pool, guildID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to reload guild feature flags", err)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, states[featureKey])
}
