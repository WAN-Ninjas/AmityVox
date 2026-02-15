# AmityVox

A self-hosted, federated, optionally-encrypted communication platform. Think Discord, but open source, federated, and designed to run on a Raspberry Pi.

**v0.5.0** | [AGPL-3.0](LICENSE) | [Discord](https://discord.gg/VvxgUpF3uQ) | [Live Instance](https://amityvox.chat/invite/b38a4701f16f)

## Features

- Guilds with channels, categories, roles, and granular permissions
- Real-time messaging with replies, reactions, threads, pins, and markdown
- DMs and group DMs with typing indicators and read receipts
- Voice and video channels powered by LiveKit (WebRTC)
- File uploads with image thumbnails, blurhash previews, and EXIF stripping
- Full-text search via Meilisearch
- MLS end-to-end encryption (RFC 9420) for channels
- Federation between instances with Ed25519-signed messages
- TOTP and WebAuthn/FIDO2 two-factor authentication
- WebPush notifications with per-guild preferences
- Bridges to Matrix, Discord, Telegram, Slack, and IRC
- Bot SDK for building custom integrations
- Webhooks, audit logs, and AutoMod (word/regex/spam/link filters)
- Admin dashboard with user management and instance settings
- Emoji picker, Giphy integration, and keyboard shortcuts
- Self-hosted translation via LibreTranslate
- Prometheus metrics endpoint
- ~16 MB Docker image, runs on 2 GB RAM

## Quick Start

### Prerequisites

- [Docker Engine](https://docs.docker.com/engine/install/) and Docker Compose v2+
- A domain name (for TLS) or `localhost` for local testing
- 2 GB+ RAM (runs comfortably on Raspberry Pi 5)

### Option A: Deploy without cloning

Download the compose file and environment template, then start:

```bash
mkdir amityvox && cd amityvox
curl -O https://raw.githubusercontent.com/WAN-Ninjas/AmityVox/main/docker_deploy/docker-compose.yml
curl -O https://raw.githubusercontent.com/WAN-Ninjas/AmityVox/main/docker_deploy/.env.example
cp .env.example .env
# Edit .env — set AMITYVOX_INSTANCE_DOMAIN, POSTGRES_PASSWORD, etc.
docker compose up -d
```

### Option B: Clone and start

```bash
git clone https://github.com/WAN-Ninjas/AmityVox.git
cd AmityVox/docker_deploy
cp .env.example .env
# Edit .env — set AMITYVOX_INSTANCE_DOMAIN, POSTGRES_PASSWORD, etc.
docker compose up -d
```

## Initial Setup

### 1. Setup Wizard

On first launch, open your domain (or `http://localhost`) in a browser. The setup wizard runs automatically and lets you configure:

- Instance name and description
- Domain and registration mode
- Federation mode (open, allowlist, or closed)

The wizard locks after completion. Further changes require admin authentication.

### 2. Create an Admin User

```bash
docker exec amityvox amityvox admin create-user admin admin@example.com YourSecurePassword
docker exec amityvox amityvox admin set-admin admin
```

### 3. Log In

Open your domain in the browser, register or log in with the admin account, and start using AmityVox.

## Configuration

All settings are controlled via environment variables in `.env`. Key variables:

| Variable | Default | Purpose |
|---|---|---|
| `AMITYVOX_INSTANCE_DOMAIN` | `localhost` | Public domain (used for TLS, WebAuthn, federation) |
| `AMITYVOX_INSTANCE_NAME` | `AmityVox` | Display name shown in the UI |
| `POSTGRES_PASSWORD` | `amityvox` | Database password (**change for production**) |
| `LIVEKIT_API_KEY` | `devkey` | LiveKit auth key (**change for production**) |
| `LIVEKIT_API_SECRET` | `secret` | LiveKit auth secret (**change for production**) |
| `MEILI_MASTER_KEY` | *(empty)* | Meilisearch API key (set for production) |
| `AMITYVOX_STORAGE_ACCESS_KEY` | *(empty)* | S3 access key (see Garage setup below) |
| `AMITYVOX_STORAGE_SECRET_KEY` | *(empty)* | S3 secret key (see Garage setup below) |

See [`docker_deploy/.env.example`](docker_deploy/.env.example) for all available options including push notifications, Giphy, translation languages, and media settings.

### TLS / Custom Domain

Set `AMITYVOX_INSTANCE_DOMAIN` to your domain in `.env`. Caddy automatically provisions Let's Encrypt certificates — no manual TLS configuration needed. Ensure ports 80 and 443 are open.

### Garage S3 Setup

After first boot, create the storage bucket and key:

```bash
# Get the node ID
docker exec amityvox-garage /garage status

# Assign and apply layout (replace NODE_ID)
docker exec amityvox-garage /garage layout assign -z dc1 -c 1G NODE_ID
docker exec amityvox-garage /garage layout apply --version 1

# Create bucket and key
docker exec amityvox-garage /garage bucket create amityvox
docker exec amityvox-garage /garage key create amityvox-key

# Allow key access to bucket
docker exec amityvox-garage /garage bucket allow amityvox --read --write --key amityvox-key

# Get the key ID and secret
docker exec amityvox-garage /garage key info amityvox-key
```

Copy the key ID and secret into `AMITYVOX_STORAGE_ACCESS_KEY` and `AMITYVOX_STORAGE_SECRET_KEY` in `.env`, then restart:

```bash
docker compose restart amityvox
```

## Services

The full stack runs 9 containers:

| Service | Image | Purpose |
|---|---|---|
| `amityvox` | `ghcr.io/wan-ninjas/amityvox` | REST API + WebSocket gateway |
| `postgresql` | `postgres:16-alpine` | Primary database |
| `nats` | `nats:2-alpine` | Message broker (JetStream) |
| `dragonflydb` | `dragonflydb/dragonfly` | Cache and sessions |
| `garage` | `dxflrs/garage:v1.0.1` | S3-compatible file storage |
| `livekit` | `livekit/livekit-server` | Voice/video (WebRTC) |
| `meilisearch` | `getmeili/meilisearch:v1.35` | Full-text search |
| `libretranslate` | `libretranslate/libretranslate` | Message translation |
| `caddy` | `caddy:2-alpine` | Reverse proxy + auto-TLS |

Total memory: ~700 MB - 1.2 GB.

## CLI Reference

```bash
docker exec amityvox amityvox <command>
```

| Command | Purpose |
|---|---|
| `admin create-user <user> <email> <pass>` | Create a user |
| `admin set-admin <user>` | Grant admin privileges |
| `admin unset-admin <user>` | Revoke admin privileges |
| `admin suspend <user>` | Suspend a user |
| `admin unsuspend <user>` | Unsuspend a user |
| `admin list-users` | List all users |
| `migrate up` | Run pending database migrations |
| `migrate down` | Rollback last migration |
| `migrate status` | Show migration status |
| `version` | Print version info |

## Backup & Restore

```bash
# Backup all data (database + volumes)
docker exec amityvox-postgresql pg_dumpall -U amityvox > backup.sql

# Restore
cat backup.sql | docker exec -i amityvox-postgresql psql -U amityvox
```

## Updating

```bash
docker compose pull        # Pull latest images
docker compose up -d       # Recreate containers
```

Migrations run automatically on startup.

## Community

- [Discord](https://discord.gg/VvxgUpF3uQ) — Support, feedback, and announcements
- [Live Instance](https://amityvox.chat/invite/b38a4701f16f) — Try AmityVox without installing
- [GitHub Issues](https://github.com/WAN-Ninjas/AmityVox/issues) — Bug reports and feature requests

## License

[GNU Affero General Public License v3.0](LICENSE)
