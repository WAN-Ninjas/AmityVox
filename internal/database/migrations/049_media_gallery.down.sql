DROP TABLE IF EXISTS attachment_tags;
DROP TABLE IF EXISTS media_tags;
DROP INDEX IF EXISTS idx_attachments_uploader_id;
DROP INDEX IF EXISTS idx_attachments_message_id;
ALTER TABLE attachments DROP COLUMN IF EXISTS nsfw;
ALTER TABLE attachments DROP COLUMN IF EXISTS description;
