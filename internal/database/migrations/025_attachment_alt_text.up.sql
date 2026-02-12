-- Add alt_text column to attachments for accessibility.
ALTER TABLE attachments ADD COLUMN IF NOT EXISTS alt_text TEXT;
