-- AmityVox initial database schema
-- This migration creates all tables, indexes, and constraints for the core
-- data model as specified in docs/architecture.md Section 5.2.

-- ============================================================
-- INSTANCE & IDENTITY
-- ============================================================

CREATE TABLE instances (
    id              TEXT PRIMARY KEY,
    domain          TEXT UNIQUE NOT NULL,
    public_key      TEXT NOT NULL,               -- Ed25519 public key (PEM)
    name            TEXT,
    description     TEXT,
    software        TEXT DEFAULT 'amityvox',     -- For identifying instance software
    software_version TEXT,
    federation_mode TEXT DEFAULT 'open'
        CHECK (federation_mode IN ('open', 'allowlist', 'closed')),
    created_at      TIMESTAMPTZ DEFAULT now(),
    last_seen_at    TIMESTAMPTZ
);

CREATE TABLE users (
    id              TEXT PRIMARY KEY,            -- ULID
    instance_id     TEXT NOT NULL REFERENCES instances(id),
    username        TEXT NOT NULL,
    display_name    TEXT,
    avatar_id       TEXT,                        -- FK to attachments
    status_text     TEXT,
    status_presence TEXT DEFAULT 'offline'
        CHECK (status_presence IN ('online','idle','focus','busy','invisible','offline')),
    bio             TEXT,
    bot_owner_id    TEXT REFERENCES users(id),   -- NULL = human
    password_hash   TEXT,                        -- Argon2id (NULL for remote/federated users)
    totp_secret     TEXT,                        -- TOTP 2FA secret (encrypted at rest)
    email           TEXT,                        -- Optional, for recovery
    flags           INTEGER DEFAULT 0,           -- Suspended, deleted, admin, etc.
    created_at      TIMESTAMPTZ DEFAULT now(),
    UNIQUE(username, instance_id)
);

CREATE TABLE user_sessions (
    id              TEXT PRIMARY KEY,            -- Session token (random)
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_name     TEXT,
    ip_address      INET,
    user_agent      TEXT,
    created_at      TIMESTAMPTZ DEFAULT now(),
    last_active_at  TIMESTAMPTZ DEFAULT now(),
    expires_at      TIMESTAMPTZ NOT NULL
);

CREATE TABLE user_relationships (
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    target_id   TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status      TEXT NOT NULL
        CHECK (status IN ('friend','blocked','pending_outgoing','pending_incoming')),
    created_at  TIMESTAMPTZ DEFAULT now(),
    PRIMARY KEY (user_id, target_id)
);

CREATE TABLE webauthn_credentials (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    credential_id   BYTEA NOT NULL UNIQUE,
    public_key      BYTEA NOT NULL,
    sign_count      BIGINT DEFAULT 0,
    name            TEXT,                        -- User-given name ("YubiKey", "TouchID")
    created_at      TIMESTAMPTZ DEFAULT now()
);

-- ============================================================
-- GUILDS & MEMBERSHIP
-- ============================================================

