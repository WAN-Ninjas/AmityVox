-- Registration tokens for invite-only mode.
CREATE TABLE IF NOT EXISTS registration_tokens (
    id TEXT PRIMARY KEY,
    created_by TEXT NOT NULL REFERENCES users(id),
    max_uses INTEGER NOT NULL DEFAULT 1,
    uses INTEGER NOT NULL DEFAULT 0,
    note TEXT,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Instance announcements visible to all users.
CREATE TABLE IF NOT EXISTS instance_announcements (
    id TEXT PRIMARY KEY,
    admin_id TEXT NOT NULL REFERENCES users(id),
    title TEXT NOT NULL,
    content TEXT NOT NULL,
    severity TEXT NOT NULL DEFAULT 'info',
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ
);
