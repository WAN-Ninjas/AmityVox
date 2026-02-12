-- Rollback Phase 6 Remaining features

DROP TABLE IF EXISTS user_emoji;
DROP TABLE IF EXISTS guild_bumps;
DROP TABLE IF EXISTS guild_guides;

ALTER TABLE guild_events DROP COLUMN IF EXISTS auto_cancel_minutes;
ALTER TABLE users DROP COLUMN IF EXISTS activity_type;
ALTER TABLE users DROP COLUMN IF EXISTS activity_name;
