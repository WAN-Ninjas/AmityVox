-- Migration 056: User-pinned DM channels
CREATE TABLE user_pinned_dms (
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    position   INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, channel_id)
);
CREATE INDEX idx_user_pinned_dms_user ON user_pinned_dms(user_id, position);
