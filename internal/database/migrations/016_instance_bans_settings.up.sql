-- Instance-level bans (admin action, separate from per-guild bans).
CREATE TABLE IF NOT EXISTS instance_bans (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    admin_id TEXT NOT NULL REFERENCES users(id),
    reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Instance settings key-value store for admin-configurable settings.
CREATE TABLE IF NOT EXISTS instance_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
