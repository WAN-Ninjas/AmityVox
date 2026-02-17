-- Retention enhancements: instance minimum setting and worker index.

-- Instance-level minimum retention floor (guild owners cannot go below this).
INSERT INTO instance_settings (key, value)
VALUES ('min_retention_days', '0')
ON CONFLICT (key) DO NOTHING;

-- Index for the retention worker to efficiently find policies due for execution.
CREATE INDEX IF NOT EXISTS idx_retention_next_run
    ON data_retention_policies (next_run_at) WHERE enabled = true;
