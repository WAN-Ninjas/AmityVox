# AmityVox — Architecture Specification v1.0

## Federated Chat Platform — Master Planning Document

*This is the authoritative reference for all architecture, design, and implementation decisions. All code, schemas, protocols, and deployment configs derive from this document.*

---

## 1. VISION

A self-hosted, federated, optionally-encrypted communication platform that combines Discord's UX and community features with a lightweight custom federation protocol. Runs on a Raspberry Pi 5 with 8GB RAM or scales to enterprise clusters. Every instance is a first-class citizen. No phone numbers. No face scans. No ads. No commercial forks. AGPL-3.0.

**Name:** AmityVox ("friendship voice")

---

## 2. CORE DECISIONS

| Decision | Choice | Rationale |
|---|---|---|
| Backend language | Go | Performant, scalable, single-binary deployment, excellent concurrency |
| Frontend framework | SvelteKit | Smallest bundle, fastest runtime for reactive chat UI |
| Desktop client | Tauri (Phase 2) | 5-10MB vs Electron's 150MB+, native system access |
| Database | PostgreSQL (always) | Relational data needs relational DB. No SQLite mode — one engine, portable everywhere |
| Message broker | NATS + JetStream | Real-time pub/sub + persistent streams + federation queuing in one system |
| Voice/Video | LiveKit | Open source SFU, WebRTC, screen sharing, Go-native, battle-tested |
| File storage | Garage (S3-compatible) | Lightest self-hosted S3, ~30MB RAM. Swappable to MinIO/AWS S3/Wasabi |
| Search | Meilisearch | Lightweight full-text search, easy self-hosting |
| Cache | DragonflyDB (Redis-compatible) | Drop-in Redis replacement, better performance and memory efficiency |
| Reverse proxy | Caddy | Auto-TLS, HTTP/3, simple config |
| Federation | Custom lightweight protocol (own design) | Simpler than Matrix DAG, optimized for our hardware targets |
| Matrix interop | Bridge service (optional Docker container) | Matrix rooms ↔ AmityVox channels, not protocol-level compat |
| Discord interop | Bridge service (optional Docker container) | For migration and ongoing relay |
| Encryption | Opt-in per channel/DM, MLS (RFC 9420) | Unencrypted default enables search + moderation; clear UX prompt |
| License | AGPL-3.0 | No commercial forks, all modifications must be shared |
| Config format | TOML (`amityvox.toml`) | Human-readable, self-hoster friendly |
| IDs | ULID | Sortable, embed timestamp, globally unique, federation-safe |
| Builder | AI Coding Agent (Claude Code / Opus 4.6, possibly Codex 5.3) | |

---

## 3. TARGET HARDWARE

### Memory Budget (RPi5 8GB — minimum target)

| Component | Target RAM | Notes |
|---|---|---|
| AmityVox core (Go binary) | 50-100MB | API + WebSocket + business logic |
| PostgreSQL | 200-400MB | `shared_buffers` tuned for small instance |
| NATS | 20-50MB | Message broker |
| LiveKit | 100-200MB | Voice/video (when active) |
| Garage | 30-50MB | S3-compatible file storage (lighter than MinIO) |
| Caddy | 20-30MB | Reverse proxy |
| DragonflyDB | 30-50MB | Cache/sessions |
| Meilisearch | 50-100MB | Search (optional on tiny deployments) |
| Web client | ~0MB | Static files served by Caddy |
| OS overhead | 200-300MB | Linux kernel, systemd, etc. |
| **Total** | **~700MB - 1.2GB** | **Leaves 6.8-7.3GB free on 8GB RPi5** |

### Test Deployment Matrix

