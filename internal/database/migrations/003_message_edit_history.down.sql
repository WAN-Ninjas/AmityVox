-- Rollback migration 003

DROP INDEX IF EXISTS idx_message_edits_message;
DROP TABLE IF EXISTS message_edits;
