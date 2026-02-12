-- Bot tokens for API authentication.
CREATE TABLE IF NOT EXISTS bot_tokens (
    id TEXT PRIMARY KEY,
    bot_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT 'default',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_bot_tokens_bot_id ON bot_tokens(bot_id);
CREATE INDEX IF NOT EXISTS idx_bot_tokens_hash ON bot_tokens(token_hash);

-- Slash commands registered by bots.
CREATE TABLE IF NOT EXISTS slash_commands (
    id TEXT PRIMARY KEY,
    bot_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    guild_id TEXT REFERENCES guilds(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    options JSONB NOT NULL DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(bot_id, guild_id, name)
);
CREATE INDEX IF NOT EXISTS idx_slash_commands_bot_id ON slash_commands(bot_id);
CREATE INDEX IF NOT EXISTS idx_slash_commands_guild_id ON slash_commands(guild_id);
