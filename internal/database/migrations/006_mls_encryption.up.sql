-- Migration 006: MLS encryption delivery service tables.
-- The server acts as an MLS Delivery Service (RFC 9420), storing key packages,
-- welcome messages, group state, and commit messages. The server never sees
-- plaintext or private key material.

CREATE TABLE mls_key_packages (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id   TEXT NOT NULL,
    data        BYTEA NOT NULL,         -- Opaque MLS KeyPackage bytes
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_mls_key_packages_user ON mls_key_packages(user_id);
CREATE INDEX idx_mls_key_packages_expires ON mls_key_packages(expires_at);

CREATE TABLE mls_welcome_messages (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    channel_id  TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    receiver_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    data        BYTEA NOT NULL,         -- Opaque MLS Welcome bytes
    created_at  TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_mls_welcome_receiver ON mls_welcome_messages(receiver_id);
CREATE INDEX idx_mls_welcome_channel ON mls_welcome_messages(channel_id);

CREATE TABLE mls_group_states (
    channel_id  TEXT PRIMARY KEY REFERENCES channels(id) ON DELETE CASCADE,
    epoch       BIGINT NOT NULL DEFAULT 0,
    tree_hash   BYTEA,                  -- Ratchet tree hash for consistency
    updated_at  TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE mls_commits (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    channel_id  TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    sender_id   TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    epoch       BIGINT NOT NULL,
    data        BYTEA NOT NULL,         -- Opaque MLS Commit bytes
    created_at  TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_mls_commits_channel ON mls_commits(channel_id, epoch);
