// Package onboarding implements REST API handlers for guild onboarding configuration.
// Guild onboarding allows server administrators to set up welcome messages, rules,
// customizable prompts with options that auto-assign roles/channels, and track
// completion status for new members. Mounted under /api/v1/guilds.
package onboarding

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/amityvox/amityvox/internal/api/apiutil"
	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/models"
)


// Handler implements guild onboarding REST API endpoints.
type Handler struct {
	Pool     *pgxpool.Pool
	EventBus *events.Bus
	Logger   *slog.Logger
}

// --- Request types ---

type updateOnboardingRequest struct {
	Enabled           *bool    `json:"enabled"`
	WelcomeMessage    *string  `json:"welcome_message"`
	Rules             any      `json:"rules"`
	DefaultChannelIDs []string `json:"default_channel_ids"`
}

type createPromptRequest struct {
	Title        string                `json:"title"`
	Required     *bool                 `json:"required"`
	SingleSelect *bool                 `json:"single_select"`
	Options      []createOptionRequest `json:"options"`
}

type createOptionRequest struct {
	Label       string   `json:"label"`
	Description *string  `json:"description"`
	Emoji       *string  `json:"emoji"`
	RoleIDs     []string `json:"role_ids"`
	ChannelIDs  []string `json:"channel_ids"`
}

type updatePromptRequest struct {
	Title        *string `json:"title"`
	Required     *bool   `json:"required"`
	SingleSelect *bool   `json:"single_select"`
}

type completeOnboardingRequest struct {
	PromptResponses map[string][]string `json:"prompt_responses"`
}

// --- Response types ---

type onboardingResponse struct {
	GuildID           string           `json:"guild_id"`
	Enabled           bool             `json:"enabled"`
	WelcomeMessage    *string          `json:"welcome_message"`
	Rules             json.RawMessage  `json:"rules"`
	DefaultChannelIDs []string         `json:"default_channel_ids"`
	UpdatedAt         time.Time        `json:"updated_at"`
	Prompts           []promptResponse `json:"prompts"`
}

type promptResponse struct {
	ID           string           `json:"id"`
	GuildID      string           `json:"guild_id"`
	Title        string           `json:"title"`
	Required     bool             `json:"required"`
	SingleSelect bool             `json:"single_select"`
	Position     int              `json:"position"`
	CreatedAt    time.Time        `json:"created_at"`
	Options      []optionResponse `json:"options"`
}

type optionResponse struct {
	ID          string   `json:"id"`
	PromptID    string   `json:"prompt_id"`
	GuildID     string   `json:"guild_id"`
	Label       string   `json:"label"`
	Description *string  `json:"description"`
	Emoji       *string  `json:"emoji"`
	RoleIDs     []string `json:"role_ids"`
	ChannelIDs  []string `json:"channel_ids"`
	CreatedAt   time.Time `json:"created_at"`
}

type onboardingStatusResponse struct {
	Completed   bool       `json:"completed"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// --- Permission helpers ---

// isGuildAdmin checks whether the user is the guild owner or an instance admin.
func (h *Handler) isGuildAdmin(ctx context.Context, guildID, userID string) bool {
	// Check if guild owner.
	var ownerID string
	if err := h.Pool.QueryRow(ctx, `SELECT owner_id FROM guilds WHERE id = $1`, guildID).Scan(&ownerID); err == nil && ownerID == userID {
		return true
	}
	// Check if instance admin (flags & 4).
	var flags int
	h.Pool.QueryRow(ctx, `SELECT flags FROM users WHERE id = $1`, userID).Scan(&flags)
	return flags&models.UserFlagAdmin != 0
}

// isMember checks whether the user is a member of the guild.
func (h *Handler) isMember(ctx context.Context, guildID, userID string) bool {
	var exists bool
	h.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM guild_members WHERE guild_id = $1 AND user_id = $2)`,
		guildID, userID,
	).Scan(&exists)
	return exists
}

// --- Handlers ---

