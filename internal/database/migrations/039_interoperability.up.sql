-- Migration 039: Interoperability — ActivityPub, Telegram/Slack/IRC bridges

-- ============================================================
-- Integration Configurations (per-guild integration settings)
-- ============================================================
CREATE TABLE IF NOT EXISTS guild_integrations (
    id TEXT PRIMARY KEY,
    guild_id TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    integration_type TEXT NOT NULL,  -- 'activitypub'
    channel_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    config JSONB NOT NULL DEFAULT '{}',
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_guild_integrations_guild ON guild_integrations(guild_id);
CREATE INDEX IF NOT EXISTS idx_guild_integrations_type ON guild_integrations(integration_type);
CREATE INDEX IF NOT EXISTS idx_guild_integrations_channel ON guild_integrations(channel_id);

-- ============================================================
-- ActivityPub Actors (federated accounts this instance follows)
-- ============================================================
CREATE TABLE IF NOT EXISTS activitypub_follows (
    id TEXT PRIMARY KEY,
    integration_id TEXT NOT NULL REFERENCES guild_integrations(id) ON DELETE CASCADE,
    actor_uri TEXT NOT NULL,        -- e.g. https://mastodon.social/users/alice
    actor_inbox TEXT,               -- inbox URL for posting
    actor_name TEXT,                -- display name
    actor_handle TEXT,              -- @alice@mastodon.social
    actor_avatar_url TEXT,
    last_fetched_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_activitypub_follows_integration ON activitypub_follows(integration_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_activitypub_follows_unique ON activitypub_follows(integration_id, actor_uri);

-- ============================================================
-- Bridge Adapter Connections (Telegram, Slack, IRC)
-- ============================================================
CREATE TABLE IF NOT EXISTS bridge_connections (
    id TEXT PRIMARY KEY,
    guild_id TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    bridge_type TEXT NOT NULL,      -- 'telegram', 'slack', 'irc'
    channel_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    remote_id TEXT NOT NULL,        -- Telegram chat ID, Slack channel ID, IRC #channel
    enabled BOOLEAN NOT NULL DEFAULT true,
    config JSONB NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'disconnected',  -- 'connected', 'disconnected', 'error'
    last_error TEXT,
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_bridge_connections_guild ON bridge_connections(guild_id);
CREATE INDEX IF NOT EXISTS idx_bridge_connections_type ON bridge_connections(bridge_type);
CREATE INDEX IF NOT EXISTS idx_bridge_connections_channel ON bridge_connections(channel_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_bridge_connections_unique ON bridge_connections(bridge_type, remote_id);

-- ============================================================
-- Integration Message Log (audit trail for bridged messages)
-- ============================================================
CREATE TABLE IF NOT EXISTS integration_message_log (
    id TEXT PRIMARY KEY,
    integration_id TEXT REFERENCES guild_integrations(id) ON DELETE SET NULL,
    bridge_connection_id TEXT REFERENCES bridge_connections(id) ON DELETE SET NULL,
    direction TEXT NOT NULL,        -- 'inbound' or 'outbound'
    source_id TEXT,                 -- external message ID
    amityvox_message_id TEXT,       -- local message ID
    channel_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'delivered',  -- 'delivered', 'failed', 'filtered'
    error_message TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_integration_msg_log_channel ON integration_message_log(channel_id);
CREATE INDEX IF NOT EXISTS idx_integration_msg_log_created ON integration_message_log(created_at);
