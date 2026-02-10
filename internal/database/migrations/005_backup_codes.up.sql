-- Migration 005: Add backup codes table for 2FA recovery.

CREATE TABLE backup_codes (
    id        TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id   TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash TEXT NOT NULL,
    used      BOOLEAN NOT NULL DEFAULT false,
    used_at   TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_backup_codes_user_id ON backup_codes(user_id);
