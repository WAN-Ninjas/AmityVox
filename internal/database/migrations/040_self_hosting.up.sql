-- Self-Hosting Excellence: setup wizard, auto-update, health monitoring, storage
-- dashboard, data retention policies, custom domains, backup scheduling.

-- Data retention policies (per-channel or instance-wide).
CREATE TABLE IF NOT EXISTS data_retention_policies (
    id TEXT PRIMARY KEY,
    channel_id TEXT REFERENCES channels(id) ON DELETE CASCADE,
    guild_id TEXT REFERENCES guilds(id) ON DELETE CASCADE,
    max_age_days INT NOT NULL DEFAULT 365,
    delete_attachments BOOLEAN NOT NULL DEFAULT true,
    delete_pins BOOLEAN NOT NULL DEFAULT false,
    enabled BOOLEAN NOT NULL DEFAULT true,
    last_run_at TIMESTAMPTZ,
    next_run_at TIMESTAMPTZ,
    messages_deleted BIGINT NOT NULL DEFAULT 0,
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Instance-wide default retention (only one row, channel_id IS NULL).
CREATE UNIQUE INDEX IF NOT EXISTS idx_retention_instance_default
    ON data_retention_policies (guild_id) WHERE channel_id IS NULL AND guild_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_retention_channel
    ON data_retention_policies (channel_id) WHERE channel_id IS NOT NULL;

-- Custom domain aliases for guilds (guild.example.com -> guild page).
CREATE TABLE IF NOT EXISTS guild_custom_domains (
    id TEXT PRIMARY KEY,
    guild_id TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    domain TEXT NOT NULL UNIQUE,
    verified BOOLEAN NOT NULL DEFAULT false,
    verification_token TEXT NOT NULL,
    ssl_provisioned BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    verified_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_guild_custom_domains_guild
    ON guild_custom_domains (guild_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_guild_custom_domains_domain
    ON guild_custom_domains (domain);

-- Backup schedule configuration.
CREATE TABLE IF NOT EXISTS backup_schedules (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    frequency TEXT NOT NULL DEFAULT 'daily',
    retention_count INT NOT NULL DEFAULT 7,
    include_media BOOLEAN NOT NULL DEFAULT false,
    include_database BOOLEAN NOT NULL DEFAULT true,
    storage_path TEXT NOT NULL DEFAULT '/backups',
    enabled BOOLEAN NOT NULL DEFAULT true,
    last_run_at TIMESTAMPTZ,
    last_run_status TEXT,
    last_run_size_bytes BIGINT,
    next_run_at TIMESTAMPTZ,
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Backup execution history.
CREATE TABLE IF NOT EXISTS backup_history (
    id TEXT PRIMARY KEY,
    schedule_id TEXT NOT NULL REFERENCES backup_schedules(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'running',
    size_bytes BIGINT,
    file_path TEXT,
    error_message TEXT,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_backup_history_schedule
    ON backup_history (schedule_id, created_at DESC);

-- Service health snapshots for monitoring dashboard.
CREATE TABLE IF NOT EXISTS health_snapshots (
    id TEXT PRIMARY KEY,
    service TEXT NOT NULL,
    status TEXT NOT NULL,
    response_time_ms INT,
    details JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_health_snapshots_service_time
    ON health_snapshots (service, created_at DESC);

-- Auto-purge old health snapshots (keep 7 days).
CREATE INDEX IF NOT EXISTS idx_health_snapshots_cleanup
    ON health_snapshots (created_at);
