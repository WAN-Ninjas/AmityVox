-- Add NSFW flag and description to attachments.
ALTER TABLE attachments ADD COLUMN IF NOT EXISTS nsfw BOOLEAN DEFAULT false;
ALTER TABLE attachments ADD COLUMN IF NOT EXISTS description TEXT;

-- Media tags (per-guild).
CREATE TABLE IF NOT EXISTS media_tags (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    guild_id TEXT REFERENCES guilds(id) ON DELETE CASCADE,
    created_by TEXT REFERENCES users(id),
    created_at TIMESTAMPTZ DEFAULT now(),
    UNIQUE(name, guild_id)
);

-- Junction table for tagging attachments.
CREATE TABLE IF NOT EXISTS attachment_tags (
    attachment_id TEXT NOT NULL REFERENCES attachments(id) ON DELETE CASCADE,
    tag_id TEXT NOT NULL REFERENCES media_tags(id) ON DELETE CASCADE,
    PRIMARY KEY (attachment_id, tag_id)
);

-- Indexes for gallery queries.
CREATE INDEX IF NOT EXISTS idx_attachments_uploader_id ON attachments (uploader_id) WHERE uploader_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_attachments_message_id ON attachments (message_id) WHERE message_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_media_tags_guild_id ON media_tags (guild_id);