| Device | CPU | RAM | Role |
|---|---|---|---|
| Raspberry Pi 5 | BCM2712 (ARM Cortex-A76 4C) | 8GB | Minimum viable deployment |
| Raspberry Pi 5 | BCM2712 (ARM Cortex-A76 4C) | 16GB | Small community (comfort margin) |
| Orange Pi 5 | RK3588S (4xA76 + 4xA55) | 16GB | Small-medium with HW transcode |
| Orange Pi 5 | RK3588S (4xA76 + 4xA55) | 32GB | Medium community |
| Orange Pi 6 Plus | RK3588 (4xA76 + 4xA55 + Mali-G610) | 32GB | Medium-large with GPU transcode |
| Minisforum MS-R1 | CIX CP8180 (ARMv9.2-A 8xA720 + 4xA520 12C, 6nm, 28W) | 64GB | Large community / multi-guild (linux/arm64) |
| Minisforum MS-A2 | AMD Ryzen 9 9955HX (Zen 5 16C/32T, 55–75W cTDP) | 96GB | Enterprise / stress testing |
| Generic enterprise | AMD EPYC or Intel Xeon (32C+ recommended) | 128GB+ | Large-scale / multi-instance federation |

### Media Transcoding

Target universal formats that work everywhere:
- **Video:** H.264 Baseline profile (universal HW decode), H.265 where supported
- **Audio:** Opus (best quality-per-bit, WebRTC native)
- **Images:** WebP primary, JPEG fallback for legacy
- **Hardware acceleration:** V4L2 (RPi5), RKMPP (Orange Pi 5/6), VAAPI (Intel N100/N150/etc.)

---

## 4. ARCHITECTURE

### 4.1 Service Topology

```
                         ┌─────────────────┐
                         │      Caddy       │
                         │  (Reverse Proxy) │
                         │  Auto-TLS, H/3   │
                         └────────┬─────────┘
                                  │
           ┌──────────────────────┼──────────────────────┐
           │                      │                      │
    ┌──────▼──────┐       ┌──────▼──────┐       ┌──────▼──────┐
    │  Web Client  │       │  REST API   │       │  WebSocket  │
    │  (SvelteKit  │       │  /api/v1/*  │       │  Gateway    │
    │   static)    │       │             │       │  /ws        │
    └──────────────┘       └──────┬──────┘       └──────┬──────┘
                                  │                      │
                           ┌──────▼──────────────────────▼──────┐
                           │         AmityVox Core (Go)         │
                           │                                     │
                           │  Packages:                          │
                           │  ├── auth          (authn/authz)   │
                           │  ├── guild         (guilds/CRUD)   │
                           │  ├── channel       (channels)      │
                           │  ├── message       (messaging)     │
                           │  ├── permission    (bitfield eval) │
                           │  ├── gateway       (WS events)     │
                           │  ├── federation    (instance sync) │
                           │  ├── media         (upload/proxy)  │
                           │  ├── encryption    (MLS/key mgmt)  │
                           │  └── admin         (instance admin)│
                           └──┬──────┬──────┬──────┬────────────┘
                              │      │      │      │
              ┌───────────────┤      │      │      ├───────────────┐
              │               │      │      │      │               │
       ┌──────▼──────┐ ┌─────▼──────▼┐ ┌──▼──────▼──┐    ┌──────▼──────┐
       │ PostgreSQL   │ │    NATS     │ │   Garage    │    │ DragonflyDB │
       │ (Data)       │ │ (Pub/Sub +  │ │ (S3 Files)  │    │ (Cache/     │
       │              │ │  JetStream) │ │             │    │  Sessions)  │
       └──────────────┘ └──────┬──────┘ └─────────────┘    └─────────────┘
                               │
                 ┌─────────────┼─────────────┐
                 │             │             │
          ┌──────▼──────┐ ┌───▼────────┐ ┌──▼───────────┐
          │   LiveKit    │ │ Meilisearch│ │ Media Worker │
          │ (Voice/Video)│ │ (Search)   │ │ (Transcode)  │
          └──────────────┘ └────────────┘ └──────────────┘

Optional Bridge Containers:
          ┌──────────────┐ ┌──────────────┐
          │ Matrix Bridge│ │Discord Bridge│
          │ (appservice) │ │ (bot-based)  │
          └──────────────┘ └──────────────┘
```

