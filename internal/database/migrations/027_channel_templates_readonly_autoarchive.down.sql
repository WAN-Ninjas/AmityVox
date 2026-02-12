-- Rollback migration 027: Remove channel templates, read-only, and auto-archive duration.

DROP TABLE IF EXISTS channel_templates;

ALTER TABLE channels DROP COLUMN IF EXISTS read_only;
ALTER TABLE channels DROP COLUMN IF EXISTS read_only_role_ids;
ALTER TABLE channels DROP COLUMN IF EXISTS default_auto_archive_duration;
