-- Remove sticker pack sharing columns.
ALTER TABLE sticker_packs DROP COLUMN IF EXISTS share_code;
ALTER TABLE sticker_packs DROP COLUMN IF EXISTS shared;

-- Drop per-thread notification preferences.
DROP TABLE IF EXISTS thread_notification_preferences;

-- Drop translation cache.
DROP TABLE IF EXISTS translation_cache;

-- Drop cross-channel quotes.
DROP TABLE IF EXISTS cross_channel_quotes;
