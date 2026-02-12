-- Migration 023: Event reminder tracking for push notifications.
-- Tracks which reminders (15-min, 1-hour) have been sent for each guild event
-- to prevent duplicate notifications.

CREATE TABLE IF NOT EXISTS event_reminder_log (
    event_id      TEXT NOT NULL REFERENCES guild_events(id) ON DELETE CASCADE,
    reminder_type TEXT NOT NULL, -- '15min' or '1hour'
    sent_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (event_id, reminder_type)
);

CREATE INDEX idx_event_reminder_log_sent ON event_reminder_log(sent_at);