### 4.2 Single Binary, Clean Packages

The Go backend compiles to a **single binary** with subcommands:

```bash
amityvox serve          # Run the full server (API + WS + background workers)
amityvox migrate        # Run database migrations
amityvox admin          # CLI admin tools (create user, reset password, etc.)
amityvox federation     # Federation management (list peers, add/remove, status)
amityvox version        # Print version info
```

Internal package structure mirrors Stoat/Revolt's clean separation but in a monolith:

```
amityvox/
├── cmd/                    # CLI entrypoints
│   └── amityvox/
│       └── main.go
├── internal/
│   ├── config/             # TOML config parsing + defaults
│   ├── database/           # PostgreSQL connection, migrations, query builders
│   ├── models/             # Shared data types (User, Guild, Channel, Message, etc.)
│   ├── auth/               # Authentication (Argon2id, TOTP, WebAuthn, sessions)
│   ├── permissions/        # Bitfield permission system (allow/deny/calculate)
│   ├── api/                # REST API handlers (/api/v1/*)
│   │   ├── users/
│   │   ├── guilds/
│   │   ├── channels/
│   │   ├── messages/
│   │   └── admin/
│   ├── gateway/            # WebSocket gateway (events, connections, heartbeat)
│   ├── federation/         # Instance-to-instance protocol (gRPC + mTLS)
│   ├── media/              # File upload, S3 operations, transcoding dispatch
│   ├── encryption/         # MLS key management, E2E channel state
│   ├── search/             # Meilisearch integration
│   ├── presence/           # Online/offline/idle tracking via DragonflyDB
│   ├── events/             # NATS pub/sub — internal event bus
│   └── workers/            # Background jobs (embed unfurling, cleanup, transcode)
├── web/                    # SvelteKit frontend (builds to static)
├── bridges/
│   ├── matrix/             # Matrix bridge (separate binary, optional container)
│   └── discord/            # Discord bridge (separate binary, optional container)
├── migrations/             # SQL migration files
├── deploy/
│   ├── docker/
│   │   ├── Dockerfile          # Multi-stage build for core binary
│   │   └── docker-compose.yml  # Full stack deployment
│   ├── caddy/
│   │   └── Caddyfile
│   └── scripts/
│       └── install.sh          # One-line installer
├── docs/
│   ├── api/                # OpenAPI spec
│   ├── protocol/           # WebSocket gateway protocol spec
│   ├── federation/         # Federation protocol spec
│   └── admin/              # Administration guide
├── amityvox.toml           # Default configuration
├── go.mod
├── go.sum
└── LICENSE                 # AGPL-3.0
```

### 4.3 Distributed Deployment (Multi-Node)

For enterprise scale, services distribute across machines via Docker Swarm or Kubernetes:

```
Machine A (API Layer):
  - amityvox-core (replicas: N, stateless)
  - caddy (load balancer)
  - nats (cluster node 1)
  - dragonflydb (replica)

Machine B (Database):
  - postgresql (primary)
  - postgresql (streaming replica, for read scaling)

Machine C (Media):
  - garage (S3-compatible storage)
  - livekit-server
  - amityvox media-worker (transcode jobs)

Machine D (Auxiliary):
  - meilisearch
  - nats (cluster node 2)
  - matrix-bridge (optional)
  - discord-bridge (optional)
```

**Design principle:** Every service communicates via network (NATS subjects, HTTP APIs, PostgreSQL connections). Never shared filesystem or shared memory. This makes distribution across machines trivial.

NATS handles inter-node event routing. The AmityVox core binary is stateless — all state lives in PostgreSQL + NATS + DragonflyDB. Run N replicas behind Caddy's load balancer. WebSocket connections use sticky sessions (Caddy's `lb_policy cookie` or header-based affinity).

---

## 5. DATA MODEL

### 5.1 Core Entities

**Instance** — A single AmityVox deployment. Has a unique domain. Analogous to a Matrix homeserver.

**User** — Belongs to a home instance. Identified globally as `@username@instance.domain`. Authenticates against home instance. Participates in guilds on any federated instance.

