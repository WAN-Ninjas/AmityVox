-- Migration 008: WebPush push notification subscriptions.
-- Users register browser/device push subscriptions. The server sends notifications
-- for mentions, DMs, and other configured events via the Web Push protocol.

CREATE TABLE push_subscriptions (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    endpoint    TEXT NOT NULL,                -- WebPush endpoint URL
    key_p256dh  TEXT NOT NULL,                -- Client public key (base64url)
    key_auth    TEXT NOT NULL,                -- Auth secret (base64url)
    user_agent  TEXT,                         -- Device/browser info
    created_at  TIMESTAMPTZ DEFAULT now(),
    last_used   TIMESTAMPTZ DEFAULT now(),
    UNIQUE (user_id, endpoint)
);

CREATE INDEX idx_push_subs_user ON push_subscriptions(user_id);

-- User notification preferences per guild (or global when guild_id is '__global__').
CREATE TABLE notification_preferences (
    user_id             TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    guild_id            TEXT NOT NULL DEFAULT '__global__',
    -- Level: all, mentions, none
    level               TEXT NOT NULL DEFAULT 'mentions' CHECK (level IN ('all','mentions','none')),
    suppress_everyone   BOOLEAN DEFAULT false,
    suppress_roles      BOOLEAN DEFAULT false,
    muted_until         TIMESTAMPTZ,
    PRIMARY KEY (user_id, guild_id)
);

CREATE INDEX idx_notif_prefs_user ON notification_preferences(user_id);
