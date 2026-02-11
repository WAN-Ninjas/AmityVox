# AmityVox

A self-hosted, federated, optionally-encrypted communication platform. Think Discord, but open source, federated, and designed to run on a Raspberry Pi.

**License:** AGPL-3.0

## Features

**Core**
- Guilds (servers) with channels, categories, roles, and permissions
- Real-time messaging with replies, reactions, pins, threads, and markdown
- WebSocket gateway with typing indicators and presence tracking
- DMs and group DMs
- File uploads with image thumbnails, blurhash previews, and EXIF stripping
- Full-text search via Meilisearch
- Webhooks with external execution
- Audit log for all administrative actions
- Rate limiting and per-guild AutoMod (word/regex/invite/spam/link filters)

**Authentication**
- Argon2id password hashing
- TOTP two-factor authentication with backup codes
- WebAuthn/FIDO2 hardware key support
- Session management with device listing and revocation

**Voice & Video**
- LiveKit-powered voice channels with WebRTC
- Token-based room join with voice state tracking

**Federation**
- Instance-to-instance communication with Ed25519-signed messages
- Hybrid Logical Clock ordering for consistency
- Configurable modes: open, allowlist, or closed
- Offline queuing with exponential backoff retry

**Encryption**
- MLS (Messaging Layer Security, RFC 9420) key management
- Per-channel encryption with key packages, welcome messages, and group state
- Server-side delivery service — client-side encryption

**Push Notifications**
- WebPush with VAPID authentication
- Per-guild notification preferences (all, mentions only, none)
- Recipient resolution for DMs, mentions, role mentions, and @everyone

**Bridges**
- Matrix bridge (Application Service API, virtual user impersonation)
- Discord bridge (bot gateway, webhook relay, channel mapping)

**Frontend**
- SvelteKit 5 web client with TailwindCSS
- Discord-like three-panel layout
- Builds to 195KB static bundle served by Caddy

## Tech Stack

| Component | Technology | Purpose |
|---|---|---|
| Backend | Go 1.23+ | Single binary, subcommands: serve, migrate, admin, version |
| Frontend | SvelteKit 5 | Static SPA served by Caddy |
| Database | PostgreSQL 16 | Primary data store (always — no SQLite) |
| Message Broker | NATS + JetStream | Real-time events, persistent streams, job queues |
| Cache | DragonflyDB | Sessions, presence, rate limiting (Redis-compatible) |
| Voice/Video | LiveKit | WebRTC SFU |
| File Storage | Garage | S3-compatible (swappable to MinIO/AWS S3/Wasabi) |
| Search | Meilisearch | Full-text message and user search |
| Reverse Proxy | Caddy | Auto-TLS, static file serving, WebSocket proxy |