// HandleGetOnboarding returns the full onboarding configuration for a guild,
// including settings, prompts, and their options.
// GET /api/v1/guilds/{guildID}/onboarding
func (h *Handler) HandleGetOnboarding(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isMember(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "You are not a member of this guild")
		return
	}

	// Fetch onboarding settings.
	var resp onboardingResponse
	var rulesRaw []byte
	err := h.Pool.QueryRow(r.Context(),
		`SELECT guild_id, enabled, welcome_message, rules, default_channel_ids, updated_at
		 FROM guild_onboarding
		 WHERE guild_id = $1`,
		guildID,
	).Scan(&resp.GuildID, &resp.Enabled, &resp.WelcomeMessage, &rulesRaw, &resp.DefaultChannelIDs, &resp.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			// Return default empty onboarding config.
			resp = onboardingResponse{
				GuildID:           guildID,
				Enabled:           false,
				WelcomeMessage:    nil,
				Rules:             json.RawMessage("[]"),
				DefaultChannelIDs: []string{},
				UpdatedAt:         time.Now(),
				Prompts:           []promptResponse{},
			}
			apiutil.WriteJSON(w, http.StatusOK, resp)
			return
		}
		apiutil.InternalError(w, h.Logger, "Failed to get onboarding config", err)
		return
	}

	if rulesRaw != nil {
		resp.Rules = json.RawMessage(rulesRaw)
	} else {
		resp.Rules = json.RawMessage("[]")
	}
	if resp.DefaultChannelIDs == nil {
		resp.DefaultChannelIDs = []string{}
	}

	// Fetch prompts.
	promptRows, err := h.Pool.Query(r.Context(),
		`SELECT id, guild_id, title, required, single_select, position, created_at
		 FROM onboarding_prompts
		 WHERE guild_id = $1
		 ORDER BY position ASC, created_at ASC`,
		guildID,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get onboarding prompts", err)
		return
	}
	defer promptRows.Close()

	prompts := make([]promptResponse, 0)
	promptIDs := make([]string, 0)
	promptIndex := make(map[string]int)

	for promptRows.Next() {
		var p promptResponse
		if err := promptRows.Scan(&p.ID, &p.GuildID, &p.Title, &p.Required, &p.SingleSelect, &p.Position, &p.CreatedAt); err != nil {
			apiutil.InternalError(w, h.Logger, "Failed to get onboarding prompts", err)
			return
		}
		p.Options = []optionResponse{}
		promptIndex[p.ID] = len(prompts)
		promptIDs = append(promptIDs, p.ID)
		prompts = append(prompts, p)
	}
	if err := promptRows.Err(); err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get onboarding prompts", err)
		return
	}

	// Batch-load options for all prompts.
	if len(promptIDs) > 0 {
		optionRows, err := h.Pool.Query(r.Context(),
			`SELECT id, prompt_id, guild_id, label, description, emoji, role_ids, channel_ids, created_at
			 FROM onboarding_options
			 WHERE prompt_id = ANY($1)
			 ORDER BY created_at ASC`,
			promptIDs,
		)
		if err != nil {
			apiutil.InternalError(w, h.Logger, "Failed to get onboarding options", err)
			return
		}
		defer optionRows.Close()

		for optionRows.Next() {
			var o optionResponse
			if err := optionRows.Scan(&o.ID, &o.PromptID, &o.GuildID, &o.Label, &o.Description, &o.Emoji, &o.RoleIDs, &o.ChannelIDs, &o.CreatedAt); err != nil {
				apiutil.InternalError(w, h.Logger, "Failed to get onboarding options", err)
				return
			}
			if o.RoleIDs == nil {
				o.RoleIDs = []string{}
			}
			if o.ChannelIDs == nil {
				o.ChannelIDs = []string{}
			}
			if idx, ok := promptIndex[o.PromptID]; ok {
				prompts[idx].Options = append(prompts[idx].Options, o)
			}
		}
		if err := optionRows.Err(); err != nil {
			apiutil.InternalError(w, h.Logger, "Failed to get onboarding options", err)
			return
		}
	}

	resp.Prompts = prompts
	apiutil.WriteJSON(w, http.StatusOK, resp)
}

