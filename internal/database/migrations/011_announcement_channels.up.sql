-- Announcement channel followers
CREATE TABLE IF NOT EXISTS channel_followers (
    id TEXT PRIMARY KEY,
    channel_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    webhook_id TEXT NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    guild_id TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_channel_followers_channel ON channel_followers(channel_id);
CREATE INDEX IF NOT EXISTS idx_channel_followers_guild ON channel_followers(guild_id);
