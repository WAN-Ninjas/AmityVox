-- Migration 058: Guild folders for organizing the guild sidebar
CREATE TABLE guild_folders (
    id        TEXT PRIMARY KEY,
    user_id   TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name      TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 20),
    position  INTEGER NOT NULL DEFAULT 0,
    color     TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_guild_folders_user ON guild_folders(user_id, position);

-- Extend user_guild_positions with optional folder membership.
ALTER TABLE user_guild_positions ADD COLUMN folder_id TEXT REFERENCES guild_folders(id) ON DELETE SET NULL;
ALTER TABLE user_guild_positions ADD COLUMN folder_position INTEGER NOT NULL DEFAULT 0;
