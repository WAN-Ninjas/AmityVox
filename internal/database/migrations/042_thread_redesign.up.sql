-- 042_thread_redesign: Add parent_channel_id and last_activity_at to channels,
-- create user_hidden_threads table, and backfill existing threads.

-- Add parent_channel_id FK from thread to its parent channel.
ALTER TABLE channels ADD COLUMN parent_channel_id TEXT REFERENCES channels(id) ON DELETE CASCADE;
CREATE INDEX idx_channels_parent_channel_id ON channels(parent_channel_id) WHERE parent_channel_id IS NOT NULL;

-- Add last_activity_at to track the most recent message in a thread.
ALTER TABLE channels ADD COLUMN last_activity_at TIMESTAMPTZ;

-- Per-user thread hide preferences.
CREATE TABLE user_hidden_threads (
    user_id   TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    thread_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    hidden_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, thread_id)
);

-- Backfill parent_channel_id for existing threads:
-- Threads are channels linked via messages.thread_id → channel.id.
-- The parent channel is the channel where the linking message lives.
UPDATE channels c
SET parent_channel_id = m.channel_id
FROM messages m
WHERE m.thread_id = c.id
  AND c.parent_channel_id IS NULL;

-- Backfill last_activity_at from the most recent message in each thread.
UPDATE channels c
SET last_activity_at = sub.max_created
FROM (
    SELECT channel_id, MAX(created_at) AS max_created
    FROM messages
    WHERE channel_id IN (SELECT id FROM channels WHERE parent_channel_id IS NOT NULL)
    GROUP BY channel_id
) sub
WHERE c.id = sub.channel_id
  AND c.last_activity_at IS NULL;

-- For threads with no messages yet, use the thread's created_at.
UPDATE channels
SET last_activity_at = created_at
WHERE parent_channel_id IS NOT NULL AND last_activity_at IS NULL;
