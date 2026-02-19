-- Migration 057: Per-user guild ordering
CREATE TABLE user_guild_positions (
    user_id  TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    guild_id TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    position INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, guild_id)
);
CREATE INDEX idx_user_guild_positions_user ON user_guild_positions(user_id, position);
