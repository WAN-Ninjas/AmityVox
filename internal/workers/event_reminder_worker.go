package workers

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/amityvox/amityvox/internal/notifications"
)

// startEventReminderWorker launches a periodic worker that checks for upcoming
// guild events and sends push notifications to RSVP'd users. It checks every
// 5 minutes for events starting within 15 minutes or 1 hour, and sends
// appropriate reminders. Sent reminders are tracked in the event_reminder_log
// table to prevent duplicates.
func (m *Manager) startEventReminderWorker(ctx context.Context) {
	m.startPeriodic(ctx, "event-reminders", 5*time.Minute, m.processEventReminders)
}

// processEventReminders queries for upcoming guild events and sends push
// notifications to RSVP'd users for both 15-minute and 1-hour reminders.
func (m *Manager) processEventReminders(ctx context.Context) error {
	if err := m.sendReminders(ctx, "15min", 15*time.Minute); err != nil {
		m.logger.Error("failed to process 15-min event reminders",
			slog.String("error", err.Error()),
		)
	}

	if err := m.sendReminders(ctx, "1hour", 1*time.Hour); err != nil {
		m.logger.Error("failed to process 1-hour event reminders",
			slog.String("error", err.Error()),
		)
	}

	return nil
}

// sendReminders finds events starting within the given window that haven't had
// the specified reminder type sent yet, and sends push notifications to all
// RSVP'd users.
func (m *Manager) sendReminders(ctx context.Context, reminderType string, window time.Duration) error {
	now := time.Now()
	cutoff := now.Add(window)

	// Find events starting within the window that:
	//  - Have status 'scheduled' (not cancelled/completed)
	//  - Haven't had this reminder type sent yet
	//  - Have at least one RSVP
	rows, err := m.pool.Query(ctx,
		`SELECT e.id, e.guild_id, e.name, e.scheduled_start
		 FROM guild_events e
		 WHERE e.status = 'scheduled'
		   AND e.scheduled_start > $1
		   AND e.scheduled_start <= $2
		   AND NOT EXISTS (
		       SELECT 1 FROM event_reminder_log r
		       WHERE r.event_id = e.id AND r.reminder_type = $3
		   )
		   AND EXISTS (
		       SELECT 1 FROM event_rsvps rv WHERE rv.event_id = e.id
		   )
		 ORDER BY e.scheduled_start ASC
		 LIMIT 50`,
		now, cutoff, reminderType,
	)
	if err != nil {
		return fmt.Errorf("querying upcoming events for %s reminders: %w", reminderType, err)
	}
	defer rows.Close()

	type upcomingEvent struct {
		ID             string
		GuildID        string
		Name           string
		ScheduledStart time.Time
	}

	var events []upcomingEvent
	for rows.Next() {
		var evt upcomingEvent
		if err := rows.Scan(&evt.ID, &evt.GuildID, &evt.Name, &evt.ScheduledStart); err != nil {
			m.logger.Error("failed to scan event row for reminders",
				slog.String("error", err.Error()),
			)
			continue
		}
		events = append(events, evt)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating event rows: %w", err)
	}

	if len(events) == 0 {
		return nil
	}

	m.logger.Info("processing event reminders",
		slog.String("type", reminderType),
		slog.Int("event_count", len(events)),
	)

	for _, evt := range events {
		if err := m.sendEventReminder(ctx, evt.ID, evt.GuildID, evt.Name, evt.ScheduledStart, reminderType); err != nil {
			m.logger.Error("failed to send event reminder",
				slog.String("event_id", evt.ID),
				slog.String("reminder_type", reminderType),
				slog.String("error", err.Error()),
			)
			continue
		}
	}

	return nil
}

// sendEventReminder sends push notifications to all RSVP'd users for a specific
// event and reminder type, then records it in the event_reminder_log.
func (m *Manager) sendEventReminder(ctx context.Context, eventID, guildID, eventName string, scheduledStart time.Time, reminderType string) error {
	// Get the guild name for the notification title.
	var guildName string
	err := m.pool.QueryRow(ctx,
		`SELECT COALESCE(name, 'Unknown Guild') FROM guilds WHERE id = $1`, guildID,
	).Scan(&guildName)
	if err != nil {
		guildName = "Unknown Guild"
	}

	// Get all RSVP'd users for this event.
	rsvpRows, err := m.pool.Query(ctx,
		`SELECT user_id FROM event_rsvps WHERE event_id = $1`,
		eventID,
	)
	if err != nil {
		return fmt.Errorf("querying RSVPs for event %s: %w", eventID, err)
	}
	defer rsvpRows.Close()

	var userIDs []string
	for rsvpRows.Next() {
		var uid string
		if err := rsvpRows.Scan(&uid); err != nil {
			continue
		}
		userIDs = append(userIDs, uid)
	}
	if err := rsvpRows.Err(); err != nil {
		return fmt.Errorf("iterating RSVP rows: %w", err)
	}

	if len(userIDs) == 0 {
		return nil
	}

	// Build the notification message.
	var timeLabel string
	switch reminderType {
	case "15min":
		timeLabel = "in 15 minutes"
	case "1hour":
		timeLabel = "in about 1 hour"
	default:
		timeLabel = "soon"
	}

	title := fmt.Sprintf("Event Reminder - %s", guildName)
	body := fmt.Sprintf("\"%s\" starts %s (%s)",
		eventName,
		timeLabel,
		scheduledStart.Local().Format("3:04 PM"),
	)

	payload := notifications.PushPayload{
		Type:    "event_reminder",
		Title:   title,
		Body:    body,
		GuildID: guildID,
	}

	// Send to each RSVP'd user.
	sentCount := 0
	for _, uid := range userIDs {
		if err := m.notifications.SendToUser(ctx, uid, payload); err != nil {
			m.logger.Debug("failed to send event reminder notification",
				slog.String("user_id", uid),
				slog.String("event_id", eventID),
				slog.String("error", err.Error()),
			)
			continue
		}
		sentCount++
	}

	// Record that we sent this reminder so we don't send it again.
	_, err = m.pool.Exec(ctx,
		`INSERT INTO event_reminder_log (event_id, reminder_type, sent_at)
		 VALUES ($1, $2, now())
		 ON CONFLICT (event_id, reminder_type) DO NOTHING`,
		eventID, reminderType,
	)
	if err != nil {
		return fmt.Errorf("recording reminder sent for event %s: %w", eventID, err)
	}

	m.logger.Info("event reminder sent",
		slog.String("event_id", eventID),
		slog.String("event_name", eventName),
		slog.String("reminder_type", reminderType),
		slog.Int("recipients", sentCount),
		slog.Int("total_rsvps", len(userIDs)),
	)

	return nil
}