CREATE TABLE guilds (
    id                      TEXT PRIMARY KEY,
    instance_id             TEXT NOT NULL REFERENCES instances(id),
    owner_id                TEXT NOT NULL REFERENCES users(id),
    name                    TEXT NOT NULL,
    description             TEXT,
    icon_id                 TEXT,
    banner_id               TEXT,
    default_permissions     BIGINT DEFAULT 0,    -- @everyone allow bitfield
    flags                   INTEGER DEFAULT 0,
    nsfw                    BOOLEAN DEFAULT false,
    discoverable            BOOLEAN DEFAULT false,
    system_channel_join     TEXT,                 -- Channel ID for join messages
    system_channel_leave    TEXT,
    system_channel_kick     TEXT,
    system_channel_ban      TEXT,
    preferred_locale        TEXT DEFAULT 'en',
    max_members             INTEGER DEFAULT 0,   -- 0 = unlimited
    created_at              TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE guild_categories (
    id          TEXT PRIMARY KEY,
    guild_id    TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    position    INTEGER DEFAULT 0,
    created_at  TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE channels (
    id                      TEXT PRIMARY KEY,
    guild_id                TEXT REFERENCES guilds(id) ON DELETE CASCADE,
    category_id             TEXT REFERENCES guild_categories(id) ON DELETE SET NULL,
    channel_type            TEXT NOT NULL
        CHECK (channel_type IN ('text','voice','dm','group','announcement','forum','stage')),
    name                    TEXT,
    topic                   TEXT,
    position                INTEGER DEFAULT 0,
    slowmode_seconds        INTEGER DEFAULT 0,
    nsfw                    BOOLEAN DEFAULT false,
    encrypted               BOOLEAN DEFAULT false,   -- E2E encryption enabled
    last_message_id         TEXT,                     -- Denormalized for perf
    owner_id                TEXT REFERENCES users(id),-- For group DMs
    default_permissions     BIGINT,                   -- Channel-level @everyone overrides
    created_at              TIMESTAMPTZ DEFAULT now()
);

-- DM/group participants (only for dm/group channel types)
CREATE TABLE channel_recipients (
    channel_id  TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    joined_at   TIMESTAMPTZ DEFAULT now(),
    PRIMARY KEY (channel_id, user_id)
);

CREATE TABLE roles (
    id                  TEXT PRIMARY KEY,
    guild_id            TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    name                TEXT NOT NULL,
    color               TEXT,                    -- CSS hex color
    hoist               BOOLEAN DEFAULT false,   -- Show separately in member list
    mentionable         BOOLEAN DEFAULT false,
    position            INTEGER DEFAULT 0,       -- Lower = higher priority
    permissions_allow   BIGINT DEFAULT 0,
    permissions_deny    BIGINT DEFAULT 0,
    created_at          TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE guild_members (
    guild_id        TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    nickname        TEXT,
    avatar_id       TEXT,
    joined_at       TIMESTAMPTZ DEFAULT now(),
    timeout_until   TIMESTAMPTZ,                 -- NULL = not timed out
    deaf            BOOLEAN DEFAULT false,
    mute            BOOLEAN DEFAULT false,
    PRIMARY KEY (guild_id, user_id)
);

CREATE TABLE member_roles (
    guild_id    TEXT NOT NULL,
    user_id     TEXT NOT NULL,
    role_id     TEXT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    PRIMARY KEY (guild_id, user_id, role_id),
    FOREIGN KEY (guild_id, user_id)
        REFERENCES guild_members(guild_id, user_id) ON DELETE CASCADE
);

CREATE TABLE channel_permission_overrides (
    channel_id          TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    target_type         TEXT NOT NULL CHECK (target_type IN ('role', 'user')),
    target_id           TEXT NOT NULL,
    permissions_allow   BIGINT DEFAULT 0,
    permissions_deny    BIGINT DEFAULT 0,
    PRIMARY KEY (channel_id, target_type, target_id)
);

-- ============================================================
-- MESSAGES & CONTENT
-- ============================================================

CREATE TABLE messages (
    id              TEXT PRIMARY KEY,            -- ULID (sortable = creation order)
    channel_id      TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    author_id       TEXT NOT NULL REFERENCES users(id),
    content         TEXT,                        -- Markdown text (NULL for system messages)
    nonce           TEXT,                        -- Client dedup token
    message_type    TEXT DEFAULT 'default'
        CHECK (message_type IN ('default','system_join','system_leave','system_kick',
               'system_ban','system_pin','reply','thread_created')),
    edited_at       TIMESTAMPTZ,
    flags           INTEGER DEFAULT 0,
    reply_to_ids    TEXT[],                      -- Message IDs being replied to
    mention_user_ids TEXT[],                     -- Denormalized for push notifications
    mention_role_ids TEXT[],
    mention_everyone BOOLEAN DEFAULT false,
    thread_id       TEXT REFERENCES channels(id),-- If this message started a thread
    -- Masquerade (for bridge bots / webhooks)
    masquerade_name   TEXT,
    masquerade_avatar TEXT,
    masquerade_color  TEXT,
    -- E2E encryption fields
    encrypted       BOOLEAN DEFAULT false,
    encryption_session_id TEXT,                  -- MLS group session reference
    created_at      TIMESTAMPTZ DEFAULT now(),
    UNIQUE(channel_id, nonce)                    -- Dedup constraint
);

CREATE TABLE attachments (
    id              TEXT PRIMARY KEY,
    message_id      TEXT REFERENCES messages(id) ON DELETE SET NULL,
    uploader_id     TEXT REFERENCES users(id),
    filename        TEXT NOT NULL,
    content_type    TEXT NOT NULL,
    size_bytes      BIGINT NOT NULL,
    width           INTEGER,                     -- For images/video
    height          INTEGER,
    duration_seconds REAL,                       -- For audio/video
    s3_bucket       TEXT NOT NULL,
    s3_key          TEXT NOT NULL,
    blurhash        TEXT,                        -- Image placeholder hash
    created_at      TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE embeds (
    id              TEXT PRIMARY KEY,
    message_id      TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    embed_type      TEXT NOT NULL
        CHECK (embed_type IN ('website','image','video','rich','special')),
    url             TEXT,
    title           TEXT,
    description     TEXT,
    site_name       TEXT,
    icon_url        TEXT,
    color           TEXT,                        -- Accent color
    image_url       TEXT,
    image_width     INTEGER,
    image_height    INTEGER,
    video_url       TEXT,
    special_type    TEXT,                        -- 'youtube', 'twitch', 'spotify', etc.
    special_id      TEXT,                        -- Platform-specific content ID
    created_at      TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE reactions (
    message_id  TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    emoji       TEXT NOT NULL,                   -- Unicode or custom emoji ID
    created_at  TIMESTAMPTZ DEFAULT now(),
    PRIMARY KEY (message_id, user_id, emoji)
);

CREATE TABLE pins (
    channel_id  TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    message_id  TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    pinned_by   TEXT NOT NULL REFERENCES users(id),
    pinned_at   TIMESTAMPTZ DEFAULT now(),
    PRIMARY KEY (channel_id, message_id)
);

-- ============================================================
-- INVITES & BANS
-- ============================================================

CREATE TABLE invites (
    code        TEXT PRIMARY KEY,
    guild_id    TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    channel_id  TEXT REFERENCES channels(id) ON DELETE SET NULL,
    creator_id  TEXT REFERENCES users(id) ON DELETE SET NULL,
    max_uses    INTEGER,                         -- NULL = unlimited
    uses        INTEGER DEFAULT 0,
    max_age_seconds INTEGER,                     -- NULL = never expires
    temporary   BOOLEAN DEFAULT false,           -- Temporary membership
    created_at  TIMESTAMPTZ DEFAULT now(),
    expires_at  TIMESTAMPTZ                      -- Computed from max_age_seconds
);

CREATE TABLE guild_bans (
    guild_id    TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reason      TEXT,
    banned_by   TEXT REFERENCES users(id),
    created_at  TIMESTAMPTZ DEFAULT now(),
    PRIMARY KEY (guild_id, user_id)
);

-- ============================================================
-- CUSTOM EMOJI
-- ============================================================

CREATE TABLE custom_emoji (
    id          TEXT PRIMARY KEY,
    guild_id    TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    creator_id  TEXT REFERENCES users(id),
    animated    BOOLEAN DEFAULT false,
    s3_key      TEXT NOT NULL,
    created_at  TIMESTAMPTZ DEFAULT now(),
    UNIQUE(guild_id, name)
);

-- ============================================================
-- WEBHOOKS
-- ============================================================

CREATE TABLE webhooks (
    id          TEXT PRIMARY KEY,
    guild_id    TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    channel_id  TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    creator_id  TEXT REFERENCES users(id),
    name        TEXT NOT NULL,
    avatar_id   TEXT,
    token       TEXT NOT NULL UNIQUE,            -- Secret token for posting
    webhook_type TEXT DEFAULT 'incoming'
        CHECK (webhook_type IN ('incoming', 'outgoing')),
    outgoing_url TEXT,                           -- For outgoing webhooks
    created_at  TIMESTAMPTZ DEFAULT now()
);

-- ============================================================
-- AUDIT LOG
-- ============================================================

CREATE TABLE audit_log (
    id          TEXT PRIMARY KEY,                -- ULID
    guild_id    TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    actor_id    TEXT NOT NULL REFERENCES users(id),
    action      TEXT NOT NULL,                   -- 'member_kick', 'channel_create', etc.
    target_type TEXT,                            -- 'user', 'channel', 'role', etc.
    target_id   TEXT,
    reason      TEXT,
    changes     JSONB,                           -- Before/after snapshot
    created_at  TIMESTAMPTZ DEFAULT now()
);

-- ============================================================
-- FEDERATION
-- ============================================================

CREATE TABLE federation_peers (
    instance_id     TEXT NOT NULL REFERENCES instances(id),
    peer_id         TEXT NOT NULL REFERENCES instances(id),
    status          TEXT DEFAULT 'active'
        CHECK (status IN ('active', 'blocked', 'pending')),
    established_at  TIMESTAMPTZ DEFAULT now(),
    last_synced_at  TIMESTAMPTZ,
    PRIMARY KEY (instance_id, peer_id)
);

-- ============================================================
-- READ STATE & NOTIFICATIONS
-- ============================================================

CREATE TABLE read_state (
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel_id      TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    last_read_id    TEXT,                        -- Last read message ULID
    mention_count   INTEGER DEFAULT 0,
    PRIMARY KEY (user_id, channel_id)
);

-- ============================================================
-- INDEXES
-- ============================================================

CREATE INDEX idx_messages_channel      ON messages(channel_id, id DESC);
CREATE INDEX idx_messages_author       ON messages(author_id, created_at DESC);
CREATE INDEX idx_messages_nonce        ON messages(channel_id, nonce) WHERE nonce IS NOT NULL;
CREATE INDEX idx_channels_guild        ON channels(guild_id, position);
CREATE INDEX idx_channels_category     ON channels(category_id, position);
CREATE INDEX idx_guild_members_user    ON guild_members(user_id);
CREATE INDEX idx_member_roles_role     ON member_roles(role_id);
CREATE INDEX idx_attachments_message   ON attachments(message_id);
CREATE INDEX idx_reactions_message     ON reactions(message_id);
CREATE INDEX idx_embeds_message        ON embeds(message_id);
CREATE INDEX idx_audit_log_guild       ON audit_log(guild_id, created_at DESC);
CREATE INDEX idx_audit_log_actor       ON audit_log(actor_id, created_at DESC);
CREATE INDEX idx_user_sessions_user    ON user_sessions(user_id);
CREATE INDEX idx_user_sessions_expiry  ON user_sessions(expires_at);
CREATE INDEX idx_read_state_user       ON read_state(user_id);
CREATE INDEX idx_invites_guild         ON invites(guild_id);
CREATE INDEX idx_custom_emoji_guild    ON custom_emoji(guild_id);
CREATE INDEX idx_webhooks_channel      ON webhooks(channel_id);
CREATE INDEX idx_users_instance        ON users(instance_id);
