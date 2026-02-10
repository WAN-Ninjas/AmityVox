package workers

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/amityvox/amityvox/internal/automod"
	"github.com/amityvox/amityvox/internal/events"
)

// startAutomodWorker subscribes to MESSAGE_CREATE events and evaluates them
// against the guild's automod rules. If a rule triggers, the configured action
// is executed (delete, warn, timeout, or log).
func (m *Manager) startAutomodWorker(ctx context.Context) {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()

		_, err := m.bus.Subscribe(events.SubjectMessageCreate, func(event events.Event) {
			m.processAutomod(ctx, event)
		})
		if err != nil {
			m.logger.Error("failed to subscribe for automod",
				slog.String("error", err.Error()))
			return
		}

		m.logger.Info("automod worker started")

		// Periodic cleanup of the spam tracker every 10 minutes.
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.automod.CleanupSpam(1 * time.Hour)
			}
		}
	}()
}

// processAutomod evaluates a message against automod rules.
func (m *Manager) processAutomod(ctx context.Context, event events.Event) {
	var msgData struct {
		ID        string `json:"id"`
		ChannelID string `json:"channel_id"`
		GuildID   string `json:"guild_id"`
		AuthorID  string `json:"author_id"`
		Content   string `json:"content"`
	}

	if err := json.Unmarshal(event.Data, &msgData); err != nil {
		return
	}

	// Skip DMs (no guild_id) and empty content.
	if msgData.GuildID == "" || msgData.Content == "" {
		return
	}

	// Look up author's roles in the guild for exemption checks.
	var roleIDs []string
	rows, err := m.pool.Query(ctx,
		`SELECT role_id FROM member_roles WHERE guild_id = $1 AND user_id = $2`,
		msgData.GuildID, msgData.AuthorID,
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var roleID string
			if rows.Scan(&roleID) == nil {
				roleIDs = append(roleIDs, roleID)
			}
		}
	}

	msgCtx := automod.MessageContext{
		MessageID:     msgData.ID,
		ChannelID:     msgData.ChannelID,
		GuildID:       msgData.GuildID,
		AuthorID:      msgData.AuthorID,
		Content:       msgData.Content,
		MemberRoleIDs: roleIDs,
	}

	rule, reason, err := m.automod.Evaluate(ctx, msgCtx)
	if err != nil {
		m.logger.Error("automod evaluation failed",
			slog.String("message_id", msgData.ID),
			slog.String("error", err.Error()),
		)
		return
	}

	if rule == nil {
		return // No rules triggered.
	}

	m.logger.Info("automod rule triggered",
		slog.String("rule_id", rule.ID),
		slog.String("rule_name", rule.Name),
		slog.String("action", rule.Action),
		slog.String("reason", reason),
		slog.String("message_id", msgData.ID),
		slog.String("user_id", msgData.AuthorID),
	)

	if err := m.automod.ExecuteAction(ctx, rule, msgCtx, reason); err != nil {
		m.logger.Error("automod action failed",
			slog.String("rule_id", rule.ID),
			slog.String("error", err.Error()),
		)
	}
}
