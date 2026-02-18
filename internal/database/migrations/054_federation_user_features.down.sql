-- Migration 054 down: Remove federation user features tables

ALTER TABLE instances DROP COLUMN IF EXISTS livekit_url;

DROP TABLE IF EXISTS federation_dead_letters;
DROP TABLE IF EXISTS federation_guild_cache;
DROP TABLE IF EXISTS federation_channel_mirrors;
DROP TABLE IF EXISTS federation_channel_peers;
