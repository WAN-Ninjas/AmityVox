-- Add archived column to channels for archiving instead of deleting.
ALTER TABLE channels ADD COLUMN IF NOT EXISTS archived BOOLEAN NOT NULL DEFAULT false;
