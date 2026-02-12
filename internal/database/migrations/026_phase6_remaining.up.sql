-- Phase 6 Remaining: Server Guide, Bump System, Auto-Cancel Events,
-- Activity Status, User Custom Emoji

-- ============================================================
-- SERVER GUIDE (curated walkthrough for new members)
-- ============================================================

CREATE TABLE guild_guides (
    id              TEXT PRIMARY KEY,            -- ULID
    guild_id        TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    title           TEXT NOT NULL,
    content         TEXT NOT NULL,               -- Markdown content
    position        INTEGER NOT NULL DEFAULT 0,
    channel_id      TEXT REFERENCES channels(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_guild_guides_guild ON guild_guides(guild_id, position);

-- ============================================================
-- BUMP SYSTEM (promote guilds in discovery)
-- ============================================================

CREATE TABLE guild_bumps (
    id              TEXT PRIMARY KEY,            -- ULID
    guild_id        TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    bumped_by       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    bumped_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_guild_bumps_guild ON guild_bumps(guild_id, bumped_at DESC);
CREATE INDEX idx_guild_bumps_user ON guild_bumps(bumped_by, bumped_at DESC);

-- ============================================================
-- AUTO-CANCEL EVENTS (grace window for overdue events)
-- ============================================================

ALTER TABLE guild_events ADD COLUMN IF NOT EXISTS auto_cancel_minutes INTEGER NOT NULL DEFAULT 30;

-- ============================================================
-- ACTIVITY STATUS (for bots and users)
-- ============================================================

ALTER TABLE users ADD COLUMN IF NOT EXISTS activity_type TEXT
    CHECK (activity_type IS NULL OR activity_type IN ('playing', 'listening', 'watching', 'streaming'));
ALTER TABLE users ADD COLUMN IF NOT EXISTS activity_name TEXT;

-- ============================================================
-- USER CUSTOM EMOJI (up to 10 personal emoji per user)
-- ============================================================

CREATE TABLE user_emoji (
    id              TEXT PRIMARY KEY,            -- ULID
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    file_id         TEXT NOT NULL,               -- S3 file reference
    animated        BOOLEAN DEFAULT false,
    created_at      TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_user_emoji_user ON user_emoji(user_id);
