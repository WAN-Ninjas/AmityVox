-- Migration 031: Admin Moderation - Rate Limiting, Content Scanning, CAPTCHA
-- Adds rate limit logging, content scan rules/logs, and CAPTCHA configuration.

-- ============================================================
-- RATE LIMIT LOG
-- ============================================================

CREATE TABLE IF NOT EXISTS rate_limit_log (
    id              TEXT PRIMARY KEY,                -- ULID
    ip_address      TEXT NOT NULL,
    endpoint        TEXT NOT NULL,
    requests_count  INTEGER NOT NULL DEFAULT 0,
    window_start    TIMESTAMPTZ NOT NULL,
    blocked         BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_rate_limit_log_ip ON rate_limit_log(ip_address, created_at DESC);
CREATE INDEX idx_rate_limit_log_blocked ON rate_limit_log(blocked, created_at DESC);
CREATE INDEX idx_rate_limit_log_created ON rate_limit_log(created_at DESC);

-- ============================================================
-- CONTENT SCAN RULES
-- ============================================================

CREATE TABLE IF NOT EXISTS content_scan_rules (
    id          TEXT PRIMARY KEY,                    -- ULID
    name        TEXT NOT NULL,
    pattern     TEXT NOT NULL,
    action      TEXT NOT NULL DEFAULT 'log'
        CHECK (action IN ('block', 'flag', 'log')),
    target      TEXT NOT NULL DEFAULT 'filename'
        CHECK (target IN ('filename', 'content_type', 'text_content')),
    enabled     BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================================================
-- CONTENT SCAN LOG
-- ============================================================

CREATE TABLE IF NOT EXISTS content_scan_log (
    id              TEXT PRIMARY KEY,                -- ULID
    rule_id         TEXT NOT NULL REFERENCES content_scan_rules(id) ON DELETE CASCADE,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel_id      TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    content_matched TEXT NOT NULL,
    action_taken    TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_content_scan_log_rule ON content_scan_log(rule_id, created_at DESC);
CREATE INDEX idx_content_scan_log_user ON content_scan_log(user_id, created_at DESC);
CREATE INDEX idx_content_scan_log_created ON content_scan_log(created_at DESC);

-- ============================================================
-- CAPTCHA SETTINGS (via instance_settings)
-- ============================================================

INSERT INTO instance_settings (key, value, updated_at)
VALUES ('captcha_provider', 'none', now())
ON CONFLICT (key) DO NOTHING;

INSERT INTO instance_settings (key, value, updated_at)
VALUES ('captcha_site_key', '', now())
ON CONFLICT (key) DO NOTHING;

INSERT INTO instance_settings (key, value, updated_at)
VALUES ('captcha_secret_key', '', now())
ON CONFLICT (key) DO NOTHING;

-- ============================================================
-- RATE LIMIT SETTINGS (via instance_settings)
-- ============================================================

INSERT INTO instance_settings (key, value, updated_at)
VALUES ('rate_limit_requests_per_window', '100', now())
ON CONFLICT (key) DO NOTHING;

INSERT INTO instance_settings (key, value, updated_at)
VALUES ('rate_limit_window_seconds', '60', now())
ON CONFLICT (key) DO NOTHING;
