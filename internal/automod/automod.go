// Package automod implements server-side content moderation for AmityVox guilds.
// Guild admins configure rules (word filters, regex patterns, spam detection, etc.)
// and the automod engine evaluates each message against enabled rules. Actions
// include deleting the message, warning the user, applying a timeout, or logging
// to a mod channel. Encrypted channels are exempt since the server cannot read
// their content.
package automod

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/models"
)

// Rule types.
const (
	RuleWordFilter   = "word_filter"
	RuleRegexFilter  = "regex_filter"
	RuleInviteFilter = "invite_filter"
	RuleMentionSpam  = "mention_spam"
	RuleCapsFilter   = "caps_filter"
	RuleSpamFilter   = "spam_filter"
	RuleLinkFilter   = "link_filter"
)

// Actions that can be taken when a rule triggers.
const (
	ActionDelete  = "delete"
	ActionWarn    = "warn"
	ActionTimeout = "timeout"
	ActionLog     = "log"
)

// Rule represents an automod rule configured for a guild.
type Rule struct {
	ID                     string    `json:"id"`
	GuildID                string    `json:"guild_id"`
	Name                   string    `json:"name"`
	Enabled                bool      `json:"enabled"`
	RuleType               string    `json:"rule_type"`
	Config                 RuleConfig `json:"config"`
	Action                 string    `json:"action"`
	TimeoutDurationSeconds int       `json:"timeout_duration_seconds,omitempty"`
	ExemptChannelIDs       []string  `json:"exempt_channel_ids,omitempty"`
	ExemptRoleIDs          []string  `json:"exempt_role_ids,omitempty"`
	CreatedBy              string    `json:"created_by,omitempty"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

// RuleConfig is the JSON configuration blob for a rule. Fields are optional
// and depend on the rule type.
type RuleConfig struct {
	// word_filter
	Words          []string `json:"words,omitempty"`
	MatchWholeWord bool     `json:"match_whole_word,omitempty"`

	// regex_filter
	Patterns []string `json:"patterns,omitempty"`

	// invite_filter
	AllowOwnGuild bool `json:"allow_own_guild,omitempty"`

	// mention_spam
	MaxMentions int `json:"max_mentions,omitempty"`

	// caps_filter
	MaxCapsPercent int `json:"max_caps_percent,omitempty"`
	MinLength      int `json:"min_length,omitempty"`

	// spam_filter
	MaxMessages   int `json:"max_messages,omitempty"`
	WindowSeconds int `json:"window_seconds,omitempty"`
	MaxDuplicates int `json:"max_duplicates,omitempty"`

	// link_filter
	AllowedDomains []string `json:"allowed_domains,omitempty"`
	BlockedDomains []string `json:"blocked_domains,omitempty"`
}

// ActionRecord is an audit log entry for an automod action.
type ActionRecord struct {
	ID        string    `json:"id"`
	GuildID   string    `json:"guild_id"`
	RuleID    string    `json:"rule_id"`
	ChannelID string    `json:"channel_id"`
	MessageID string    `json:"message_id,omitempty"`
	UserID    string    `json:"user_id"`
	Action    string    `json:"action"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

// MessageContext holds the data needed to evaluate automod rules against a message.
type MessageContext struct {
	MessageID string
	ChannelID string
	GuildID   string
	AuthorID  string
	Content   string
	// MemberRoleIDs are the role IDs of the message author in the guild.
	MemberRoleIDs []string
}

// Service is the automod engine. It loads guild rules and evaluates messages.
type Service struct {
	pool   *pgxpool.Pool
	bus    *events.Bus
	logger *slog.Logger
	spam   *SpamTracker
}

// Config holds configuration for the automod service.
type Config struct {
	Pool   *pgxpool.Pool
	Bus    *events.Bus
	Logger *slog.Logger
}

// NewService creates a new automod service.
func NewService(cfg Config) *Service {
	return &Service{
		pool:   cfg.Pool,
		bus:    cfg.Bus,
		logger: cfg.Logger,
		spam:   NewSpamTracker(),
	}
}

// LoadGuildRules fetches all enabled automod rules for a guild.
func (s *Service) LoadGuildRules(ctx context.Context, guildID string) ([]Rule, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, guild_id, name, enabled, rule_type, config, action,
		        timeout_duration_seconds, exempt_channel_ids, exempt_role_ids,
		        created_by, created_at, updated_at
		 FROM automod_rules
		 WHERE guild_id = $1 AND enabled = true
		 ORDER BY created_at ASC`,
		guildID,
	)
	if err != nil {
		return nil, fmt.Errorf("loading automod rules for guild %s: %w", guildID, err)
	}
	defer rows.Close()

	var rules []Rule
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
			s.logger.Error("scanning automod rule", slog.String("error", err.Error()))
			continue
		}

		if err := json.Unmarshal(configJSON, &r.Config); err != nil {
			s.logger.Error("parsing automod rule config",
				slog.String("rule_id", r.ID),
				slog.String("error", err.Error()),
			)
			continue
		}

		r.ExemptChannelIDs = exemptChannels
		r.ExemptRoleIDs = exemptRoles
		if createdBy != nil {
			r.CreatedBy = *createdBy
		}
		rules = append(rules, r)
	}

	return rules, nil
}

// Evaluate checks a message against all enabled rules for its guild.
// Returns the first triggered rule and reason, or nil if no rules triggered.
func (s *Service) Evaluate(ctx context.Context, msg MessageContext) (*Rule, string, error) {
	if msg.GuildID == "" {
		return nil, "", nil // DMs have no automod.
	}

	rules, err := s.LoadGuildRules(ctx, msg.GuildID)
	if err != nil {
		return nil, "", err
	}

	for i := range rules {
		rule := &rules[i]

		// Check channel exemptions.
		if isExempt(msg.ChannelID, rule.ExemptChannelIDs) {
			continue
		}

		// Check role exemptions.
		if hasExemptRole(msg.MemberRoleIDs, rule.ExemptRoleIDs) {
			continue
		}

		triggered, reason := s.checkRule(rule, msg)
		if triggered {
			return rule, reason, nil
		}
	}

	return nil, "", nil
}

// CleanupSpam removes stale entries from the spam tracker.
func (s *Service) CleanupSpam(maxAge time.Duration) {
	s.spam.Cleanup(maxAge)
}

// ExecuteAction performs the configured action for a triggered rule.
func (s *Service) ExecuteAction(ctx context.Context, rule *Rule, msg MessageContext, reason string) error {
	// Log the action to the audit table.
	actionID := models.NewULID().String()
	_, err := s.pool.Exec(ctx,
		`INSERT INTO automod_actions (id, guild_id, rule_id, channel_id, message_id, user_id, action, reason, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())`,
		actionID, rule.GuildID, rule.ID, msg.ChannelID, msg.MessageID, msg.AuthorID, rule.Action, reason,
	)
	if err != nil {
		s.logger.Error("failed to log automod action",
			slog.String("error", err.Error()),
		)
	}

	switch rule.Action {
	case ActionDelete:
		return s.deleteMessage(ctx, msg)
	case ActionTimeout:
		duration := time.Duration(rule.TimeoutDurationSeconds) * time.Second
		if duration <= 0 {
			duration = 60 * time.Second
		}
		return s.timeoutUser(ctx, msg, duration)
	case ActionWarn:
		// Publish a warning event that can be picked up by the gateway.
		return s.publishAutomodEvent(ctx, rule, msg, reason)
	case ActionLog:
		// Just log — the audit entry above is the record.
		return s.publishAutomodEvent(ctx, rule, msg, reason)
	}

	return nil
}

// deleteMessage removes the offending message from the database and publishes
// a MESSAGE_DELETE event.
func (s *Service) deleteMessage(ctx context.Context, msg MessageContext) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM messages WHERE id = $1`, msg.MessageID)
	if err != nil {
		return fmt.Errorf("deleting message %s: %w", msg.MessageID, err)
	}

	s.bus.PublishChannelEvent(ctx, events.SubjectMessageDelete, "MESSAGE_DELETE", msg.ChannelID, map[string]string{
		"id":         msg.MessageID,
		"channel_id": msg.ChannelID,
		"guild_id":   msg.GuildID,
	})

	s.logger.Info("automod deleted message",
		slog.String("message_id", msg.MessageID),
		slog.String("guild_id", msg.GuildID),
	)
	return nil
}