// HandleUpdateOnboarding updates the onboarding settings for a guild.
// Uses UPSERT to create or update the configuration.
// PUT /api/v1/guilds/{guildID}/onboarding
func (h *Handler) HandleUpdateOnboarding(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isGuildAdmin(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You must be the guild owner or an instance admin")
		return
	}

	var req updateOnboardingRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	// Serialize rules to JSON.
	var rulesJSON []byte
	if req.Rules != nil {
		var err error
		rulesJSON, err = json.Marshal(req.Rules)
		if err != nil {
			apiutil.WriteError(w, http.StatusBadRequest, "invalid_rules", "Rules must be valid JSON")
			return
		}
	}

	// Normalize default_channel_ids to non-nil.
	if req.DefaultChannelIDs == nil {
		req.DefaultChannelIDs = []string{}
	}

	var resp onboardingResponse
	var rulesRaw []byte
	err := h.Pool.QueryRow(r.Context(),
		`INSERT INTO guild_onboarding (guild_id, enabled, welcome_message, rules, default_channel_ids, updated_at)
		 VALUES ($1, COALESCE($2, false), $3, COALESCE($4, '[]'::jsonb), $5, now())
		 ON CONFLICT (guild_id) DO UPDATE SET
		     enabled = COALESCE($2, guild_onboarding.enabled),
		     welcome_message = COALESCE($3, guild_onboarding.welcome_message),
		     rules = COALESCE($4, guild_onboarding.rules),
		     default_channel_ids = COALESCE($5, guild_onboarding.default_channel_ids),
		     updated_at = now()
		 RETURNING guild_id, enabled, welcome_message, rules, default_channel_ids, updated_at`,
		guildID, req.Enabled, req.WelcomeMessage, rulesJSON, req.DefaultChannelIDs,
	).Scan(&resp.GuildID, &resp.Enabled, &resp.WelcomeMessage, &rulesRaw, &resp.DefaultChannelIDs, &resp.UpdatedAt)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to update onboarding config", err)
		return
	}

	if rulesRaw != nil {
		resp.Rules = json.RawMessage(rulesRaw)
	} else {
		resp.Rules = json.RawMessage("[]")
	}
	if resp.DefaultChannelIDs == nil {
		resp.DefaultChannelIDs = []string{}
	}

	// Publish event so connected clients see the change.
	h.EventBus.PublishGuildEvent(r.Context(), events.SubjectGuildUpdate, "GUILD_ONBOARDING_UPDATE", guildID, map[string]interface{}{
		"guild_id":            guildID,
		"enabled":             resp.Enabled,
		"welcome_message":     resp.WelcomeMessage,
		"rules":               resp.Rules,
		"default_channel_ids": resp.DefaultChannelIDs,
		"updated_at":          resp.UpdatedAt,
	})

	apiutil.WriteJSON(w, http.StatusOK, resp)
}

// HandleCreatePrompt creates a new onboarding prompt with its options in a single request.
// POST /api/v1/guilds/{guildID}/onboarding/prompts
func (h *Handler) HandleCreatePrompt(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isGuildAdmin(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You must be the guild owner or an instance admin")
		return
	}

	var req createPromptRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	if !apiutil.RequireNonEmpty(w, "Prompt title", req.Title) {
		return
	}

	required := false
	if req.Required != nil {
		required = *req.Required
	}
	singleSelect := false
	if req.SingleSelect != nil {
		singleSelect = *req.SingleSelect
	}

	// Determine next position.
	var maxPos int
	h.Pool.QueryRow(r.Context(),
		`SELECT COALESCE(MAX(position), -1) FROM onboarding_prompts WHERE guild_id = $1`,
		guildID,
	).Scan(&maxPos)
	position := maxPos + 1

	// Use a transaction to create the prompt and its options atomically.
	promptID := models.NewULID().String()
	now := time.Now()
	options := make([]optionResponse, 0, len(req.Options))

	err := apiutil.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(r.Context(),
			`INSERT INTO onboarding_prompts (id, guild_id, title, required, single_select, position, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			promptID, guildID, req.Title, required, singleSelect, position, now,
		)
		if err != nil {
			return err
		}

		for _, opt := range req.Options {
			if opt.Label == "" {
				continue
			}
			optionID := models.NewULID().String()
			optNow := time.Now()

			roleIDs := opt.RoleIDs
			if roleIDs == nil {
				roleIDs = []string{}
			}
			channelIDs := opt.ChannelIDs
			if channelIDs == nil {
				channelIDs = []string{}
			}

			_, err = tx.Exec(r.Context(),
				`INSERT INTO onboarding_options (id, prompt_id, guild_id, label, description, emoji, role_ids, channel_ids, created_at)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
				optionID, promptID, guildID, opt.Label, opt.Description, opt.Emoji, roleIDs, channelIDs, optNow,
			)
			if err != nil {
				return err
			}

			options = append(options, optionResponse{
				ID:          optionID,
				PromptID:    promptID,
				GuildID:     guildID,
				Label:       opt.Label,
				Description: opt.Description,
				Emoji:       opt.Emoji,
				RoleIDs:     roleIDs,
				ChannelIDs:  channelIDs,
				CreatedAt:   optNow,
			})
		}

		return nil
	})
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to create prompt", err)
		return
	}

	resp := promptResponse{
		ID:           promptID,
		GuildID:      guildID,
		Title:        req.Title,
		Required:     required,
		SingleSelect: singleSelect,
		Position:     position,
		CreatedAt:    now,
		Options:      options,
	}

	apiutil.WriteJSON(w, http.StatusCreated, resp)
}

