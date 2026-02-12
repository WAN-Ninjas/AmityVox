-- Rollback migration 018: Remove dedicated user_blocks table.

DROP INDEX IF EXISTS idx_user_blocks_target;
DROP INDEX IF EXISTS idx_user_blocks_user;
DROP TABLE IF EXISTS user_blocks;
