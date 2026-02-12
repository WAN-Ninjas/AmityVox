-- Migration 036: Extensibility features — widgets, plugins, key backup

-- ============================================================
-- Server Widgets (embeddable widget for external websites)
-- ============================================================
CREATE TABLE IF NOT EXISTS guild_widgets (
    guild_id TEXT PRIMARY KEY REFERENCES guilds(id) ON DELETE CASCADE,
    enabled BOOLEAN NOT NULL DEFAULT false,
    invite_channel_id TEXT REFERENCES channels(id) ON DELETE SET NULL,
    style TEXT NOT NULL DEFAULT 'banner_1',  -- 'banner_1', 'banner_2', 'shield'
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================================
-- Channel Widgets (interactive embeds within channels)
-- ============================================================
CREATE TABLE IF NOT EXISTS channel_widgets (
    id TEXT PRIMARY KEY,
    channel_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    guild_id TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    widget_type TEXT NOT NULL,  -- 'notes', 'youtube', 'countdown', 'custom_iframe'
    title TEXT NOT NULL,
    config JSONB NOT NULL DEFAULT '{}',
    creator_id TEXT NOT NULL REFERENCES users(id),
    position INT NOT NULL DEFAULT 0,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_channel_widgets_channel ON channel_widgets(channel_id);
CREATE INDEX IF NOT EXISTS idx_channel_widgets_guild ON channel_widgets(guild_id);

-- ============================================================
-- Widget Permissions (who can add/remove/manage widgets)
-- ============================================================
CREATE TABLE IF NOT EXISTS widget_permissions (
    id TEXT PRIMARY KEY,
    guild_id TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    role_id TEXT REFERENCES roles(id) ON DELETE CASCADE,
    user_id TEXT REFERENCES users(id) ON DELETE CASCADE,
    can_add BOOLEAN NOT NULL DEFAULT false,
    can_remove BOOLEAN NOT NULL DEFAULT false,
    can_configure BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT widget_perm_target CHECK (
        (role_id IS NOT NULL AND user_id IS NULL) OR
        (role_id IS NULL AND user_id IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_widget_permissions_guild ON widget_permissions(guild_id);

-- ============================================================
-- Plugin System — installed plugins per guild
-- ============================================================
CREATE TABLE IF NOT EXISTS plugins (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    author TEXT NOT NULL,
    version TEXT NOT NULL DEFAULT '1.0.0',
    homepage_url TEXT,
    icon_url TEXT,
    wasm_s3_key TEXT,              -- S3 key for the WASM module
    wasm_hash TEXT,                -- SHA-256 hash of the WASM module for integrity
    manifest JSONB NOT NULL DEFAULT '{}',  -- hooks, permissions, config schema
    category TEXT NOT NULL DEFAULT 'utility',  -- utility, moderation, fun, integration
    public BOOLEAN NOT NULL DEFAULT true,
    verified BOOLEAN NOT NULL DEFAULT false,
    install_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_plugins_category ON plugins(category);
CREATE INDEX IF NOT EXISTS idx_plugins_public ON plugins(public) WHERE public = true;

CREATE TABLE IF NOT EXISTS guild_plugins (
    id TEXT PRIMARY KEY,
    guild_id TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    plugin_id TEXT NOT NULL REFERENCES plugins(id) ON DELETE CASCADE,
    enabled BOOLEAN NOT NULL DEFAULT true,
    config JSONB NOT NULL DEFAULT '{}',     -- user-configurable settings
    installed_by TEXT NOT NULL REFERENCES users(id),
    installed_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(guild_id, plugin_id)
);

CREATE INDEX IF NOT EXISTS idx_guild_plugins_guild ON guild_plugins(guild_id);
CREATE INDEX IF NOT EXISTS idx_guild_plugins_plugin ON guild_plugins(plugin_id);

-- Plugin execution log for auditing and debugging
CREATE TABLE IF NOT EXISTS plugin_execution_log (
    id TEXT PRIMARY KEY,
    guild_id TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    plugin_id TEXT NOT NULL REFERENCES plugins(id) ON DELETE CASCADE,
    hook_type TEXT NOT NULL,       -- 'message_create', 'member_join', 'scheduled', etc.
    status TEXT NOT NULL,          -- 'success', 'error', 'timeout'
    duration_ms INT NOT NULL DEFAULT 0,
    memory_bytes BIGINT NOT NULL DEFAULT 0,
    error_message TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_plugin_exec_guild ON plugin_execution_log(guild_id);
CREATE INDEX IF NOT EXISTS idx_plugin_exec_created ON plugin_execution_log(created_at);

-- ============================================================
-- Key Backup (E2E encryption MLS key backup/recovery)
-- ============================================================
CREATE TABLE IF NOT EXISTS key_backups (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    encrypted_data BYTEA NOT NULL,     -- AES-256-GCM encrypted key material
    salt BYTEA NOT NULL,               -- PBKDF2 salt for passphrase derivation
    nonce BYTEA NOT NULL,              -- AES-GCM nonce
    key_count INT NOT NULL DEFAULT 0,  -- number of keys stored
    version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_key_backups_user ON key_backups(user_id);

-- Recovery codes for key backup (hashed, one-time use)
CREATE TABLE IF NOT EXISTS key_backup_recovery_codes (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash TEXT NOT NULL,
    used BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_key_recovery_user ON key_backup_recovery_codes(user_id);