// HandleUpdatePrompt updates an existing onboarding prompt's settings.
// PUT /api/v1/guilds/{guildID}/onboarding/prompts/{promptID}
func (h *Handler) HandleUpdatePrompt(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	promptID := chi.URLParam(r, "promptID")

	if !h.isGuildAdmin(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You must be the guild owner or an instance admin")
		return
	}

	var req updatePromptRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	var prompt promptResponse
	err := h.Pool.QueryRow(r.Context(),
		`UPDATE onboarding_prompts SET
		     title = COALESCE($3, title),
		     required = COALESCE($4, required),
		     single_select = COALESCE($5, single_select)
		 WHERE id = $1 AND guild_id = $2
		 RETURNING id, guild_id, title, required, single_select, position, created_at`,
		promptID, guildID, req.Title, req.Required, req.SingleSelect,
	).Scan(&prompt.ID, &prompt.GuildID, &prompt.Title, &prompt.Required, &prompt.SingleSelect, &prompt.Position, &prompt.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			apiutil.WriteError(w, http.StatusNotFound, "prompt_not_found", "Prompt not found")
			return
		}
		apiutil.InternalError(w, h.Logger, "Failed to update prompt", err)
		return
	}

	// Load the prompt's options for the response.
	optionRows, err := h.Pool.Query(r.Context(),
		`SELECT id, prompt_id, guild_id, label, description, emoji, role_ids, channel_ids, created_at
		 FROM onboarding_options
		 WHERE prompt_id = $1
		 ORDER BY created_at ASC`,
		promptID,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to load prompt options", err)
		return
	}
	defer optionRows.Close()

	prompt.Options = make([]optionResponse, 0)
	for optionRows.Next() {
		var o optionResponse
		if err := optionRows.Scan(&o.ID, &o.PromptID, &o.GuildID, &o.Label, &o.Description, &o.Emoji, &o.RoleIDs, &o.ChannelIDs, &o.CreatedAt); err != nil {
			h.Logger.Error("failed to scan option row", slog.String("error", err.Error()))
			continue
		}
		if o.RoleIDs == nil {
			o.RoleIDs = []string{}
		}
		if o.ChannelIDs == nil {
			o.ChannelIDs = []string{}
		}
		prompt.Options = append(prompt.Options, o)
	}

	apiutil.WriteJSON(w, http.StatusOK, prompt)
}

