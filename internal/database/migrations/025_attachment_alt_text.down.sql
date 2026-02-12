-- Remove alt_text column from attachments.
ALTER TABLE attachments DROP COLUMN IF EXISTS alt_text;
