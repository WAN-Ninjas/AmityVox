# Project Overview

AmityVox is a self-hosted, federated communication platform implemented as a Go backend, SvelteKit frontend, and Docker Compose deployment. It targets communities that want Discord-like guilds, channels, DMs, moderation tools, voice/video, and optional federation under their own operational control.

## Current Version

- Application version: `0.5.0`
- Canonical version file: `VERSION`
- Frontend version mirror: `web/package.json`
- Backend build metadata: injected into `cmd/amityvox` by linker flags and exposed through CLI, health, client config, and gateway HELLO payloads.

## Core Capabilities

- User accounts, sessions, registration controls, MFA, WebAuthn, backup codes, user settings, relationships, blocking, notes, and account export/import.
- Guilds with channels, categories, roles, permission overrides, members, invites, bans, audit logs, onboarding, templates, channel groups, retention policies, vanity URLs, discovery, and widgets.
- Messaging with text messages, edits, delete, bulk delete, reactions, pins, typing, acknowledgements, threads, replies, polls, scheduled messages, expiring messages, bookmarks, crosspost/publish flows, translation, and reports.
- Voice/video through LiveKit, including joins/leaves, voice state, server mute/deafen/move, preferences, input mode, priority speaker, soundboard, broadcast, and screen-share endpoints.
- Media uploads through S3-compatible storage, with metadata rows, direct file fetch, attachment updates, deletion, gallery tags, and media tagging.
- Search through Meilisearch for messages plus API-backed user/guild search.
- Moderation with reports, global issues, issue export/access tokens, AutoMod rules, raid config, warnings, ban lists, instance bans, content scanning, and rate-limit diagnostics.
- Federation using instance discovery, handshake, signed inbox/sync routes, federated DMs, federated guilds, invites, media proxying, voice token exchange, MLS relay operations, admin diagnostics, and local proxy routes for users accessing remote resources.
- Optional MLS-style end-to-end encryption support for key packages, welcome messages, group state, commits, and key backups.
- Notifications and PWA push subscriptions, type preferences, channel preferences, unread counts, search, and mark-read/clear flows.
- Extensibility features including bots, bot tokens, commands, event subscriptions, webhooks, widgets, plugin installs, themes, stickers, bridge connections, and ActivityPub-style integrations.

## Users and Roles

- Anonymous users can load public configuration, registration status, public guild/widget/file routes, invites, and selected federation discovery routes.
- Authenticated users can use the app surface: guilds, DMs, messages, settings, media, voice, search, notifications, reports, federation proxy routes, and personalization.
- Guild moderators and administrators can manage guild-specific moderation, roles, bans, reports, AutoMod, onboarding, integrations, webhooks, widgets, and feature flags according to permission checks.
- Instance administrators can manage instance settings, users, guilds, registration, features, federation, reports, rate limits, content scanning, updates, storage, backups, bridges, and media from `/api/v1/admin`.
- Remote instance operators and tools can use issue export/access-token routes under `/api/v1/support/issues`.
- Federated peers communicate through `/federation/v1/*` routes signed with instance keys.

## Runtime Services

The default Docker deployment runs:

| Service | Role |
|---|---|
| `amityvox` | Go API server, WebSocket gateway, workers, migrations, CLI |
| `postgresql` | Primary relational database |
| `nats` | Event bus and JetStream streams |
| `dragonflydb` | Redis-compatible cache/session/presence store |
| `garage` | S3-compatible object storage |
| `livekit` | WebRTC SFU for voice/video |
| `meilisearch` | Full-text search |
| `libretranslate` | Optional translation backend |
| `caddy` | Reverse proxy and TLS termination |

## Important Boundaries

- PostgreSQL is the source of truth. Migrations live in `internal/database/migrations`.
- NATS is used for internal events and gateway fanout.
- DragonflyDB stores short-lived sessions, presence, and WebSocket/session cache data.
- S3-compatible storage stores uploaded media; PostgreSQL stores metadata and permissions.
- The SvelteKit app talks to the Go API through `web/src/lib/api/client.ts` and realtime gateway through `web/src/lib/api/ws.ts`.
- Feature flags exist at instance and guild scope and gate many newer features.
