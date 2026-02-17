DROP INDEX IF EXISTS idx_users_last_online;
ALTER TABLE users DROP COLUMN IF EXISTS last_online;
