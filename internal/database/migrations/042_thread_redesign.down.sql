-- 041_thread_redesign: Reverse thread redesign changes.

DROP TABLE IF EXISTS user_hidden_threads;
DROP INDEX IF EXISTS idx_channels_parent_channel_id;
ALTER TABLE channels DROP COLUMN IF EXISTS last_activity_at;
ALTER TABLE channels DROP COLUMN IF EXISTS parent_channel_id;
