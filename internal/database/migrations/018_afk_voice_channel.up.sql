-- AFK voice channel support for guilds.
ALTER TABLE guilds ADD COLUMN IF NOT EXISTS afk_channel_id TEXT REFERENCES channels(id) ON DELETE SET NULL;
ALTER TABLE guilds ADD COLUMN IF NOT EXISTS afk_timeout INTEGER NOT NULL DEFAULT 300;
