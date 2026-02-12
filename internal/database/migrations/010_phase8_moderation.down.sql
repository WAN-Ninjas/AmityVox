-- Reverse Phase 8: Moderation & Safety Enhancements

DROP TABLE IF EXISTS guild_raid_config;

ALTER TABLE channels DROP COLUMN IF EXISTS locked;
ALTER TABLE channels DROP COLUMN IF EXISTS locked_by;
ALTER TABLE channels DROP COLUMN IF EXISTS locked_at;

DROP TABLE IF EXISTS message_reports;
DROP TABLE IF EXISTS member_warnings;

ALTER TABLE guilds DROP COLUMN IF EXISTS verification_level;