// HandleDeletePrompt deletes an onboarding prompt and its options (cascading via FK).
// DELETE /api/v1/guilds/{guildID}/onboarding/prompts/{promptID}
func (h *Handler) HandleDeletePrompt(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	promptID := chi.URLParam(r, "promptID")

	if !h.isGuildAdmin(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You must be the guild owner or an instance admin")
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM onboarding_prompts WHERE id = $1 AND guild_id = $2`,
		promptID, guildID,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to delete prompt", err)
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "prompt_not_found", "Prompt not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleCompleteOnboarding processes a new member's onboarding responses.
// For each selected option, the option's role_ids are assigned to the member.
// Records the completion in onboarding_completions.
// POST /api/v1/guilds/{guildID}/onboarding/complete
func (h *Handler) HandleCompleteOnboarding(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isMember(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "You are not a member of this guild")
		return
	}

	// Check if already completed.
	var alreadyCompleted bool
	h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM onboarding_completions WHERE guild_id = $1 AND user_id = $2)`,
		guildID, userID,
	).Scan(&alreadyCompleted)
	if alreadyCompleted {
		apiutil.WriteError(w, http.StatusConflict, "already_completed", "You have already completed onboarding for this guild")
		return
	}

	var req completeOnboardingRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	// Collect all selected option IDs for batch lookup.
	allOptionIDs := make([]string, 0)
	for _, optionIDs := range req.PromptResponses {
		allOptionIDs = append(allOptionIDs, optionIDs...)
	}

	err := apiutil.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		// Batch-load role_ids for all selected options.
		if len(allOptionIDs) > 0 {
			optRows, err := tx.Query(r.Context(),
				`SELECT id, role_ids FROM onboarding_options
				 WHERE id = ANY($1) AND guild_id = $2`,
				allOptionIDs, guildID,
			)
			if err != nil {
				return err
			}
			defer optRows.Close()

			// Collect all role IDs to assign (deduplicated).
			roleSet := make(map[string]struct{})
			for optRows.Next() {
				var optionID string
				var roleIDs []string
				if err := optRows.Scan(&optionID, &roleIDs); err != nil {
					h.Logger.Error("failed to scan option role_ids", slog.String("error", err.Error()))
					continue
				}
				for _, roleID := range roleIDs {
					roleSet[roleID] = struct{}{}
				}
			}
			if err := optRows.Err(); err != nil {
				return err
			}

			// Assign roles to the member.
			for roleID := range roleSet {
				_, err := tx.Exec(r.Context(),
					`INSERT INTO member_roles (guild_id, user_id, role_id)
					 VALUES ($1, $2, $3)
					 ON CONFLICT DO NOTHING`,
					guildID, userID, roleID,
				)
				if err != nil {
					h.Logger.Error("failed to assign role from onboarding",
						slog.String("role_id", roleID),
						slog.String("error", err.Error()),
					)
					// Continue assigning other roles; do not fail the whole operation.
				}
			}
		}

		// Record completion.
		_, err := tx.Exec(r.Context(),
			`INSERT INTO onboarding_completions (guild_id, user_id, completed_at)
			 VALUES ($1, $2, now())
			 ON CONFLICT DO NOTHING`,
			guildID, userID,
		)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to complete onboarding", err)
		return
	}

	// Publish event for real-time update.
	h.EventBus.PublishGuildEvent(r.Context(), events.SubjectGuildMemberUpdate, "GUILD_MEMBER_UPDATE", guildID, map[string]interface{}{
		"guild_id":             guildID,
		"user_id":              userID,
		"onboarding_completed": true,
	})

	w.WriteHeader(http.StatusNoContent)
}

// HandleGetOnboardingStatus returns whether the current user has completed
// onboarding for this guild.
// GET /api/v1/guilds/{guildID}/onboarding/status
func (h *Handler) HandleGetOnboardingStatus(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isMember(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "You are not a member of this guild")
		return
	}

	var completedAt *time.Time
	err := h.Pool.QueryRow(r.Context(),
		`SELECT completed_at FROM onboarding_completions
		 WHERE guild_id = $1 AND user_id = $2`,
		guildID, userID,
	).Scan(&completedAt)

	if err != nil && err != pgx.ErrNoRows {
		apiutil.InternalError(w, h.Logger, "Failed to get onboarding status", err)
		return
	}

	resp := onboardingStatusResponse{
		Completed:   completedAt != nil,
		CompletedAt: completedAt,
	}

	apiutil.WriteJSON(w, http.StatusOK, resp)
}
