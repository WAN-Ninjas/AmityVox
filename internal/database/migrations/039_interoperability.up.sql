-- Migration 039: Interoperability — ActivityPub, RSS feeds, calendar sync,
-- email gateway, SMS bridge, Telegram/Slack/IRC bridges

-- ============================================================
-- Integration Configurations (per-guild integration settings)
-- ============================================================
CREATE TABLE IF NOT EXISTS guild_integrations (
    id TEXT PRIMARY KEY,
    guild_id TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    integration_type TEXT NOT NULL,  -- 'activitypub', 'rss', 'calendar', 'email', 'sms'
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
-- RSS Feed Subscriptions
-- ============================================================
CREATE TABLE IF NOT EXISTS rss_feeds (
    id TEXT PRIMARY KEY,
    integration_id TEXT NOT NULL REFERENCES guild_integrations(id) ON DELETE CASCADE,
    feed_url TEXT NOT NULL,
    title TEXT,
    description TEXT,
    last_item_id TEXT,              -- last seen item guid/link to avoid duplicates
    last_item_published_at TIMESTAMPTZ,
    check_interval_seconds INT NOT NULL DEFAULT 900,  -- 15 min default
    last_checked_at TIMESTAMPTZ,
    error_count INT NOT NULL DEFAULT 0,
    last_error TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_rss_feeds_integration ON rss_feeds(integration_id);
CREATE INDEX IF NOT EXISTS idx_rss_feeds_next_check ON rss_feeds(last_checked_at);

-- ============================================================
-- Calendar Sync Connections
-- ============================================================
CREATE TABLE IF NOT EXISTS calendar_connections (
    id TEXT PRIMARY KEY,
    integration_id TEXT NOT NULL REFERENCES guild_integrations(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,         -- 'google', 'caldav', 'ical_url'
    calendar_url TEXT,              -- CalDAV URL or iCal URL
    calendar_name TEXT,
    access_token_encrypted TEXT,    -- encrypted OAuth token for Google Calendar
    refresh_token_encrypted TEXT,
    token_expires_at TIMESTAMPTZ,
    sync_direction TEXT NOT NULL DEFAULT 'import',  -- 'import', 'export', 'both'
    last_synced_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_calendar_connections_integration ON calendar_connections(integration_id);

-- ============================================================
-- Email Gateway Channels
-- ============================================================
CREATE TABLE IF NOT EXISTS email_gateways (
    id TEXT PRIMARY KEY,
    integration_id TEXT NOT NULL REFERENCES guild_integrations(id) ON DELETE CASCADE,
    email_address TEXT NOT NULL UNIQUE,  -- e.g. channel-abc123@amityvox.chat
    allowed_senders TEXT[] DEFAULT '{}', -- empty = allow all, or list of email patterns
    strip_signatures BOOLEAN NOT NULL DEFAULT true,
    max_attachment_size_mb INT NOT NULL DEFAULT 10,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_email_gateways_integration ON email_gateways(integration_id);
CREATE INDEX IF NOT EXISTS idx_email_gateways_email ON email_gateways(email_address);

-- ============================================================
-- SMS Bridge Connections
-- ============================================================
CREATE TABLE IF NOT EXISTS sms_bridges (
    id TEXT PRIMARY KEY,
    integration_id TEXT NOT NULL REFERENCES guild_integrations(id) ON DELETE CASCADE,
    provider TEXT NOT NULL DEFAULT 'twilio',  -- 'twilio', 'vonage'
    phone_number TEXT NOT NULL,               -- E.164 format
    api_key_encrypted TEXT NOT NULL,
    api_secret_encrypted TEXT NOT NULL,
    account_sid TEXT,                          -- Twilio Account SID
    allowed_numbers TEXT[] DEFAULT '{}',       -- empty = allow all
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sms_bridges_integration ON sms_bridges(integration_id);

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