**Guild** — The Discord "server" equivalent. Lives on a specific instance. Has roles, permissions, channels, categories.

**Channel** — Lives within a guild (for guild channels) or standalone (for DMs/groups).
- Types: `text`, `voice`, `dm`, `group`, `category`, `announcement`, `forum`, `stage` (later phases)

**Message** — Belongs to a channel. ULID for ID (sortable by creation time). Contains markdown content, attachments, embeds, reactions, reply references.

**Thread** — Branched conversation from a message. Child channel with parent message reference.

**Role** — Permission bundle within a guild. Allow/deny bitfield pairs. Rank-ordered.

**Member** — User's membership in a guild. Per-guild nickname, avatar, roles, timeout.

### 5.2 PostgreSQL Schema

```sql
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
```

### 5.3 Permission Bitfield Definitions

```go
// Server-scoped permissions (bits 0-19)
const (
    PermManageChannels      uint64 = 1 << 0
    PermManageGuild         uint64 = 1 << 1
    PermManagePermissions   uint64 = 1 << 2
    PermManageRoles         uint64 = 1 << 3
    PermManageEmoji         uint64 = 1 << 4
    PermManageWebhooks      uint64 = 1 << 5
    PermKickMembers         uint64 = 1 << 6
    PermBanMembers          uint64 = 1 << 7
    PermTimeoutMembers      uint64 = 1 << 8
    PermAssignRoles         uint64 = 1 << 9
    PermChangeNickname      uint64 = 1 << 10
    PermManageNicknames     uint64 = 1 << 11
    PermChangeAvatar        uint64 = 1 << 12
    PermRemoveAvatars       uint64 = 1 << 13
    PermViewAuditLog        uint64 = 1 << 14
    PermViewGuildInsights   uint64 = 1 << 15
    PermMentionEveryone     uint64 = 1 << 16
)

// Channel-scoped permissions (bits 20-39)
const (
    PermViewChannel         uint64 = 1 << 20
    PermReadHistory         uint64 = 1 << 21
    PermSendMessages        uint64 = 1 << 22
    PermManageMessages      uint64 = 1 << 23
    PermEmbedLinks          uint64 = 1 << 24
    PermUploadFiles         uint64 = 1 << 25
    PermAddReactions        uint64 = 1 << 26
    PermUseExternalEmoji    uint64 = 1 << 27
    PermConnect             uint64 = 1 << 28  // Voice
    PermSpeak               uint64 = 1 << 29  // Voice
    PermMuteMembers         uint64 = 1 << 30  // Voice
    PermDeafenMembers       uint64 = 1 << 31  // Voice
    PermMoveMembers         uint64 = 1 << 32  // Voice
    PermUseVAD              uint64 = 1 << 33  // Voice Activity Detection
    PermPrioritySpeaker     uint64 = 1 << 34  // Voice
    PermStream              uint64 = 1 << 35  // Screen share
    PermMasquerade          uint64 = 1 << 36  // Override name/avatar (bridges)
    PermCreateInvites       uint64 = 1 << 37
    PermManageThreads       uint64 = 1 << 38
    PermCreateThreads       uint64 = 1 << 39
)

// Administrator (bit 63) — bypasses all permission checks
const PermAdministrator uint64 = 1 << 63
```

### 5.4 Permission Resolution Algorithm

