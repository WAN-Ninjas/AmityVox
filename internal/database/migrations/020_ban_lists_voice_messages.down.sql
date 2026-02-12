ALTER TABLE messages DROP COLUMN IF EXISTS voice_waveform;
ALTER TABLE messages DROP COLUMN IF EXISTS voice_duration_ms;
DROP TABLE IF EXISTS ban_list_subscriptions;
DROP TABLE IF EXISTS ban_list_entries;
DROP TABLE IF EXISTS ban_lists;
