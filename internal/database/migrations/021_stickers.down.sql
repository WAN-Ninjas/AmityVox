ALTER TABLE message_bookmarks DROP COLUMN IF EXISTS reminded;
ALTER TABLE message_bookmarks DROP COLUMN IF EXISTS reminder_at;
DROP TABLE IF EXISTS channel_templates;
DROP TABLE IF EXISTS stickers;
DROP TABLE IF EXISTS sticker_packs;