```
func CalculatePermissions(member, guild, channel) uint64:
    // Guild owner always has everything
    if member.user_id == guild.owner_id:
        return ALL_PERMISSIONS

    // 1. Start with @everyone base permissions
    perms = guild.default_permissions

    // 2. Apply role permissions (sorted by position, ascending = highest priority last)
    for role in member.roles.sorted_by(position DESC):  // lowest position = highest priority
        perms |= role.permissions_allow
        perms &^= role.permissions_deny

    // 3. Administrator bypasses everything
    if perms & PermAdministrator != 0:
        return ALL_PERMISSIONS

    // 4. Apply channel-level @everyone overrides
    if channel.default_permissions != nil:
        perms |= channel.default_permissions_allow
        perms &^= channel.default_permissions_deny

    // 5. Apply channel-level role overrides
    for role in member.roles:
        if override = channel.overrides[role.id]:
            perms |= override.permissions_allow
            perms &^= override.permissions_deny

    // 6. Apply channel-level user overrides
    if override = channel.overrides[member.user_id]:
        perms |= override.permissions_allow
        perms &^= override.permissions_deny

    // 7. Timeout strips action permissions
    if member.timeout_until != nil && member.timeout_until > now():
        action_mask = PermSendMessages | PermAddReactions | PermConnect |
                      PermSpeak | PermStream | PermCreateThreads |
                      PermCreateInvites
        perms &^= action_mask

    // 8. Can't do anything in a channel you can't see
    if perms & PermViewChannel == 0:
        return 0

    return perms
```

---

## 6. ENCRYPTION MODEL

### 6.1 User-Facing Behavior

**Default state:** All text channels and DMs are **unencrypted** by default.

**Enabling encryption:**
- DMs: Either party can enable encryption. The other party sees: *"[User] has enabled end-to-end encryption for this conversation. Messages from this point forward are encrypted."*
- Guild channels: Guild admin/owner enables per-channel. Existing messages remain unencrypted; new messages are encrypted.
- Visual indicator: Lock icon next to channel name. Green = encrypted active. Grey = unencrypted.
- New DM prompt: *"This is a new conversation. Messages are not end-to-end encrypted. [Enable Encryption]"*

**What encryption affects:**
- Encrypted messages can't be server-side searched (client-side search only)
- Encrypted channels can't use server-side moderation (AutoMod can't read content)
- Embeds/link previews don't work (server can't see URLs to unfurl)
- File attachments are encrypted at rest (client encrypts before upload)

### 6.2 Technical Implementation (MLS — RFC 9420)

- Each user has a long-lived identity key (Ed25519) stored locally
- MLS groups are created per-channel/per-DM when encryption is enabled
- Key packages are uploaded to the server (public key bundles, not secret keys)
- Group state uses a ratchet tree for O(log n) member operations
- Server acts as MLS Delivery Service (routes encrypted messages, stores key packages)
- Server never sees plaintext or private keys

---

## 7. FEDERATION PROTOCOL (AmityVox Native)

### 7.1 Design Principles

- Lighter than Matrix's DAG model
- HTTP-based with signed payloads (not gRPC — simpler for debugging, firewall-friendly)
- Each instance has an Ed25519 signing keypair
- All federated payloads are JSON, signed with the sender instance's private key
- Receiving instance verifies signature using sender's public key (discovered via `.well-known`)

### 7.2 Instance Discovery

```
GET https://chat.example.com/.well-known/amityvox
{
    "instance_id": "01HXYZ...",
    "domain": "chat.example.com",
    "name": "Example Community",
    "public_key": "-----BEGIN PUBLIC KEY-----\n...\n-----END PUBLIC KEY-----",
    "software": "amityvox",
    "software_version": "0.1.0",
    "federation_mode": "open",
    "api_endpoint": "https://chat.example.com/federation/v1",
    "supported_protocols": ["amityvox-federation/1.0"]
}
```

### 7.3 Federation Modes

- **Open:** Accepts federation from any instance (like email)
- **Allowlist:** Only federates with approved instances
- **Closed:** No federation (corporate/family use case)

### 7.4 Message Ordering

Hybrid Logical Clocks (HLC) — combines wall clock time with logical counters for causal ordering without a central coordinator. Simpler than Matrix's DAG.

### 7.5 Offline Federation

Messages queue in NATS JetStream when target instance is unreachable. Replayed when connection restores. Configurable retention (default: 7 days).

---

## 8. WEBSOCKET GATEWAY PROTOCOL

### 8.1 Connection Flow