// timeoutUser applies a timeout to the offending user.
func (s *Service) timeoutUser(ctx context.Context, msg MessageContext, duration time.Duration) error {
	until := time.Now().Add(duration)
	_, err := s.pool.Exec(ctx,
		`UPDATE guild_members SET timeout_until = $1 WHERE guild_id = $2 AND user_id = $3`,
		until, msg.GuildID, msg.AuthorID,
	)
	if err != nil {
		return fmt.Errorf("timing out user %s: %w", msg.AuthorID, err)
	}

	// Also delete the offending message.
	s.deleteMessage(ctx, msg)

	s.logger.Info("automod timed out user",
		slog.String("user_id", msg.AuthorID),
		slog.String("guild_id", msg.GuildID),
		slog.String("duration", duration.String()),
	)
	return nil
}

// publishAutomodEvent publishes an automod action event to the event bus.
func (s *Service) publishAutomodEvent(ctx context.Context, rule *Rule, msg MessageContext, reason string) error {
	return s.bus.PublishGuildEvent(ctx, events.SubjectAutomodAction, "AUTOMOD_ACTION", msg.GuildID, map[string]interface{}{
		"guild_id":   msg.GuildID,
		"channel_id": msg.ChannelID,
		"message_id": msg.MessageID,
		"user_id":    msg.AuthorID,
		"rule_id":    rule.ID,
		"rule_name":  rule.Name,
		"action":     rule.Action,
		"reason":     reason,
	})
}

// checkRule evaluates a single rule against a message.
func (s *Service) checkRule(rule *Rule, msg MessageContext) (bool, string) {
	switch rule.RuleType {
	case RuleWordFilter:
		return checkWordFilter(msg.Content, rule.Config)
	case RuleRegexFilter:
		return checkRegexFilter(msg.Content, rule.Config)
	case RuleInviteFilter:
		return checkInviteFilter(msg.Content, rule.Config, msg.GuildID)
	case RuleMentionSpam:
		return checkMentionSpam(msg.Content, rule.Config)
	case RuleCapsFilter:
		return checkCapsFilter(msg.Content, rule.Config)
	case RuleSpamFilter:
		return s.spam.Check(msg.AuthorID, msg.ChannelID, msg.Content, rule.Config)
	case RuleLinkFilter:
		return checkLinkFilter(msg.Content, rule.Config)
	default:
		return false, ""
	}
}

// isExempt checks if a channel is in the exempt list.
func isExempt(channelID string, exemptIDs []string) bool {
	for _, id := range exemptIDs {
		if id == channelID {
			return true
		}
	}
	return false
}

// hasExemptRole checks if any of the member's roles are in the exempt list.
func hasExemptRole(memberRoles, exemptRoles []string) bool {
	for _, mr := range memberRoles {
		for _, er := range exemptRoles {
			if mr == er {
				return true
			}
		}
	}
	return false
}
