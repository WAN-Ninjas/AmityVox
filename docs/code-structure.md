# Code Structure

This map describes the current repository layout and where maintainers should make changes.

## Root

| Path | Purpose |
|---|---|
| `cmd/amityvox/` | CLI entrypoint and server wiring |
| `internal/` | Go application packages |
| `web/` | SvelteKit frontend |
| `deploy/` | Docker, Caddy, Garage, LiveKit, Prometheus, Grafana, and LibreTranslate deployment files |
| `docker_deploy/` | Minimal prebuilt-image deployment bundle |
| `bridges/` | Bridge adapter stubs/implementations for Discord, IRC, Matrix, Slack, and Telegram |
| `desktop/` | Tauri desktop configuration |
| `scripts/` | Operational scripts such as backup/restore |
| `docs/` | Current documentation and archived planning/audit docs |
| `.github/` | GitHub automation |
| `.claude/` | Local agent/workflow scaffolding |
| `.opencode/` | Local OpenCode scaffolding, currently untracked in this workspace |

## Backend Packages

| Package | Responsibility |
|---|---|
| `internal/api` | Main API server, middleware, health, metrics, voice, search, Giphy, MFA, rate limiting |
| `internal/api/admin` | Admin instance, users, guilds, features, federation, self-hosting, updates, backups, bridges |
| `internal/api/activities` | Activities, games, watch-together, music-party |
| `internal/api/bookmarks` | Message bookmark handlers |
| `internal/api/bots` | Bot ownership, tokens, commands, permissions, presence, rate limits, subscriptions |
| `internal/api/channels` | Channels, messages, reactions, pins, threads, forums, gallery, emoji, templates, translation |
| `internal/api/experimental` | Location, effects, summaries, transcription, whiteboards, code snippets, recordings, kanban |
| `internal/api/guildevents` | Guild events and RSVPs |
| `internal/api/guilds` | Guilds, members, roles, bans, categories, invites, features, retention, templates, channel groups |
| `internal/api/integrations` | ActivityPub-style integrations and bridge connections |
| `internal/api/invites` | Invite resolve/accept/delete |
| `internal/api/moderation` | Reports, issue system, ban lists, global moderation |
| `internal/api/onboarding` | Guild onboarding prompts and completion |
| `internal/api/polls` | Poll create/read/vote/close/delete |
| `internal/api/social` | Boosts, vanity, achievements, leveling, leaderboard, starboard, welcome, auto roles |
| `internal/api/stickers` | User/guild sticker packs and sharing |
| `internal/api/themes` | Shared theme gallery |
| `internal/api/users` | Self/user profile, guilds, DMs, relationships, blocks, activity, links, export/import |
| `internal/api/webhooks` | Webhook templates, preview, execution, SSRF protections |
| `internal/api/widgets` | Guild/channel widgets, plugins, key backup |
| `internal/auth` | Password hashing, sessions, middleware |
| `internal/automod` | AutoMod service, filters, rule handlers |
| `internal/config` | TOML config, environment overrides, defaults, validation |
| `internal/database` | PostgreSQL pool, migration runner, embedded migrations |
| `internal/encryption` | MLS key-package, welcome, group-state, commit handlers |
| `internal/events` | NATS event bus |
| `internal/features` | Feature flag definitions and resolution |
| `internal/federation` | Discovery, handshake, signed sync, DMs, guilds, invites, media, users, voice, MLS |
| `internal/gateway` | WebSocket protocol and event dispatch |
| `internal/media` | S3 upload/fetch/delete/tag support |
| `internal/mentions` | Mention parsing/resolution |
| `internal/middleware` | Security, CSP, tracing middleware |
| `internal/models` | Shared model types, permissions, ULIDs |
| `internal/notifications` | Notifications, preferences, push subscriptions |
| `internal/permissions` | Permission calculation and checks |
| `internal/plugins` | Plugin definitions and sandbox helpers |
| `internal/presence` | Redis/Dragonfly presence and cache |
| `internal/scanning` | ClamAV integration |
| `internal/search` | Meilisearch integration |
| `internal/voice` | LiveKit integration |
| `internal/workers` | Background worker manager and jobs |

## Frontend Structure

| Path | Purpose |
|---|---|
| `web/src/routes/+page.svelte` | Public landing/root page |
| `web/src/routes/login`, `register`, `setup`, `invite` | Auth and first-run flows |
| `web/src/routes/app` | Authenticated app shell |
| `web/src/routes/app/admin` | Admin dashboard pages |
| `web/src/routes/app/guilds/[guildId]` | Guild layout and channel pages |
| `web/src/routes/app/dms/[channelId]` | DM channel page |
| `web/src/routes/app/moderation` | Moderation surface |
| `web/src/routes/app/settings`, `friends`, `discover`, `notifications`, `plugins`, `themes`, `bookmarks` | User-facing app sections |
| `web/src/lib/api` | REST and WebSocket clients |
| `web/src/lib/stores` | Svelte stores for app state |
| `web/src/lib/types` | Frontend type definitions |
| `web/src/lib/utils` | Shared UI/client helpers |
| `web/src/lib/encryption` | Browser encryption helpers |

## Database Structure

- Migrations are embedded from `internal/database/migrations`.
- Migration files use numeric prefixes and `.up.sql` / `.down.sql` pairs.
- Latest migration at rewrite time: `072_issue_access_tokens`.
- Do not edit applied migrations. Add a new numbered migration pair.

## Adding New Features

1. Add schema changes in a new migration pair.
2. Add or update model types in `internal/models` when shared across handlers.
3. Add service logic in the closest existing package.
4. Register HTTP routes in `internal/api/server.go` or federation routes in `cmd/amityvox/main.go`.
5. Add frontend API client methods in `web/src/lib/api/client.ts`.
6. Add or update stores/components/routes under `web/src`.
7. Add tests close to the changed package.
