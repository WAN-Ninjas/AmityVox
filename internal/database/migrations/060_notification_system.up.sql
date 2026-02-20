-- Migration 060: Notification system overhaul
-- Adds persistent notifications table and per-type delivery preferences.

CREATE TABLE notifications (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type            TEXT NOT NULL,
    category        TEXT NOT NULL,
    guild_id        TEXT,
    guild_name      TEXT,
    guild_icon_id   TEXT,
    channel_id      TEXT,
    channel_name    TEXT,
    message_id      TEXT,
    actor_id        TEXT NOT NULL,
    actor_name      TEXT NOT NULL,
    actor_avatar_id TEXT,
    content         TEXT,
    metadata        JSONB,
    read            BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_notifications_user_created ON notifications(user_id, created_at DESC);
CREATE INDEX idx_notifications_user_unread ON notifications(user_id) WHERE NOT read;
CREATE INDEX idx_notifications_user_type ON notifications(user_id, type);
CREATE INDEX idx_notifications_retention ON notifications(created_at);

CREATE TABLE notification_type_preferences (
    user_id  TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type     TEXT NOT NULL,
    in_app   BOOLEAN NOT NULL DEFAULT true,
    push     BOOLEAN NOT NULL DEFAULT true,
    sound    BOOLEAN NOT NULL DEFAULT true,
    PRIMARY KEY (user_id, type)
);
