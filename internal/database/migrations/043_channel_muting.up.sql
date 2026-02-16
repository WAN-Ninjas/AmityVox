-- Migration 043: Channel-level notification preferences (muting).
-- Supports per-channel and per-DM muting with optional timed expiry.

CREATE TABLE IF NOT EXISTS channel_notification_preferences (
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    level      TEXT NOT NULL DEFAULT 'mentions' CHECK (level IN ('all','mentions','none')),
    muted_until TIMESTAMPTZ,
    PRIMARY KEY (user_id, channel_id)
);

CREATE INDEX idx_channel_notif_prefs_user ON channel_notification_preferences(user_id);
