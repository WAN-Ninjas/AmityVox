package automod

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/models"
)

// HandleListRules handles GET /api/v1/guilds/{guildID}/automod/rules.
func (s *Service) HandleListRules(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")

	rows, err := s.pool.Query(r.Context(),
		`SELECT id, guild_id, name, enabled, rule_type, config, action,
		        timeout_duration_seconds, exempt_channel_ids, exempt_role_ids,
		        created_by, created_at, updated_at
		 FROM automod_rules
		 WHERE guild_id = $1
		 ORDER BY created_at ASC`,
		guildID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to query rules")
		return
	}
	defer rows.Close()

	rules := []Rule{}
	for rows.Next() {
		var r Rule
		var configJSON []byte
		var exemptChannels, exemptRoles []string
		var createdBy *string

		if err := rows.Scan(
			&r.ID, &r.GuildID, &r.Name, &r.Enabled, &r.RuleType,
			&configJSON, &r.Action, &r.TimeoutDurationSeconds,
			&exemptChannels, &exemptRoles,
			&createdBy, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			continue
		}

		json.Unmarshal(configJSON, &r.Config)
		r.ExemptChannelIDs = exemptChannels
		r.ExemptRoleIDs = exemptRoles
		if createdBy != nil {
			r.CreatedBy = *createdBy
		}
		rules = append(rules, r)
	}

	writeJSON(w, http.StatusOK, rules)
}

