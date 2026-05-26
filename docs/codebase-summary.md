# Codebase Summary

AmityVox is a Go 1.26 backend with a SvelteKit 2/Svelte 5 frontend. The production deployment is Docker Compose.

## Main Runtime

- CLI entrypoint: `cmd/amityvox/main.go`
- Main server command: `amityvox serve`
- Database migrations: `internal/database/migrations`
- REST route registration: `internal/api/server.go`
- Federation route registration: `cmd/amityvox/main.go`
- WebSocket gateway: `internal/gateway/gateway.go`
- Frontend app: `web/src`

## Primary Services

- PostgreSQL stores users, guilds, channels, messages, federation state, media metadata, moderation data, feature flags, notifications, and admin settings.
- NATS distributes internal events and supports gateway fanout and workers.
- DragonflyDB/Redis stores sessions, presence, cache entries, and short-lived WebAuthn state.
- Garage/S3 stores uploaded object content.
- LiveKit provides voice/video tokens and room state.
- Meilisearch provides message full-text search.
- Caddy terminates HTTP/TLS and proxies API, gateway, media, and frontend traffic.

## Current Feature Areas

- Auth and user account lifecycle
- Guild/channel/role/member management
- DMs and group DMs
- Messaging, reactions, pins, threads, polls, scheduled/expiring messages
- Voice/video and soundboard/broadcast/screen-share controls
- Media upload/fetch/gallery/tagging
- Moderation, reports, issue export/access tokens, AutoMod, content scan, ban lists
- Federation for discovery, handshake, DMs, guilds, invites, media, voice, and MLS relay
- Notifications and PWA push
- Bot framework, commands, tokens, subscriptions, webhooks, plugins, widgets, themes, stickers, integrations, bridges
- Admin dashboards for instance, users, guilds, features, federation, rate limits, health, storage, backups, domains, and updates

## Test Coverage Shape

Tests exist across backend unit packages, API packages, federation, workers, gateway, media, permissions, search, and frontend encryption/client logic. Integration tests live under `internal/integration`.

## Operational Entry Points

- Install: `install.sh`
- Update: `update.sh`
- Backup: `scripts/backup.sh`
- Restore: `scripts/restore.sh`
- Docker Compose: `deploy/docker/docker-compose.yml`
- Prebuilt deployment files: `docker_deploy/`
