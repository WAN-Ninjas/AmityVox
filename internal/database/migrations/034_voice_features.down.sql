-- Rollback Migration 034: Phase 7 Voice/Media Features

DROP TABLE IF EXISTS screen_share_sessions;
DROP TABLE IF EXISTS voice_broadcasts;
DROP TABLE IF EXISTS soundboard_play_log;
DROP TABLE IF EXISTS soundboard_config;
DROP TABLE IF EXISTS soundboard_sounds;
DROP TABLE IF EXISTS voice_preferences;
