-- Migration 050: Add block level to user_blocks table.
-- Supports two-tier blocking: 'ignore' (hide messages, show in member list)
-- and 'block' (completely hide messages and filter from member list).

ALTER TABLE user_blocks ADD COLUMN level TEXT NOT NULL DEFAULT 'block';
ALTER TABLE user_blocks ADD CONSTRAINT user_blocks_level_check CHECK (level IN ('ignore', 'block'));
