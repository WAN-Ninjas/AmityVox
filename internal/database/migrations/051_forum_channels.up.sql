-- Forum channels: tags, post-tag associations, and forum-specific channel fields.

-- Forum tags (per-forum-channel).
CREATE TABLE IF NOT EXISTS forum_tags (
    id TEXT PRIMARY KEY,
    channel_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    emoji TEXT,
    color TEXT,
    position INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_forum_tags_channel
    ON forum_tags (channel_id, position);

-- Forum post-to-tag join table (posts are thread channels with parent = forum).
CREATE TABLE IF NOT EXISTS forum_post_tags (
    post_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    tag_id TEXT NOT NULL REFERENCES forum_tags(id) ON DELETE CASCADE,
    PRIMARY KEY (post_id, tag_id)
);

-- Forum-specific channel fields.
ALTER TABLE channels ADD COLUMN IF NOT EXISTS forum_default_sort TEXT DEFAULT 'latest_activity';
ALTER TABLE channels ADD COLUMN IF NOT EXISTS forum_post_guidelines TEXT;
ALTER TABLE channels ADD COLUMN IF NOT EXISTS forum_require_tags BOOLEAN DEFAULT false;

-- Post metadata fields on channels (for forum threads/posts).
ALTER TABLE channels ADD COLUMN IF NOT EXISTS pinned BOOLEAN DEFAULT false;
ALTER TABLE channels ADD COLUMN IF NOT EXISTS reply_count INTEGER DEFAULT 0;
