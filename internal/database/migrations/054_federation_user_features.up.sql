-- Migration 054: Federation User Features
-- Adds tables for channel-level peer tracking, channel mirrors, guild caching,
-- dead letter queue, and LiveKit URL for federated voice.

-- ============================================================
-- FEDERATION CHANNEL PEERS (which instances need events for a channel)
-- ============================================================

CREATE TABLE IF NOT EXISTS federation_channel_peers (
    channel_id  TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    instance_id TEXT NOT NULL REFERENCES instances(id),
    PRIMARY KEY (channel_id, instance_id)
);

CREATE INDEX IF NOT EXISTS idx_federation_channel_peers_instance ON federation_channel_peers(instance_id);

-- ============================================================
-- FEDERATION CHANNEL MIRRORS (local ↔ remote channel mapping)
-- ============================================================

CREATE TABLE IF NOT EXISTS federation_channel_mirrors (
    local_channel_id   TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    remote_channel_id  TEXT NOT NULL,
    remote_instance_id TEXT NOT NULL REFERENCES instances(id),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (local_channel_id, remote_instance_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_federation_channel_mirrors_remote ON federation_channel_mirrors(remote_channel_id, remote_instance_id);

-- ============================================================
-- FEDERATION GUILD CACHE (per-user cache of remote guild metadata)
-- ============================================================

CREATE TABLE IF NOT EXISTS federation_guild_cache (
    guild_id       TEXT NOT NULL,
    user_id        TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    instance_id    TEXT NOT NULL REFERENCES instances(id),
    name           TEXT NOT NULL,
    icon_id        TEXT,
    description    TEXT,
    member_count   INTEGER DEFAULT 0,
    channels_json  JSONB DEFAULT '[]',
    roles_json     JSONB DEFAULT '[]',
    cached_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (guild_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_federation_guild_cache_user ON federation_guild_cache(user_id);
CREATE INDEX IF NOT EXISTS idx_federation_guild_cache_instance ON federation_guild_cache(instance_id);
CREATE INDEX IF NOT EXISTS idx_federation_guild_cache_guild ON federation_guild_cache(guild_id);

-- ============================================================
-- FEDERATION DEAD LETTERS (permanently failed deliveries)
-- ============================================================

CREATE TABLE IF NOT EXISTS federation_dead_letters (
    id            TEXT PRIMARY KEY,
    target_domain TEXT NOT NULL,
    payload       JSONB NOT NULL,
    error_message TEXT,
    attempts      INTEGER NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_federation_dead_letters_domain ON federation_dead_letters(target_domain);
CREATE INDEX IF NOT EXISTS idx_federation_dead_letters_created ON federation_dead_letters(created_at DESC);

-- ============================================================
-- LIVEKIT URL ON INSTANCES (for federated voice token exchange)
-- ============================================================

ALTER TABLE instances ADD COLUMN IF NOT EXISTS livekit_url TEXT;
