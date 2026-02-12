-- Shared ban lists for cross-server ban sharing
CREATE TABLE IF NOT EXISTS ban_lists (
    id TEXT PRIMARY KEY,
    guild_id TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT,
    public BOOLEAN NOT NULL DEFAULT false,
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_ban_lists_guild ON ban_lists(guild_id);
CREATE INDEX IF NOT EXISTS idx_ban_lists_public ON ban_lists(public) WHERE public = true;

-- Entries in a ban list
CREATE TABLE IF NOT EXISTS ban_list_entries (
    id TEXT PRIMARY KEY,
    ban_list_id TEXT NOT NULL REFERENCES ban_lists(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL,
    username TEXT,
    reason TEXT,
    added_by TEXT NOT NULL REFERENCES users(id),
    added_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_ban_list_entries_list ON ban_list_entries(ban_list_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_ban_list_entries_unique ON ban_list_entries(ban_list_id, user_id);

-- Subscriptions to ban lists (other guilds subscribing to a ban list)
CREATE TABLE IF NOT EXISTS ban_list_subscriptions (
    id TEXT PRIMARY KEY,
    guild_id TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    ban_list_id TEXT NOT NULL REFERENCES ban_lists(id) ON DELETE CASCADE,
    auto_ban BOOLEAN NOT NULL DEFAULT true,
    subscribed_by TEXT NOT NULL REFERENCES users(id),
    subscribed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_ban_list_sub_unique ON ban_list_subscriptions(guild_id, ban_list_id);

-- Voice messages metadata
ALTER TABLE messages ADD COLUMN IF NOT EXISTS voice_duration_ms INTEGER;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS voice_waveform JSONB;
