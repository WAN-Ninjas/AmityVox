# AmityVox

A self-hosted, federated, optionally-encrypted communication platform. Think Discord, but open source, federated, and designed to run on a Raspberry Pi.

**v0.5.0** | [AGPL-3.0](LICENSE) | [Discord](https://discord.gg/VvxgUpF3uQ) | [Live Instance](https://amityvox.chat/)

First, a bit about myself. I am a 27 year veteran in IT, with the bulk of that in senior and management roles in what is now known as DevOps and Network Ops/Engineering. This software was written with Codex and Claude Code. This software was architected and specced out across a 31 page document by myself and my friends, with no AI involvement other than taking the spec we wrote and making it more presentable. If AI written code offends you, please move along. If a project that is not yet in a completed and ready-for-public state offends you, move along, come check back later. This is being worked on daily by myself and a small team of friends who all share the same vision: Take all the things we love about Discord, Matrix, and other platforms we have used/tried... and create one cohesive self-hostable open source, AGPL3 licensed project that we can use for ourselves and our communities. We have decided to make this project available to everyone as a means to provide people, when we are done, a powerful turn key solution that can act as a viable alternative to Discord that will NEVER be monetized. It will be maintained as long as we ourselves are using it. Should that ever change, our intention with the code being produced is to make sure it is well documented, and full code spec documents will be made available once we are done with our current development phase. 

Right now, the code is a mess. It's getting better through a lot of manual review and changes as it evolves. It is presently in a functional state(any code you see here, is what is running on the version hosted at amityvox.chat, sometimes off by a few hours if we're working on specific new features).

AmityVox, at it's core, is not a Matrix replacement. I've been asked several times about how certain features work or are going to work. We are going to be building in some messaging encryption, but the core of this is not architected for privacy-first like Matrix. What is LOGGED is ultimately not under our control outside of the instance we host. Even if we disable logging, it would be trivial for someone to re-enable it or add their own in, being open source. Instead, we have focused on features being OPTIONAL. Such as requiring e-mail validation to register. Federation will be OPTIONAL and disabled by default.

Federation, and how it will work:
We intend to host a 'Master/Public' federation server, and joining that is optional. Beyond that, you have three other federation modes: Open(Private, but anyone who knows your server exists can add your server to their federation list and interact with your instance.), Closed(Requires exchanging of keys and whitelisting on both sides), Disabled(No federation at all, completely standalone). When you are federated with another server, you may:
Join guilds on other instances you are federated with.
DM both 1:1 and 1:Many between people on your instance and instances you are federated with.
Voice chat, video chat and screenshare between instances that are federated together. This includes guild voice/video chat. It will require all involved parties to have LiveKit setup correctly in their stack.

Within a week or so of writing this(2/18/26), the front end will be locked in and set as stable. At that time, one of my friends will be getting Tauri going and handling getting Windows, MacOS, Linux, Android, and iPad/iPhone apps going and start figuring out the app registration stuff for Google and Apple. The apps will be instance agnostic, when you launch, you'll be asked what instance you want to connect to. You can choose one of our instances, or you can specify another.

Everything below this is mostly written by AI. I'm keeping it in because it has good information. Not all of it is 100% accurate as of the update to this file, I will endeavor to update it before we 'launch'.


## Features(not all implemented yet. Will all be finished by end of 02/2026)

- Guilds with channels, categories, roles, and granular permissions
- Real-time messaging with replies, reactions, threads, pins, and markdown
- Message expiration options at DM level, Guild level, and Instance level(Configurable by the instance host)
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
- [Live Instance](https://amityvox.chat/) — Try AmityVox without installing
- [GitHub Issues](https://github.com/WAN-Ninjas/AmityVox/issues) — Bug reports and feature requests

## License

[GNU Affero General Public License v3.0](LICENSE)
