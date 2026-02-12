-- Migration 027: Channel templates, read-only channels, and thread auto-archive duration.

-- Channel templates allow guilds to save channel configurations as reusable blueprints.
CREATE TABLE IF NOT EXISTS channel_templates (
    id                    TEXT PRIMARY KEY,
    guild_id              TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    name                  TEXT NOT NULL,
    channel_type          TEXT NOT NULL DEFAULT 'text',
    topic                 TEXT,
    slowmode_seconds      INTEGER NOT NULL DEFAULT 0,
    nsfw                  BOOLEAN NOT NULL DEFAULT FALSE,
    permission_overwrites JSONB,
    created_by            TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_channel_templates_guild_id ON channel_templates(guild_id);

-- Read-only channel support: when read_only is true, only users with roles listed
-- in read_only_role_ids (or guild owner/admin) may send messages.
ALTER TABLE channels ADD COLUMN IF NOT EXISTS read_only BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE channels ADD COLUMN IF NOT EXISTS read_only_role_ids TEXT[] NOT NULL DEFAULT '{}';

-- Thread auto-archive duration: the default auto-archive duration (in minutes) for
-- threads created in this channel. 0 means never auto-archive.
-- Valid values: 0 (never), 60 (1h), 1440 (1d), 4320 (3d), 10080 (7d).
ALTER TABLE channels ADD COLUMN IF NOT EXISTS default_auto_archive_duration INTEGER NOT NULL DEFAULT 0;
