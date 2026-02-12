-- Migration 033: Theme Gallery, Channel Emoji, User Channel Groups, Image Compression
-- Adds shared themes with likes, channel-specific emoji, user-side channel groups,
-- and compressed attachment columns.

-- ============================================================
-- SHARED THEMES (Community Theme Gallery)
-- ============================================================

CREATE TABLE IF NOT EXISTS shared_themes (
    id              TEXT PRIMARY KEY,                -- ULID
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    variables       JSONB NOT NULL DEFAULT '{}',
    custom_css      TEXT NOT NULL DEFAULT '',
    preview_colors  JSONB NOT NULL DEFAULT '[]',
    share_code      TEXT NOT NULL UNIQUE,
    downloads       INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_shared_themes_user ON shared_themes(user_id);
CREATE INDEX idx_shared_themes_share_code ON shared_themes(share_code);
CREATE INDEX idx_shared_themes_downloads ON shared_themes(downloads DESC);
CREATE INDEX idx_shared_themes_created ON shared_themes(created_at DESC);

-- ============================================================
-- THEME LIKES
-- ============================================================

CREATE TABLE IF NOT EXISTS theme_likes (
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    theme_id    TEXT NOT NULL REFERENCES shared_themes(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, theme_id)
);

CREATE INDEX idx_theme_likes_theme ON theme_likes(theme_id);

-- ============================================================
-- CHANNEL-SPECIFIC EMOJI
-- ============================================================

CREATE TABLE IF NOT EXISTS channel_emoji (
    id          TEXT PRIMARY KEY,                    -- ULID
    channel_id  TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    guild_id    TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    creator_id  TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    animated    BOOLEAN NOT NULL DEFAULT false,
    s3_key      TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_channel_emoji_channel ON channel_emoji(channel_id);
CREATE INDEX idx_channel_emoji_guild ON channel_emoji(guild_id);
CREATE UNIQUE INDEX idx_channel_emoji_name_channel ON channel_emoji(channel_id, name);

-- ============================================================
-- USER CHANNEL GROUPS (client-side organization)
-- ============================================================

CREATE TABLE IF NOT EXISTS user_channel_groups (
    id          TEXT PRIMARY KEY,                    -- ULID
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    position    INTEGER NOT NULL DEFAULT 0,
    color       TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_user_channel_groups_user ON user_channel_groups(user_id, position);

CREATE TABLE IF NOT EXISTS user_channel_group_items (
    group_id    TEXT NOT NULL REFERENCES user_channel_groups(id) ON DELETE CASCADE,
    channel_id  TEXT NOT NULL,
    PRIMARY KEY (group_id, channel_id)
);

CREATE INDEX idx_user_channel_group_items_channel ON user_channel_group_items(channel_id);

-- ============================================================
-- IMAGE COMPRESSION COLUMNS ON ATTACHMENTS
-- ============================================================

ALTER TABLE attachments ADD COLUMN IF NOT EXISTS compressed_s3_key TEXT;
ALTER TABLE attachments ADD COLUMN IF NOT EXISTS compressed_size_bytes BIGINT;
