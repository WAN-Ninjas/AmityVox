-- Rollback Phase 6: Community & Social Features

DROP TABLE IF EXISTS event_rsvps;
DROP TABLE IF EXISTS guild_events;
DROP TABLE IF EXISTS message_bookmarks;
DROP TABLE IF EXISTS poll_votes;
DROP TABLE IF EXISTS poll_options;
DROP TABLE IF EXISTS polls;

ALTER TABLE users DROP COLUMN IF EXISTS status_emoji;
ALTER TABLE users DROP COLUMN IF EXISTS status_expires_at;
ALTER TABLE users DROP COLUMN IF EXISTS banner_id;
ALTER TABLE users DROP COLUMN IF EXISTS accent_color;
ALTER TABLE users DROP COLUMN IF EXISTS pronouns;
