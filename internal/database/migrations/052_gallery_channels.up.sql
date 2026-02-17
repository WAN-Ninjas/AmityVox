-- Gallery channel tags (per gallery channel, same pattern as forum_tags).
CREATE TABLE IF NOT EXISTS gallery_tags (
    id TEXT PRIMARY KEY,
    channel_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    emoji TEXT,
    color TEXT,
    position INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_gallery_tags_channel ON gallery_tags (channel_id, position);

-- Gallery post tags (posts are thread channels with parent = gallery).
CREATE TABLE IF NOT EXISTS gallery_post_tags (
    post_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    tag_id TEXT NOT NULL REFERENCES gallery_tags(id) ON DELETE CASCADE,
    PRIMARY KEY (post_id, tag_id)
);

-- Gallery-specific channel settings.
ALTER TABLE channels ADD COLUMN IF NOT EXISTS gallery_default_sort TEXT DEFAULT 'newest';
ALTER TABLE channels ADD COLUMN IF NOT EXISTS gallery_post_guidelines TEXT;
ALTER TABLE channels ADD COLUMN IF NOT EXISTS gallery_require_tags BOOLEAN DEFAULT false;

-- Add 'gallery' to the channel_type check constraint.
ALTER TABLE channels DROP CONSTRAINT IF EXISTS channels_channel_type_check;
ALTER TABLE channels ADD CONSTRAINT channels_channel_type_check
    CHECK (channel_type IN ('text', 'voice', 'dm', 'group', 'announcement', 'forum', 'stage', 'gallery'));
