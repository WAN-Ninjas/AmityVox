-- User reports (report a user to instance moderators)
CREATE TABLE user_reports (
    id TEXT PRIMARY KEY,
    reporter_id TEXT NOT NULL REFERENCES users(id),
    reported_user_id TEXT NOT NULL REFERENCES users(id),
    reason TEXT NOT NULL,
    context_guild_id TEXT REFERENCES guilds(id),
    context_channel_id TEXT REFERENCES channels(id),
    status TEXT NOT NULL DEFAULT 'open'
        CHECK (status IN ('open', 'resolved', 'dismissed')),
    resolved_by TEXT REFERENCES users(id),
    resolved_at TIMESTAMPTZ,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_user_reports_status ON user_reports(status);
CREATE INDEX idx_user_reports_reported ON user_reports(reported_user_id);
CREATE INDEX idx_user_reports_status_created ON user_reports(status, created_at DESC);

-- Reported issues (general bugs/concerns from any user)
CREATE TABLE reported_issues (
    id TEXT PRIMARY KEY,
    reporter_id TEXT NOT NULL REFERENCES users(id),
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    category TEXT NOT NULL DEFAULT 'general'
        CHECK (category IN ('general', 'bug', 'abuse', 'suggestion')),
    status TEXT NOT NULL DEFAULT 'open'
        CHECK (status IN ('open', 'in_progress', 'resolved', 'dismissed')),
    resolved_by TEXT REFERENCES users(id),
    resolved_at TIMESTAMPTZ,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_reported_issues_status ON reported_issues(status);
CREATE INDEX idx_reported_issues_status_created ON reported_issues(status, created_at DESC);

-- Fix message_reports CHECK constraint to include 'admin_pending' status
ALTER TABLE message_reports DROP CONSTRAINT IF EXISTS message_reports_status_check;
ALTER TABLE message_reports ADD CONSTRAINT message_reports_status_check
    CHECK (status IN ('open', 'resolved', 'dismissed', 'admin_pending'));
