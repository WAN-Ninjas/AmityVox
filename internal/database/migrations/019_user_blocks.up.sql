-- Migration 018: Add dedicated user_blocks table for improved blocking.
-- The user_relationships table continues to track blocked status for
-- relationship-based queries, while user_blocks provides richer metadata
-- (reason, context) and an efficient lookup path for block lists.

CREATE TABLE user_blocks (
    id          TEXT PRIMARY KEY,              -- ULID
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    target_id   TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reason      TEXT,                          -- Optional reason for the block
    created_at  TIMESTAMPTZ DEFAULT now(),
    UNIQUE(user_id, target_id)
);

-- Index for fast blocked-user lookups per user.
CREATE INDEX idx_user_blocks_user ON user_blocks(user_id, created_at DESC);

-- Index for checking if a specific user is blocked by another.
CREATE INDEX idx_user_blocks_target ON user_blocks(target_id, user_id);