```
Client                              Server
  |                                    |
  |--- WS Connect (/ws) ------------->|
  |                                    |
  |<-- op:10 HELLO {heartbeat_ms} ----|
  |                                    |
  |--- op:2 IDENTIFY {token} -------->|
  |                                    |
  |<-- op:0 DISPATCH t:READY          |
  |     {user, guilds, channels,       |
  |      members, read_states,         |
  |      presences} ------------------|
  |                                    |
  |--- op:1 HEARTBEAT {seq} --------->|
  |<-- op:11 HEARTBEAT_ACK -----------|
  |                                    |
  |<-- op:0 DISPATCH t:* events ------|
```

### 8.2 Opcodes

| Opcode | Name | Direction | Description |
|---|---|---|---|
| 0 | DISPATCH | Server→Client | Event dispatch (has `t` type and `d` data) |
| 1 | HEARTBEAT | Client→Server | Keepalive |
| 2 | IDENTIFY | Client→Server | Auth with token |
| 3 | PRESENCE_UPDATE | Client→Server | Update own presence |
| 4 | VOICE_STATE_UPDATE | Client→Server | Join/leave/mute voice |
| 5 | RESUME | Client→Server | Resume dropped connection |
| 6 | RECONNECT | Server→Client | Server wants client to reconnect |
| 7 | REQUEST_MEMBERS | Client→Server | Request guild member list |
| 8 | TYPING | Client→Server | Typing indicator |
| 9 | SUBSCRIBE | Client→Server | Subscribe to presence for guild |
| 10 | HELLO | Server→Client | First message after connect |
| 11 | HEARTBEAT_ACK | Server→Client | Heartbeat response |

### 8.3 Event Types (t field in op:0 DISPATCH)

**Messages:** MESSAGE_CREATE, MESSAGE_UPDATE, MESSAGE_DELETE, MESSAGE_DELETE_BULK, MESSAGE_REACTION_ADD, MESSAGE_REACTION_REMOVE, MESSAGE_REACTION_CLEAR, MESSAGE_EMBED_UPDATE (async embed unfurl)

**Channels:** CHANNEL_CREATE, CHANNEL_UPDATE, CHANNEL_DELETE, CHANNEL_PINS_UPDATE, TYPING_START

**Guilds:** GUILD_CREATE (full state on join/connect), GUILD_UPDATE, GUILD_DELETE, GUILD_MEMBER_ADD, GUILD_MEMBER_UPDATE, GUILD_MEMBER_REMOVE, GUILD_ROLE_CREATE, GUILD_ROLE_UPDATE, GUILD_ROLE_DELETE, GUILD_BAN_ADD, GUILD_BAN_REMOVE, GUILD_EMOJI_UPDATE

**Users:** PRESENCE_UPDATE, USER_UPDATE

**Voice:** VOICE_STATE_UPDATE, VOICE_SERVER_UPDATE (LiveKit connection info)

**Read State:** CHANNEL_ACK (read position update)

### 8.4 Partial Update Pattern

Updates send only changed fields plus a `_clear` array for removed fields:
```json
{
    "op": 0,
    "t": "CHANNEL_UPDATE",
    "d": {
        "id": "01HXY...",
        "name": "new-name",
        "_clear": ["topic", "icon_id"]
    }
}
```

---

## 9. REST API STRUCTURE

All routes prefixed with `/api/v1/`. Authentication via `Authorization: Bearer {session_token}` header.

### 9.1 Endpoint Summary

**Auth:** POST /auth/register, POST /auth/login, POST /auth/logout, POST /auth/totp/enable, POST /auth/totp/verify, POST /auth/webauthn/register, POST /auth/webauthn/verify

**Users:** GET /users/@me, PATCH /users/@me, GET /users/{id}, GET /users/@me/guilds, GET /users/@me/dms, POST /users/{id}/dm, PUT /users/{id}/friend, DELETE /users/{id}/friend, PUT /users/{id}/block, DELETE /users/{id}/block