## Quick Start

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/) and [Docker Compose](https://docs.docker.com/compose/install/) v2+
- 2GB+ RAM (runs comfortably on Raspberry Pi 5 with 8GB)

### 1. Clone and start

```bash
git clone https://github.com/WAN-Ninjas/AmityVox.git
cd AmityVox
cp .env.example .env        # Review and edit settings
make docker-up               # Start all services
```

### 2. Create an admin user

```bash
docker exec amityvox amityvox admin create-user \
  --username admin \
  --email admin@example.com \
  --password 'YourSecurePassword'

docker exec amityvox amityvox admin set-admin --username admin
```

### 3. Open the web client

Navigate to `http://localhost` in your browser. Log in with the admin credentials.

### Stopping

```bash
make docker-down
```

## Local Development

### Requirements

- Go 1.23+
- Node.js 20+
- PostgreSQL 16
- NATS 2.x
- DragonflyDB or Redis 7+

### Backend

```bash
# Install Go dependencies
make deps

# Copy and edit config
cp amityvox.example.toml amityvox.toml

# Run database migrations
make migrate-up

# Build and run
make run
```

### Frontend

```bash
cd web
npm install
npm run dev    # Dev server with API proxy to localhost:8080
```

### Testing

```bash
# Run all Go tests (unit + integration if Docker available)
make test

# Run with coverage report
make test-cover

# Type-check frontend
cd web && npm run check
```

## Docker Compose Services

The full stack runs 8 containers:

| Service | Image | Ports | Purpose |
|---|---|---|---|
| amityvox | Built from source | 8080, 8081 | REST API + WebSocket |
| postgresql | postgres:16-alpine | 5432 | Database |
| nats | nats:2-alpine | 4222, 8222 | Message broker |
| dragonflydb | dragonflydb/dragonfly | 6379 | Cache/sessions |
| garage | dxflrs/garage:v1.0 | 3900, 3902 | S3 file storage |
| livekit | livekit/livekit-server | 7880, 7881, 7882/udp | Voice/video |
| meilisearch | meilisearch:v1.7 | 7700 | Search |
| caddy | caddy:2-alpine | 80, 443 | Reverse proxy + TLS |

Total memory: ~700MB–1.2GB (leaves 6.8–7.3GB free on 8GB RPi5).

## Configuration

AmityVox is configured via `amityvox.toml` with environment variable overrides (prefixed `AMITYVOX_`). See [`amityvox.example.toml`](amityvox.example.toml) for all options.

Key environment variables for Docker:

```bash
AMITYVOX_DATABASE_URL=postgres://user:pass@host:5432/db
AMITYVOX_NATS_URL=nats://host:4222
AMITYVOX_CACHE_URL=redis://host:6379
AMITYVOX_STORAGE_ENDPOINT=http://host:3900
AMITYVOX_STORAGE_ACCESS_KEY=your-key
AMITYVOX_STORAGE_SECRET_KEY=your-secret
AMITYVOX_LIVEKIT_URL=ws://host:7880
AMITYVOX_LIVEKIT_API_KEY=devkey
AMITYVOX_LIVEKIT_API_SECRET=secret
AMITYVOX_SEARCH_URL=http://host:7700
AMITYVOX_INSTANCE_DOMAIN=chat.example.com
```

### Production with TLS

Edit `deploy/caddy/Caddyfile` — replace `:80` with your domain:

```
chat.example.com {
    # ... same route blocks ...
}
```

Caddy automatically provisions Let's Encrypt certificates.

## CLI Commands

```bash
amityvox serve                          # Start the server
amityvox migrate up                     # Run pending migrations
amityvox migrate down                   # Rollback last migration
amityvox migrate status                 # Show migration status
amityvox admin create-user              # Create a user
amityvox admin suspend --username X     # Suspend a user
amityvox admin unsuspend --username X   # Unsuspend a user
amityvox admin set-admin --username X   # Grant admin flag
amityvox admin unset-admin --username X # Revoke admin flag
amityvox admin list-users               # List all users
amityvox version                        # Print version info
```

## Project Structure

```
amityvox/
├── cmd/amityvox/           # CLI entrypoint (serve, migrate, admin, version)
├── internal/
│   ├── api/                # REST API handlers (/api/v1/*)
│   ├── auth/               # Authentication (Argon2id, TOTP, WebAuthn, sessions)
│   ├── automod/            # AutoMod engine (filters, rules, actions)
│   ├── config/             # TOML config parsing + env overrides
│   ├── database/           # PostgreSQL connection, embedded migrations
│   ├── encryption/         # MLS key management
│   ├── events/             # NATS pub/sub event bus
│   ├── federation/         # Instance-to-instance protocol
│   ├── gateway/            # WebSocket gateway (identify, heartbeat, events)
│   ├── integration/        # Integration tests (dockertest)
│   ├── media/              # File upload, S3, thumbnails, blurhash
│   ├── models/             # Shared data types
│   ├── notifications/      # WebPush notification service
│   ├── permissions/        # Bitfield permission system
│   ├── presence/           # Online/offline tracking via DragonflyDB
│   ├── search/             # Meilisearch integration
│   ├── voice/              # LiveKit voice integration
│   └── workers/            # Background jobs (transcode, unfurl, automod, push)
├── web/                    # SvelteKit frontend
├── bridges/
│   ├── matrix/             # Matrix bridge (standalone binary)
│   └── discord/            # Discord bridge (standalone binary)
├── deploy/
│   ├── docker/             # Dockerfile + docker-compose.yml
│   └── caddy/              # Caddyfile
├── docs/
│   └── architecture.md     # Master architecture specification
├── amityvox.example.toml   # Configuration template
├── Makefile                # Build, test, deploy targets
└── TODO.md                 # Development roadmap
```

## API Overview

All endpoints under `/api/v1/`. Authentication via `Authorization: Bearer <token>`.

Response format:
```json
// Success
{ "data": { ... } }

// Error
{ "error": { "code": "not_found", "message": "Guild not found" } }
```

Major endpoint groups:
- `POST /auth/register`, `POST /auth/login` — Authentication
- `GET/PATCH /users/@me` — Current user
- `POST /guilds`, `GET /guilds/{id}` — Guild management
- `GET /guilds/{id}/channels`, `POST /guilds/{id}/channels` — Channels
- `GET/POST /channels/{id}/messages` — Messaging
- `GET /channels/{id}/messages/{id}/reactions` — Reactions
- `POST /voice/{channelId}/join` — Voice channels
- `/ws` — WebSocket gateway

See [`docs/architecture.md`](docs/architecture.md) for the complete API reference.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, coding standards, and contribution guidelines.

## License

[GNU Affero General Public License v3.0](LICENSE)
