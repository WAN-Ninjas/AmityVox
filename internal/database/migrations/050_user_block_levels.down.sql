-- Rollback migration 050: Remove block level from user_blocks.

ALTER TABLE user_blocks DROP CONSTRAINT IF EXISTS user_blocks_level_check;
ALTER TABLE user_blocks DROP COLUMN IF EXISTS level;
