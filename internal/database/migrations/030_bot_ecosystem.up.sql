-- Phase 9: Bot Ecosystem — guild permissions, message components, presence,
-- event subscriptions, and rate limiting.

-- Bot guild permissions: per-guild scope and role-hierarchy limits for bots.
CREATE TABLE IF NOT EXISTS bot_guild_permissions (
    bot_id            TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    guild_id          TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    scopes            TEXT[] NOT NULL DEFAULT '{}',
    max_role_position INT NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (bot_id, guild_id)
);
CREATE INDEX IF NOT EXISTS idx_bot_guild_permissions_guild ON bot_guild_permissions(guild_id);

-- Message components: interactive elements (buttons, select menus) on messages.
CREATE TABLE IF NOT EXISTS message_components (
    id              TEXT PRIMARY KEY,
    message_id      TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    component_type  TEXT NOT NULL,
    style           TEXT,
    label           TEXT,
    custom_id       TEXT,
    url             TEXT,
    disabled        BOOLEAN NOT NULL DEFAULT false,
    options         JSONB,
    min_values      INT,
    max_values      INT,
    placeholder     TEXT,
    position        INT NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_message_components_message ON message_components(message_id);

-- Bot presence/status: bots can advertise their current activity.
CREATE TABLE IF NOT EXISTS bot_presence (
    bot_id          TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    status          TEXT NOT NULL DEFAULT 'online',
    activity_type   TEXT,
    activity_name   TEXT,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Bot event subscriptions: per-guild webhook-based event delivery.
CREATE TABLE IF NOT EXISTS bot_event_subscriptions (
    id              TEXT PRIMARY KEY,
    bot_id          TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    guild_id        TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    event_types     TEXT[] NOT NULL DEFAULT '{}',
    webhook_url     TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_bot_event_subs_bot ON bot_event_subscriptions(bot_id);
CREATE INDEX IF NOT EXISTS idx_bot_event_subs_guild ON bot_event_subscriptions(guild_id);

-- Bot rate limits: per-bot configurable request throttling.
CREATE TABLE IF NOT EXISTS bot_rate_limits (
    bot_id               TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    requests_per_second  INT NOT NULL DEFAULT 50,
    burst                INT NOT NULL DEFAULT 100,
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
