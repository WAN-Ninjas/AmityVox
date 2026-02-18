# AmityVox

A self-hosted, federated, optionally-encrypted communication platform. Think Discord, but open source, federated, and designed to run on a Raspberry Pi.

**v0.5.0** | [AGPL-3.0](LICENSE) | [Discord](https://discord.gg/VvxgUpF3uQ) | [Live Instance](https://amityvox.chat/)

I'm a 27 year IT veteran, most of that in senior/management DevOps and Network Ops/Engineering roles. This software was written with Codex and Claude Code. The architecture and spec behind it is a 31 page document written by myself and my friends — no AI involvement beyond making our spec more presentable. If AI written code offends you, move along. If a project that isn't finished yet offends you, come back later.

This is being worked on daily by myself and a small team of friends who share the same vision: take all the things we love about Discord, Matrix, and other platforms we've used or tried, and build one cohesive self-hostable, AGPL-3.0 licensed project for ourselves and our communities. We've decided to make it available to everyone so that when we're done, people have a powerful turnkey alternative to Discord that will NEVER be monetized. We'll maintain it as long as we're using it. If that ever changes, we intend the code to be well documented — full spec documents will be published once we finish the current dev phase.

Right now, the code is a mess. It's getting better through a lot of manual review and changes as it evolves. It is functional — the code here is what's running at amityvox.chat, sometimes off by a few hours if we're working on new features.

### What AmityVox Is (and Isn't)

AmityVox is not a Matrix replacement. I've been asked several times how certain features work or will work, so let me clarify:

We'll have some messaging encryption, but this is not privacy-first like Matrix. Federation gives people control over their own instances while keeping global communication between instances possible. What gets logged is ultimately not under our control outside our own instance — even if we disable logging, anyone running the code could trivially re-enable it. Instead, we've focused on making features **optional**: email validation for registration, federation (disabled by default), encryption, etc.

**Encryption model:** The party initiating encryption (DM, group DM, or guild channel) generates a key client-side that is NOT sent to the server backend. If the key or passphrase is lost, there's no recovery. Without it, instance owners (assuming unmodified code) can't decrypt stored messages or media. Users joining an encrypted channel will need to receive the key out-of-band. We have ideas for making this more user-friendly, but it's not a priority right now. Anyone with experience in encrypted chat is welcome to give us pointers.

### Federation Modes

We plan to host a **Master/Public** federation server (joining is optional). Beyond that, three modes:

- **Open** — Private, but anyone who knows your server can add it to their federation list and interact with your instance.
- **Closed** — Requires exchanging keys and whitelisting on both sides.
- **Disabled** — Completely standalone, no federation.

When federated with another server, you can: join guilds on other instances, DM (1:1 and group) across instances, and voice/video/screenshare across instances (requires LiveKit on all sides).

### Roadmap & Apps

Within a week or so of writing this (2/18/26), the front end will be locked in as stable. At that point, one of my friends will be getting Tauri going for Windows, macOS, Linux, Android, and iOS apps, plus handling app store registration for Google and Apple. The apps will be instance-agnostic — on launch you pick which instance to connect to.

We're also considering hosting a dynamic DNS service for instance hosts. It's not strictly needed (plenty of free options exist), but we've had requests for it.

### Feedback

We welcome any and all feedback — preferably via [GitHub Issues](https://github.com/WAN-Ninjas/AmityVox/issues) or the Report Issue button on [amityvox.chat](https://amityvox.chat/). Or just come chat with us there — I'm @Horatio.

---

*Everything below is mostly AI-written. It has good info but may not be 100% accurate as of this update — I'll clean it up before launch.*

---

## Features

*Not all implemented yet. All planned for completion by end of 02/2026.*

- Guilds with channels, categories, roles, and granular permissions
- Real-time messaging with replies, reactions, threads, pins, and markdown
- Message expiration (configurable per DM, guild, or instance)
- DMs and group DMs with typing indicators and read receipts
- Voice and video channels via LiveKit (WebRTC)
- File uploads with image thumbnails, blurhash previews, and EXIF stripping
- Full-text search via Meilisearch
- MLS end-to-end encryption (RFC 9420)
- Federation with Ed25519-signed messages
- TOTP and WebAuthn/FIDO2 two-factor auth
- WebPush notifications with per-guild preferences
- Bridges to Matrix, Discord, Telegram, Slack, and IRC
- Bot SDK and webhooks
- Audit logs and AutoMod (word/regex/spam/link filters)
- Admin dashboard with user management and instance settings
- Emoji picker, Giphy integration, keyboard shortcuts
- Self-hosted translation via LibreTranslate

## Quick Start

### Prerequisites

- [Docker Engine](https://docs.docker.com/engine/install/) + Docker Compose v2+
- A domain name (for TLS) or `localhost` for local testing
- 2 GB+ RAM (runs comfortably on Raspberry Pi 5)

### Option A: Deploy without cloning

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

On first launch, open your domain (or `http://localhost`). The wizard runs automatically and lets you configure instance name, domain, registration mode, and federation mode. It locks after completion — further changes require admin auth.

### 2. Create an Admin User

```bash
docker exec amityvox amityvox admin create-user admin admin@example.com YourSecurePassword
docker exec amityvox amityvox admin set-admin admin
```

### 3. Log In

Open your domain, log in with the admin account, and start using AmityVox.

## Configuration

All settings live in `.env`. Key variables:

| Variable | Default | Purpose |
|---|---|---|
| `AMITYVOX_INSTANCE_DOMAIN` | `localhost` | Public domain (TLS, WebAuthn, federation) |
| `AMITYVOX_INSTANCE_NAME` | `AmityVox` | Display name in the UI |
| `POSTGRES_PASSWORD` | `amityvox` | Database password (**change for production**) |
| `LIVEKIT_API_KEY` | `devkey` | LiveKit auth key (**change for production**) |
| `LIVEKIT_API_SECRET` | `secret` | LiveKit auth secret (**change for production**) |
| `MEILI_MASTER_KEY` | *(empty)* | Meilisearch API key (set for production) |
| `AMITYVOX_STORAGE_ACCESS_KEY` | *(empty)* | S3 access key (see Garage setup) |
| `AMITYVOX_STORAGE_SECRET_KEY` | *(empty)* | S3 secret key (see Garage setup) |

See [`docker_deploy/.env.example`](docker_deploy/.env.example) for all options including push notifications, Giphy, translation, and media settings.

### TLS / Custom Domain

Set `AMITYVOX_INSTANCE_DOMAIN` in `.env`. Caddy auto-provisions Let's Encrypt certs — just ensure ports 80 and 443 are open.

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

Copy the key ID and secret into `.env` as `AMITYVOX_STORAGE_ACCESS_KEY` and `AMITYVOX_STORAGE_SECRET_KEY`, then restart:

```bash
docker compose restart amityvox
```

## Services

The full stack runs 9 containers (~700 MB–1.2 GB total):

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
| `migrate up` | Run pending migrations |
| `migrate down` | Rollback last migration |
| `migrate status` | Show migration status |
| `version` | Print version info |

## Backup & Restore

```bash
# Backup
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
- [Live Instance](https://amityvox.chat/) — Try AmityVox without installing
- [GitHub Issues](https://github.com/WAN-Ninjas/AmityVox/issues) — Bug reports and feature requests

## License

[GNU Affero General Public License v3.0](LICENSE)