**Guilds:** POST /guilds, GET /guilds/{id}, PATCH /guilds/{id}, DELETE /guilds/{id}, GET /guilds/{id}/channels, POST /guilds/{id}/channels, GET /guilds/{id}/members, GET /guilds/{id}/members/{id}, PATCH /guilds/{id}/members/{id}, DELETE /guilds/{id}/members/{id}, GET /guilds/{id}/bans, PUT /guilds/{id}/bans/{id}, DELETE /guilds/{id}/bans/{id}, GET /guilds/{id}/roles, POST /guilds/{id}/roles, PATCH /guilds/{id}/roles/{id}, DELETE /guilds/{id}/roles/{id}, GET /guilds/{id}/invites, GET /guilds/{id}/audit-log, GET /guilds/{id}/emoji, POST /guilds/{id}/emoji

**Channels:** GET /channels/{id}, PATCH /channels/{id}, DELETE /channels/{id}, GET /channels/{id}/messages, POST /channels/{id}/messages, GET /channels/{id}/messages/{id}, PATCH /channels/{id}/messages/{id}, DELETE /channels/{id}/messages/{id}, PUT /channels/{id}/messages/{id}/reactions/{emoji}, DELETE /channels/{id}/messages/{id}/reactions/{emoji}, GET /channels/{id}/pins, PUT /channels/{id}/pins/{id}, DELETE /channels/{id}/pins/{id}, POST /channels/{id}/typing, POST /channels/{id}/ack, PUT /channels/{id}/permissions/{id}, DELETE /channels/{id}/permissions/{id}

**Message Pagination:** `?before={id}&limit=50`, `?after={id}&limit=50`, `?around={id}&limit=50`

**Invites:** GET /invites/{code}, POST /invites/{code}, DELETE /invites/{code}

**Webhooks:** POST /webhooks/{id}/{token} (execute webhook — no auth required, token is the secret)

**Admin:** GET /admin/instance, PATCH /admin/instance, GET /admin/federation/peers, POST /admin/federation/peers, DELETE /admin/federation/peers/{id}, GET /admin/stats

**File Upload:** POST /files/upload (multipart form, returns attachment object with S3 reference)

---

## 10. BRIDGE ARCHITECTURE

### 10.1 Matrix Bridge

Runs as a separate Docker container. Implements the Matrix Application Service API:
- Registers as an appservice on a Matrix homeserver (Conduit, Dendrite, or Synapse)
- Maps AmityVox channels ↔ Matrix rooms bidirectionally
- Translates message formats (markdown ↔ Matrix event format)
- Bridges user presence and typing indicators
- Uses masquerade on the AmityVox side (bridged Matrix users show their Matrix name/avatar)
- Uses virtual Matrix users on the Matrix side (bridged AmityVox users get `@amityvox_username:matrix.server`)

### 10.2 Discord Bridge

Runs as a separate Docker container. Uses Discord's bot API:
- A Discord bot token connects to Discord gateway
- Maps AmityVox channels ↔ Discord channels
- Relays messages bidirectionally using webhooks for display name/avatar fidelity
- Useful for migration: run both simultaneously during transition

### 10.3 Bridge Bot API Access

Bridges connect to AmityVox via the standard Bot API (REST + WebSocket). No special internal APIs needed. They're just bots with the masquerade permission.

---

## 11. MVP SCOPE (v0.1.0)

### What Ships First

1. **Core server** — Single Go binary, Docker Compose deployment
2. **User auth** — Registration, login, sessions, TOTP 2FA
3. **Guilds** — Create, join, leave, edit, delete, categories
4. **Channels** — Text channels, DMs, group DMs
5. **Messaging** — Send, edit, delete, reply, reactions, markdown, embeds (async unfurl)
6. **Permissions** — Full bitfield system, roles, channel overrides
7. **File uploads** — S3-based, image thumbnails, basic type detection
8. **WebSocket gateway** — Real-time events, typing indicators, presence
9. **Voice chat** — LiveKit integration, basic voice channels
10. **Web client** — SvelteKit app (guild list, channel list, messages, member list, settings)
11. **Admin dashboard** — Instance settings, user management, basic stats
12. **Docker Compose** — One-command deployment with all services

