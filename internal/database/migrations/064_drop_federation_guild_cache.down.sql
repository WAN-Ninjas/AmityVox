-- Recreate federation_guild_cache for rollback.
CREATE TABLE IF NOT EXISTS federation_guild_cache (
    guild_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    instance_id TEXT NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    icon_id TEXT,
    description TEXT,
    member_count INTEGER DEFAULT 0,
    channels_json JSONB DEFAULT '[]',
    roles_json JSONB DEFAULT '[]',
    cached_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (guild_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_federation_guild_cache_user ON federation_guild_cache(user_id);
CREATE INDEX IF NOT EXISTS idx_federation_guild_cache_instance ON federation_guild_cache(instance_id);
CREATE INDEX IF NOT EXISTS idx_federation_guild_cache_guild ON federation_guild_cache(guild_id);
