-- Rollback migration 002

DROP INDEX IF EXISTS idx_guilds_vanity;
ALTER TABLE guilds DROP COLUMN IF EXISTS vanity_url;
DROP TABLE IF EXISTS user_settings;
DROP TABLE IF EXISTS user_notes;