### What's NOT in v0.1.0

- Federation (v0.2.0)
- E2E encryption (v0.2.0 — protocol designed from day one, implementation second)
- Matrix/Discord bridges (v0.3.0)
- Video/screen sharing (v0.2.0 — LiveKit already supports it, needs client UI)
- Forum/Stage channels (v0.3.0)
- Threads (v0.2.0)
- Plugin/WASM system (v0.4.0+)
- Mobile clients (v0.3.0+)
- Desktop client / Tauri (v0.3.0+)
- Search / Meilisearch (v0.2.0)
- Bot API (v0.2.0 — internal events system designed for it from day one)

### MVP Priority Order for Implementation

```
Phase 1: Foundation (Weeks 1-3)
├── Go project scaffold + build system
├── PostgreSQL schema + migrations
├── Config loading (amityvox.toml)
├── NATS connection
├── DragonflyDB connection
├── User registration + login + sessions
└── Basic REST API skeleton

Phase 2: Core Features (Weeks 4-8)
├── Guild CRUD + membership
├── Channel CRUD + categories
├── Message send/receive/edit/delete (REST)
├── WebSocket gateway + event dispatch
├── Permission system (full bitfield implementation)
├── File upload (S3/Garage integration)
└── Typing indicators + presence

Phase 3: Rich Features (Weeks 9-12)
├── Reactions + embeds (async unfurl)
├── Roles + channel permission overrides
├── DMs + group DMs
├── Invites
├── Read state / unread tracking
├── Audit log
├── Voice channels (LiveKit integration)
└── Webhooks

Phase 4: Frontend + Polish (Weeks 13-18)
├── SvelteKit web client (full UI)
├── Admin dashboard
├── Docker Compose orchestration
├── Caddyfile configuration
├── Documentation
├── RPi5 / ARM64 testing + optimization
└── PostgreSQL tuning for small instances

Phase 5: v0.1.0 Release
├── Multi-arch Docker images (amd64, arm64)
├── install.sh one-liner
├── Release notes + migration guide
└── GitHub release
```

---

## 12. CONFIGURATION

### amityvox.toml (Default)

```toml
[instance]
domain = "localhost"
name = "AmityVox"
description = ""
federation_mode = "closed"  # "open", "allowlist", "closed"

[database]
url = "postgres://amityvox:amityvox@localhost:5432/amityvox?sslmode=disable"
max_connections = 25
# For RPi5: max_connections = 10

[nats]
url = "nats://localhost:4222"

[cache]
url = "redis://localhost:6379"  # DragonflyDB is Redis-compatible

[storage]
type = "s3"
endpoint = "http://localhost:3900"   # Garage S3 API port (MinIO uses 9000)
bucket = "amityvox"
access_key = ""                       # Set from Garage admin CLI
secret_key = ""                       # Set from Garage admin CLI
region = "garage"
# Compatible with: Garage (default), MinIO, AWS S3, Wasabi, Backblaze B2

[livekit]
url = "ws://localhost:7880"
api_key = ""
api_secret = ""

[search]
enabled = true
url = "http://localhost:7700"
api_key = ""

[auth]
session_duration = "720h"  # 30 days
registration_enabled = true
invite_only = false
require_email = false

[media]
max_upload_size = "100MB"
image_thumbnail_sizes = [128, 256, 512]
transcode_video = true
strip_exif = true

[http]
listen = "0.0.0.0:8080"
cors_origins = ["*"]

[websocket]
listen = "0.0.0.0:8081"
heartbeat_interval = "30s"
heartbeat_timeout = "90s"

[logging]
level = "info"  # debug, info, warn, error
format = "json"

[metrics]
enabled = true
listen = "0.0.0.0:9090"  # Prometheus endpoint
```

---

*This document is the single source of truth for AmityVox. All implementation work derives from here. Update this document before making significant architectural changes.*

*Next steps: Set up the Git repository, scaffold the Go project, and begin Phase 1 implementation.*
