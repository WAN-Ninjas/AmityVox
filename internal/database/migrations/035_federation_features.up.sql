-- Migration 035: Phase 10 Federation, Bridges, and Multi-Instance Support
-- Adds federation status tracking, peer controls, bridge configurations,
-- delivery receipts, federated search opt-in, and protocol versioning.

-- ============================================================
-- FEDERATION PEER STATUS (health, sync status, event lag)
-- ============================================================

CREATE TABLE IF NOT EXISTS federation_peer_status (
    peer_id         TEXT PRIMARY KEY,
    instance_id     TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'unknown',   -- 'healthy', 'degraded', 'unreachable', 'unknown'
    last_sync_at    TIMESTAMPTZ,
    last_event_at   TIMESTAMPTZ,
    event_lag_ms    INTEGER NOT NULL DEFAULT 0,
    events_sent     BIGINT NOT NULL DEFAULT 0,
    events_received BIGINT NOT NULL DEFAULT 0,
    errors_24h      INTEGER NOT NULL DEFAULT 0,
    version         TEXT,                              -- remote protocol version
    capabilities    JSONB NOT NULL DEFAULT '[]',       -- negotiated capabilities
    last_check_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_federation_peer_status_instance ON federation_peer_status(instance_id);

-- ============================================================
-- FEDERATION PEER CONTROLS (allow/block per peer)
-- ============================================================

CREATE TABLE IF NOT EXISTS federation_peer_controls (
    id              TEXT PRIMARY KEY,                   -- ULID
    instance_id     TEXT NOT NULL,
    peer_id         TEXT NOT NULL,
    action          TEXT NOT NULL DEFAULT 'allow',      -- 'allow', 'block', 'mute'
    reason          TEXT,
    created_by      TEXT NOT NULL REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(instance_id, peer_id)
);

CREATE INDEX IF NOT EXISTS idx_federation_peer_controls_instance ON federation_peer_controls(instance_id);

-- ============================================================
-- FEDERATION DELIVERY RECEIPTS (message delivery tracking)
-- ============================================================

CREATE TABLE IF NOT EXISTS federation_delivery_receipts (
    id              TEXT PRIMARY KEY,                   -- ULID
    message_id      TEXT NOT NULL,
    source_instance TEXT NOT NULL,
    target_instance TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending',    -- 'pending', 'delivered', 'failed', 'retrying'
    attempts        INTEGER NOT NULL DEFAULT 0,
    last_attempt_at TIMESTAMPTZ,
    delivered_at    TIMESTAMPTZ,
    error_message   TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fed_delivery_receipts_message ON federation_delivery_receipts(message_id);
CREATE INDEX IF NOT EXISTS idx_fed_delivery_receipts_status ON federation_delivery_receipts(status) WHERE status != 'delivered';
CREATE INDEX IF NOT EXISTS idx_fed_delivery_receipts_target ON federation_delivery_receipts(target_instance, created_at DESC);

-- ============================================================
-- FEDERATION SEARCH CONFIG (opt-in per instance)
-- ============================================================

CREATE TABLE IF NOT EXISTS federation_search_config (
    instance_id     TEXT PRIMARY KEY,
    enabled         BOOLEAN NOT NULL DEFAULT false,     -- opt-in to federated search
    index_outgoing  BOOLEAN NOT NULL DEFAULT false,     -- allow others to search our content
    index_incoming  BOOLEAN NOT NULL DEFAULT false,     -- index remote content locally
    allowed_peers   JSONB NOT NULL DEFAULT '[]',        -- peer IDs allowed for search
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================================================
-- BRIDGE CONFIGURATIONS
-- ============================================================

CREATE TABLE IF NOT EXISTS bridge_configs (
    id              TEXT PRIMARY KEY,                   -- ULID
    instance_id     TEXT NOT NULL,
    bridge_type     TEXT NOT NULL,                      -- 'matrix', 'discord', 'telegram', 'slack', 'irc', 'xmpp'
    enabled         BOOLEAN NOT NULL DEFAULT false,
    display_name    TEXT NOT NULL DEFAULT '',
    config          JSONB NOT NULL DEFAULT '{}',        -- bridge-specific config (tokens, endpoints)
    status          TEXT NOT NULL DEFAULT 'disconnected', -- 'connected', 'disconnected', 'error', 'configuring'
    last_sync_at    TIMESTAMPTZ,
    error_message   TEXT,
    created_by      TEXT NOT NULL REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(instance_id, bridge_type)
);

CREATE INDEX IF NOT EXISTS idx_bridge_configs_instance ON bridge_configs(instance_id);

-- ============================================================
-- BRIDGE CHANNEL MAPPINGS
-- ============================================================

CREATE TABLE IF NOT EXISTS bridge_channel_mappings (
    id              TEXT PRIMARY KEY,                   -- ULID
    bridge_id       TEXT NOT NULL REFERENCES bridge_configs(id) ON DELETE CASCADE,
    local_channel_id TEXT NOT NULL,
    remote_channel_id TEXT NOT NULL,                    -- channel/room ID on remote platform
    remote_channel_name TEXT,                           -- display name on remote platform
    direction       TEXT NOT NULL DEFAULT 'bidirectional', -- 'bidirectional', 'inbound', 'outbound'
    active          BOOLEAN NOT NULL DEFAULT true,
    last_message_at TIMESTAMPTZ,
    message_count   BIGINT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_bridge_channel_mappings_bridge ON bridge_channel_mappings(bridge_id);
CREATE INDEX IF NOT EXISTS idx_bridge_channel_mappings_local ON bridge_channel_mappings(local_channel_id);

-- ============================================================
-- BRIDGE VIRTUAL USERS (users created for remote platform users)
-- ============================================================

CREATE TABLE IF NOT EXISTS bridge_virtual_users (
    id              TEXT PRIMARY KEY,                   -- ULID
    bridge_id       TEXT NOT NULL REFERENCES bridge_configs(id) ON DELETE CASCADE,
    local_user_id   TEXT REFERENCES users(id) ON DELETE SET NULL,  -- puppet user
    remote_user_id  TEXT NOT NULL,                      -- user ID on remote platform
    remote_username TEXT NOT NULL,
    remote_avatar   TEXT,                               -- URL to avatar on remote platform
    platform        TEXT NOT NULL,                      -- same as bridge_type
    last_active_at  TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(bridge_id, remote_user_id)
);

CREATE INDEX IF NOT EXISTS idx_bridge_virtual_users_bridge ON bridge_virtual_users(bridge_id);
CREATE INDEX IF NOT EXISTS idx_bridge_virtual_users_local ON bridge_virtual_users(local_user_id);

-- ============================================================
-- BRIDGE MESSAGE ATTRIBUTION (mark which messages came from bridges)
-- ============================================================

ALTER TABLE messages ADD COLUMN IF NOT EXISTS bridge_source TEXT;          -- 'matrix', 'discord', etc.
ALTER TABLE messages ADD COLUMN IF NOT EXISTS bridge_remote_id TEXT;       -- original message ID on remote platform
ALTER TABLE messages ADD COLUMN IF NOT EXISTS bridge_author_name TEXT;     -- original author name on remote platform
ALTER TABLE messages ADD COLUMN IF NOT EXISTS bridge_author_avatar TEXT;   -- original author avatar URL

-- ============================================================
-- FEDERATION PROTOCOL CAPABILITIES (per instance)
-- ============================================================

ALTER TABLE instances ADD COLUMN IF NOT EXISTS protocol_version TEXT DEFAULT 'amityvox-federation/1.0';
ALTER TABLE instances ADD COLUMN IF NOT EXISTS capabilities JSONB DEFAULT '[]';

-- ============================================================
-- INSTANCE CONNECTION PROFILES (multi-instance client support)
-- ============================================================

CREATE TABLE IF NOT EXISTS instance_connection_profiles (
    id              TEXT PRIMARY KEY,                   -- ULID
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    instance_url    TEXT NOT NULL,
    instance_name   TEXT,
    instance_icon   TEXT,
    session_token   TEXT,                               -- encrypted at rest
    is_primary      BOOLEAN NOT NULL DEFAULT false,
    last_connected  TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, instance_url)
);

CREATE INDEX IF NOT EXISTS idx_instance_connection_profiles_user ON instance_connection_profiles(user_id);
