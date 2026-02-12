-- Phase 10: Guild templates for data portability.
-- Stores snapshots of a guild's structure (roles, channels, categories,
-- permissions, settings) that can be exported and applied to other guilds.

CREATE TABLE IF NOT EXISTS guild_templates (
    id              TEXT PRIMARY KEY,
    guild_id        TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    description     TEXT,
    template_data   JSONB NOT NULL DEFAULT '{}',
    creator_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_guild_templates_guild_id ON guild_templates(guild_id);
CREATE INDEX IF NOT EXISTS idx_guild_templates_creator_id ON guild_templates(creator_id);
