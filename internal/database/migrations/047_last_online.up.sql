ALTER TABLE users ADD COLUMN IF NOT EXISTS last_online TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_users_last_online ON users (last_online) WHERE last_online IS NOT NULL;
