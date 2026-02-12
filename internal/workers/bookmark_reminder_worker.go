package workers

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/amityvox/amityvox/internal/notifications"
)

// startBookmarkReminderWorker launches a periodic worker that checks for
// bookmark reminders whose reminder_at has passed and sends push notifications
// to the owning user. It runs every 60 seconds and processes up to 50
// reminders per tick.
func (m *Manager) startBookmarkReminderWorker(ctx context.Context) {
	m.startPeriodic(ctx, "bookmark-reminders", 60*time.Second, m.processBookmarkReminders)
}

// processBookmarkReminders queries for bookmarks with due reminders, sends
// push notifications, and marks them as reminded.
func (m *Manager) processBookmarkReminders(ctx context.Context) error {
	rows, err := m.pool.Query(ctx,
		`SELECT mb.user_id, mb.message_id, m.channel_id, m.content
		 FROM message_bookmarks mb
		 JOIN messages m ON m.id = mb.message_id
		 WHERE mb.reminder_at <= now()
		   AND mb.reminded = false
		 LIMIT 50`,
	)
	if err != nil {
		return fmt.Errorf("querying bookmark reminders: %w", err)
	}
	defer rows.Close()

	type reminder struct {
		UserID    string
		MessageID string
		ChannelID string
		Content   *string
	}

	var reminders []reminder
	for rows.Next() {
		var r reminder
		if err := rows.Scan(&r.UserID, &r.MessageID, &r.ChannelID, &r.Content); err != nil {
			m.logger.Error("failed to scan bookmark reminder row",
				slog.String("error", err.Error()),
			)
			continue
		}
		reminders = append(reminders, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating bookmark reminder rows: %w", err)
	}

	if len(reminders) == 0 {
		return nil
	}

	m.logger.Info("processing bookmark reminders",
		slog.Int("count", len(reminders)),
	)

	sentCount := 0
	for _, r := range reminders {
		// Mark as reminded first to prevent double-sending on failure.
		_, err := m.pool.Exec(ctx,
			`UPDATE message_bookmarks SET reminded = true WHERE user_id = $1 AND message_id = $2`,
			r.UserID, r.MessageID,
		)
		if err != nil {
			m.logger.Error("failed to mark bookmark as reminded",
				slog.String("user_id", r.UserID),
				slog.String("message_id", r.MessageID),
				slog.String("error", err.Error()),
			)
			continue
		}

		// Build notification body from message content.
		body := "You have a bookmark reminder."
		if r.Content != nil && *r.Content != "" {
			body = *r.Content
			if len(body) > 200 {
				body = body[:200] + "..."
			}
		}

		payload := notifications.PushPayload{
			Type:      "bookmark_reminder",
			Title:     "Bookmark Reminder",
			Body:      body,
			ChannelID: r.ChannelID,
			MessageID: r.MessageID,
		}

		if err := m.notifications.SendToUser(ctx, r.UserID, payload); err != nil {
			m.logger.Debug("failed to send bookmark reminder notification",
				slog.String("user_id", r.UserID),
				slog.String("message_id", r.MessageID),
				slog.String("error", err.Error()),
			)
			continue
		}
		sentCount++
	}

	if sentCount > 0 {
		m.logger.Info("bookmark reminders sent",
			slog.Int("sent", sentCount),
			slog.Int("total", len(reminders)),
		)
	}

	return nil
}
