-- Migration 002: Add user notes, user settings, and guild vanity URLs.

-- ============================================================
-- USER NOTES (personal notes about other users)
-- ============================================================

CREATE TABLE user_notes (
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    target_id   TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    note        TEXT NOT NULL,
    updated_at  TIMESTAMPTZ DEFAULT now(),
    PRIMARY KEY (user_id, target_id)
);

-- ============================================================
-- USER SETTINGS (client-side settings stored as JSON)
-- ============================================================

CREATE TABLE user_settings (
    user_id     TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    settings    JSONB NOT NULL DEFAULT '{}',
    updated_at  TIMESTAMPTZ DEFAULT now()
);

-- ============================================================
-- GUILD VANITY URLS
-- ============================================================

ALTER TABLE guilds ADD COLUMN vanity_url TEXT UNIQUE;

CREATE INDEX idx_guilds_vanity ON guilds(vanity_url) WHERE vanity_url IS NOT NULL;
