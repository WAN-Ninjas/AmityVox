package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/models"
	"github.com/amityvox/amityvox/internal/notifications"
)

// startNotificationWorker subscribes to message events and sends push
// notifications to mentioned/DMed users.
func (m *Manager) startNotificationWorker(ctx context.Context) {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()

		_, err := m.bus.Subscribe(events.SubjectMessageCreate, func(event events.Event) {
			m.processNotification(ctx, event)
		})
		if err != nil {
			m.logger.Error("failed to subscribe for push notifications",
				slog.String("error", err.Error()))
			return
		}

		m.logger.Info("push notification worker started")
		<-ctx.Done()
	}()
}

// processNotification evaluates a message for push notification delivery.
func (m *Manager) processNotification(ctx context.Context, event events.Event) {
	var msg struct {
		ID              string   `json:"id"`
		ChannelID       string   `json:"channel_id"`
		GuildID         string   `json:"guild_id"`
		AuthorID        string   `json:"author_id"`
		Content         string   `json:"content"`
		Flags           int      `json:"flags"`
		MentionUserIDs  []string `json:"mention_user_ids"`
		MentionRoleIDs  []string `json:"mention_role_ids"`
		MentionEveryone bool     `json:"mention_everyone"`
	}

	if err := json.Unmarshal(event.Data, &msg); err != nil {
		return
	}

	if msg.Content == "" {
		return
	}

	// Skip notifications for silent messages.
	if msg.Flags&models.MessageFlagSilent != 0 {
		return
	}

	// Get author's display name for notification title.
	var authorName string
	err := m.pool.QueryRow(ctx,
		`SELECT COALESCE(display_name, username) FROM users WHERE id = $1`, msg.AuthorID,
	).Scan(&authorName)
	if err != nil {
		authorName = "Someone"
	}

	// Determine notification recipients.
	recipients := map[string]bool{}

	isDM := msg.GuildID == ""

	if isDM {
		// For DMs, notify all participants except the author.
		rows, err := m.pool.Query(ctx,
			`SELECT user_id FROM dm_participants WHERE channel_id = $1 AND user_id != $2`,
			msg.ChannelID, msg.AuthorID,
		)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var uid string
				if rows.Scan(&uid) == nil {
					recipients[uid] = true
				}
			}
		}
	}

	// Direct mentions.
	for _, uid := range msg.MentionUserIDs {
		if uid != msg.AuthorID {
			recipients[uid] = true
		}
	}

	// Role mentions — find users with those roles.
	for _, roleID := range msg.MentionRoleIDs {
		rows, err := m.pool.Query(ctx,
			`SELECT user_id FROM member_roles WHERE role_id = $1 AND guild_id = $2`,
			roleID, msg.GuildID,
		)
		if err == nil {
			for rows.Next() {
				var uid string
				if rows.Scan(&uid) == nil && uid != msg.AuthorID {
					recipients[uid] = true
				}
			}
			rows.Close()
		}
	}

	// @everyone — notify all guild members.
	if msg.MentionEveryone && msg.GuildID != "" {
		rows, err := m.pool.Query(ctx,
			`SELECT user_id FROM guild_members WHERE guild_id = $1 AND user_id != $2`,
			msg.GuildID, msg.AuthorID,
		)
		if err == nil {
			for rows.Next() {
				var uid string
				if rows.Scan(&uid) == nil {
					recipients[uid] = true
				}
			}
			rows.Close()
		}
	}

	if len(recipients) == 0 {
		return
	}

	// Build notification payload.
	body := msg.Content
	if len(body) > 200 {
		body = body[:200] + "..."
	}

	title := authorName
	if msg.GuildID != "" {
		var guildName, channelName string
		m.pool.QueryRow(ctx, `SELECT name FROM guilds WHERE id = $1`, msg.GuildID).Scan(&guildName)
		m.pool.QueryRow(ctx, `SELECT name FROM channels WHERE id = $1`, msg.ChannelID).Scan(&channelName)
		if guildName != "" && channelName != "" {
			title = fmt.Sprintf("%s in #%s (%s)", authorName, channelName, guildName)
		}
	}

	payload := notifications.PushPayload{
		Type:      "message",
		Title:     title,
		Body:      body,
		ChannelID: msg.ChannelID,
		GuildID:   msg.GuildID,
		MessageID: msg.ID,
	}

	// Send to each recipient, respecting notification preferences.
	for uid := range recipients {
		isMention := false
		for _, mid := range msg.MentionUserIDs {
			if mid == uid {
				isMention = true
				break
			}
		}

		if !m.notifications.ShouldNotify(ctx, uid, msg.GuildID, msg.ChannelID, isMention, isDM, msg.MentionEveryone) {
			continue
		}

		if err := m.notifications.SendToUser(ctx, uid, payload); err != nil {
			m.logger.Debug("failed to send push notification",
				slog.String("user_id", uid),
				slog.String("error", err.Error()),
			)
		}
	}
}
