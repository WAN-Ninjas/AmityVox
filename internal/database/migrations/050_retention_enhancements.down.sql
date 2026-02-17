DROP INDEX IF EXISTS idx_retention_next_run;
DELETE FROM instance_settings WHERE key = 'min_retention_days';