// HandleCreateRule handles POST /api/v1/guilds/{guildID}/automod/rules.
func (s *Service) HandleCreateRule(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")
	userID := auth.UserIDFromContext(r.Context())

	var req struct {
		Name                   string     `json:"name"`
		RuleType               string     `json:"rule_type"`
		Config                 RuleConfig `json:"config"`
		Action                 string     `json:"action"`
		Enabled                *bool      `json:"enabled"`
		TimeoutDurationSeconds int        `json:"timeout_duration_seconds"`
		ExemptChannelIDs       []string   `json:"exempt_channel_ids"`
		ExemptRoleIDs          []string   `json:"exempt_role_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Name == "" || req.RuleType == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "name and rule_type are required")
		return
	}

	// Validate rule type.
	validTypes := map[string]bool{
		RuleWordFilter: true, RuleRegexFilter: true, RuleInviteFilter: true,
		RuleMentionSpam: true, RuleCapsFilter: true, RuleSpamFilter: true,
		RuleLinkFilter: true,
	}
	if !validTypes[req.RuleType] {
		writeError(w, http.StatusBadRequest, "invalid_type", "Invalid rule_type")
		return
	}

	action := req.Action
	if action == "" {
		action = ActionDelete
	}
	validActions := map[string]bool{
		ActionDelete: true, ActionWarn: true, ActionTimeout: true, ActionLog: true,
	}
	if !validActions[action] {
		writeError(w, http.StatusBadRequest, "invalid_action", "Invalid action")
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	configJSON, _ := json.Marshal(req.Config)
	if req.ExemptChannelIDs == nil {
		req.ExemptChannelIDs = []string{}
	}
	if req.ExemptRoleIDs == nil {
		req.ExemptRoleIDs = []string{}
	}

	id := models.NewULID().String()
	_, err := s.pool.Exec(r.Context(),
		`INSERT INTO automod_rules (id, guild_id, name, enabled, rule_type, config, action,
		 timeout_duration_seconds, exempt_channel_ids, exempt_role_ids, created_by, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, now(), now())`,
		id, guildID, req.Name, enabled, req.RuleType, configJSON, action,
		req.TimeoutDurationSeconds, req.ExemptChannelIDs, req.ExemptRoleIDs, userID,
	)
	if err != nil {
		s.logger.Error("failed to create automod rule", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create rule")
		return
	}

	writeJSON(w, http.StatusCreated, Rule{
		ID:                     id,
		GuildID:                guildID,
		Name:                   req.Name,
		Enabled:                enabled,
		RuleType:               req.RuleType,
		Config:                 req.Config,
		Action:                 action,
		TimeoutDurationSeconds: req.TimeoutDurationSeconds,
		ExemptChannelIDs:       req.ExemptChannelIDs,
		ExemptRoleIDs:          req.ExemptRoleIDs,
		CreatedBy:              userID,
		CreatedAt:              time.Now().UTC(),
		UpdatedAt:              time.Now().UTC(),
	})
}

// HandleUpdateRule handles PATCH /api/v1/guilds/{guildID}/automod/rules/{ruleID}.
func (s *Service) HandleUpdateRule(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")
	ruleID := chi.URLParam(r, "ruleID")

	var req struct {
		Name                   *string     `json:"name"`
		Enabled                *bool       `json:"enabled"`
		Config                 *RuleConfig `json:"config"`
		Action                 *string     `json:"action"`
		TimeoutDurationSeconds *int        `json:"timeout_duration_seconds"`
		ExemptChannelIDs       *[]string   `json:"exempt_channel_ids"`
		ExemptRoleIDs          *[]string   `json:"exempt_role_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	// Verify rule exists and belongs to guild.
	var exists bool
	s.pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM automod_rules WHERE id = $1 AND guild_id = $2)`,
		ruleID, guildID,
	).Scan(&exists)

	if !exists {
		writeError(w, http.StatusNotFound, "not_found", "Rule not found")
		return
	}

	// Build dynamic update.
	if req.Name != nil {
		s.pool.Exec(r.Context(),
			`UPDATE automod_rules SET name = $1, updated_at = now() WHERE id = $2`,
			*req.Name, ruleID)
	}
	if req.Enabled != nil {
		s.pool.Exec(r.Context(),
			`UPDATE automod_rules SET enabled = $1, updated_at = now() WHERE id = $2`,
			*req.Enabled, ruleID)
	}
	if req.Config != nil {
		configJSON, _ := json.Marshal(req.Config)
		s.pool.Exec(r.Context(),
			`UPDATE automod_rules SET config = $1, updated_at = now() WHERE id = $2`,
			configJSON, ruleID)
	}
	if req.Action != nil {
		s.pool.Exec(r.Context(),
			`UPDATE automod_rules SET action = $1, updated_at = now() WHERE id = $2`,
			*req.Action, ruleID)
	}
	if req.TimeoutDurationSeconds != nil {
		s.pool.Exec(r.Context(),
			`UPDATE automod_rules SET timeout_duration_seconds = $1, updated_at = now() WHERE id = $2`,
			*req.TimeoutDurationSeconds, ruleID)
	}
	if req.ExemptChannelIDs != nil {
		s.pool.Exec(r.Context(),
			`UPDATE automod_rules SET exempt_channel_ids = $1, updated_at = now() WHERE id = $2`,
			*req.ExemptChannelIDs, ruleID)
	}
	if req.ExemptRoleIDs != nil {
		s.pool.Exec(r.Context(),
			`UPDATE automod_rules SET exempt_role_ids = $1, updated_at = now() WHERE id = $2`,
			*req.ExemptRoleIDs, ruleID)
	}

	// Fetch updated rule.
	var rule Rule
	var configJSON []byte
	var exemptChannels, exemptRoles []string
	var createdBy *string
	err := s.pool.QueryRow(r.Context(),
		`SELECT id, guild_id, name, enabled, rule_type, config, action,
		        timeout_duration_seconds, exempt_channel_ids, exempt_role_ids,
		        created_by, created_at, updated_at
		 FROM automod_rules WHERE id = $1`,
		ruleID,
	).Scan(
		&rule.ID, &rule.GuildID, &rule.Name, &rule.Enabled, &rule.RuleType,
		&configJSON, &rule.Action, &rule.TimeoutDurationSeconds,
		&exemptChannels, &exemptRoles,
		&createdBy, &rule.CreatedAt, &rule.UpdatedAt,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to fetch updated rule")
		return
	}

	json.Unmarshal(configJSON, &rule.Config)
	rule.ExemptChannelIDs = exemptChannels
	rule.ExemptRoleIDs = exemptRoles
	if createdBy != nil {
		rule.CreatedBy = *createdBy
	}

	writeJSON(w, http.StatusOK, rule)
}

// HandleDeleteRule handles DELETE /api/v1/guilds/{guildID}/automod/rules/{ruleID}.
func (s *Service) HandleDeleteRule(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")
	ruleID := chi.URLParam(r, "ruleID")

	result, err := s.pool.Exec(r.Context(),
		`DELETE FROM automod_rules WHERE id = $1 AND guild_id = $2`,
		ruleID, guildID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete rule")
		return
	}
	if result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Rule not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleGetActions handles GET /api/v1/guilds/{guildID}/automod/actions.
func (s *Service) HandleGetActions(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")

	rows, err := s.pool.Query(r.Context(),
		`SELECT id, guild_id, rule_id, channel_id, message_id, user_id, action, reason, created_at
		 FROM automod_actions
		 WHERE guild_id = $1
		 ORDER BY created_at DESC
		 LIMIT 100`,
		guildID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to query actions")
		return
	}
	defer rows.Close()

	actions := []ActionRecord{}
	for rows.Next() {
		var a ActionRecord
		var messageID, reason *string
		if err := rows.Scan(&a.ID, &a.GuildID, &a.RuleID, &a.ChannelID, &messageID, &a.UserID, &a.Action, &reason, &a.CreatedAt); err != nil {
			continue
		}
		if messageID != nil {
			a.MessageID = *messageID
		}
		if reason != nil {
			a.Reason = *reason
		}
		actions = append(actions, a)
	}

	writeJSON(w, http.StatusOK, actions)
}

// HandleGetRule handles GET /api/v1/guilds/{guildID}/automod/rules/{ruleID}.
func (s *Service) HandleGetRule(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")
	ruleID := chi.URLParam(r, "ruleID")

	var rule Rule
	var configJSON []byte
	var exemptChannels, exemptRoles []string
	var createdBy *string

	err := s.pool.QueryRow(r.Context(),
		`SELECT id, guild_id, name, enabled, rule_type, config, action,
		        timeout_duration_seconds, exempt_channel_ids, exempt_role_ids,
		        created_by, created_at, updated_at
		 FROM automod_rules WHERE id = $1 AND guild_id = $2`,
		ruleID, guildID,
	).Scan(
		&rule.ID, &rule.GuildID, &rule.Name, &rule.Enabled, &rule.RuleType,
		&configJSON, &rule.Action, &rule.TimeoutDurationSeconds,
		&exemptChannels, &exemptRoles,
		&createdBy, &rule.CreatedAt, &rule.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "not_found", "Rule not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to query rule")
		return
	}

	json.Unmarshal(configJSON, &rule.Config)
	rule.ExemptChannelIDs = exemptChannels
	rule.ExemptRoleIDs = exemptRoles
	if createdBy != nil {
		rule.CreatedBy = *createdBy
	}

	writeJSON(w, http.StatusOK, rule)
}

// HandleTestRule handles POST /api/v1/guilds/{guildID}/automod/rules/test.
// It evaluates sample text against a rule configuration without taking any action,
// allowing admins to preview what messages would match before enabling a rule.
func (s *Service) HandleTestRule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RuleType   string     `json:"rule_type"`
		Config     RuleConfig `json:"config"`
		SampleText string     `json:"sample_text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.SampleText == "" {
		writeError(w, http.StatusBadRequest, "missing_sample", "sample_text is required")
		return
	}

	if req.RuleType == "" {
		writeError(w, http.StatusBadRequest, "missing_type", "rule_type is required")
		return
	}

	// Validate rule type.
	validTypes := map[string]bool{
		RuleWordFilter: true, RuleRegexFilter: true, RuleInviteFilter: true,
		RuleMentionSpam: true, RuleCapsFilter: true, RuleLinkFilter: true,
	}
	if !validTypes[req.RuleType] {
		// spam_filter requires stateful tracking and cannot be meaningfully tested
		// against a single sample message.
		if req.RuleType == RuleSpamFilter {
			writeError(w, http.StatusBadRequest, "unsupported_type",
				"Spam filter cannot be tested with a single message (it requires message history)")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_type", "Invalid rule_type for testing")
		return
	}

	// Build a temporary rule and evaluate it.
	rule := &Rule{
		RuleType: req.RuleType,
		Config:   req.Config,
	}

	matched, reason := s.checkRule(rule, MessageContext{
		Content: req.SampleText,
	})

	type testResult struct {
		Matched        bool    `json:"matched"`
		MatchedContent *string `json:"matched_content"`
	}

	result := testResult{Matched: matched}
	if matched && reason != "" {
		result.MatchedContent = &reason
	}

	writeJSON(w, http.StatusOK, result)
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
