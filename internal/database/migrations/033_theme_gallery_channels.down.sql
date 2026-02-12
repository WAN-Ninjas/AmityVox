-- Rollback migration 033: Theme Gallery, Channel Emoji, User Channel Groups, Image Compression

-- Remove compressed attachment columns.
ALTER TABLE attachments DROP COLUMN IF EXISTS compressed_s3_key;
ALTER TABLE attachments DROP COLUMN IF EXISTS compressed_size_bytes;

-- Drop user channel group tables.
DROP TABLE IF EXISTS user_channel_group_items;
DROP TABLE IF EXISTS user_channel_groups;

-- Drop channel emoji.
DROP TABLE IF EXISTS channel_emoji;

-- Drop theme likes and shared themes.
DROP TABLE IF EXISTS theme_likes;
DROP TABLE IF EXISTS shared_themes;
