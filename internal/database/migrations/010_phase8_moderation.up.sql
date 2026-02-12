-- Phase 8: Moderation & Safety Enhancements
-- Adds verification levels, warnings, raid protection, channel locks, and reports.

-- ============================================================
-- GUILD VERIFICATION LEVELS
-- ============================================================

ALTER TABLE guilds ADD COLUMN IF NOT EXISTS verification_level INTEGER DEFAULT 0;
-- 0=None, 1=Low (verified email), 2=Medium (registered 5+ min),
-- 3=High (member 10+ min), 4=Highest (admin bypass only)

-- ============================================================
-- MEMBER WARNINGS
-- ============================================================

CREATE TABLE member_warnings (
    id              TEXT PRIMARY KEY,            -- ULID
    guild_id        TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    moderator_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reason          TEXT NOT NULL,
    created_at      TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_member_warnings_guild_user ON member_warnings(guild_id, user_id, created_at DESC);
CREATE INDEX idx_member_warnings_guild ON member_warnings(guild_id, created_at DESC);

-- ============================================================
-- MESSAGE REPORTS
-- ============================================================

CREATE TABLE message_reports (
    id              TEXT PRIMARY KEY,            -- ULID
    guild_id        TEXT REFERENCES guilds(id) ON DELETE CASCADE,
    channel_id      TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    message_id      TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    reporter_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reason          TEXT NOT NULL,
    status          TEXT DEFAULT 'open'
        CHECK (status IN ('open', 'resolved', 'dismissed', 'admin_pending')),
    resolved_by     TEXT REFERENCES users(id) ON DELETE SET NULL,
    resolved_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_reports_guild ON message_reports(guild_id, status, created_at DESC);
CREATE INDEX idx_reports_reporter ON message_reports(reporter_id, created_at DESC);

-- ============================================================
-- CHANNEL LOCKS
-- ============================================================

ALTER TABLE channels ADD COLUMN IF NOT EXISTS locked BOOLEAN DEFAULT false;
ALTER TABLE channels ADD COLUMN IF NOT EXISTS locked_by TEXT REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE channels ADD COLUMN IF NOT EXISTS locked_at TIMESTAMPTZ;

-- ============================================================
-- RAID PROTECTION CONFIGURATION
-- ============================================================

CREATE TABLE guild_raid_config (
    guild_id            TEXT PRIMARY KEY REFERENCES guilds(id) ON DELETE CASCADE,
    enabled             BOOLEAN DEFAULT false,
    join_rate_limit     INTEGER DEFAULT 10,       -- Max joins per window
    join_rate_window    INTEGER DEFAULT 10,        -- Window in seconds
    min_account_age     INTEGER DEFAULT 0,         -- Minimum account age in seconds
    lockdown_active     BOOLEAN DEFAULT false,
    lockdown_started_at TIMESTAMPTZ,
    updated_at          TIMESTAMPTZ DEFAULT now()
);
