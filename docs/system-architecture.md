# System Architecture

This document reflects the implementation in `cmd/amityvox`, `internal/`, `web/`, and `deploy/` as of 2026-05-26.

## Process Model

`cmd/amityvox/main.go` is the main entrypoint. The `serve` command:

1. Loads config from `amityvox.toml` or `AMITYVOX_CONFIG_PATH`, then applies `AMITYVOX_` environment overrides.
2. Connects to PostgreSQL.
3. Runs all pending migrations.
4. Bootstraps the local instance row and federation Ed25519 keypair.
5. Connects to NATS and ensures streams.
6. Connects to DragonflyDB/Redis for cache, sessions, and presence.
7. Initializes auth, media, search, federation, AutoMod, notification, worker, voice, and encryption services.
8. Registers HTTP routes on the API server.
9. Registers federation routes in the root router.
10. Starts the HTTP API server and WebSocket gateway.
11. Handles graceful shutdown on SIGINT/SIGTERM.

## Backend Layers

| Layer | Main packages | Responsibility |
|---|---|---|
| Entrypoint and CLI | `cmd/amityvox` | Serve, migrate, admin, version commands |
| HTTP API | `internal/api`, subpackages | REST routing and request handlers |
| Auth | `internal/auth` | Argon2id password auth, sessions, middleware |
| Database | `internal/database`, `internal/models` | PostgreSQL pool, migrations, model types, ULIDs |
| Events | `internal/events` | NATS event bus and JetStream setup |
| Gateway | `internal/gateway` | WebSocket protocol, heartbeats, dispatch, presence routing |
| Presence/cache | `internal/presence` | Redis-compatible presence/session cache |
| Media | `internal/media` | S3-compatible upload, fetch, metadata, thumbnails |
| Search | `internal/search` | Meilisearch indexes and message indexing |
| Voice | `internal/voice` | LiveKit token/state support |
| Federation | `internal/federation` | Discovery, handshake, signed delivery, DM/guild/voice/MLS sync |
| Moderation | `internal/automod`, `internal/api/moderation` | AutoMod, reports, issue system, ban lists |
| Encryption | `internal/encryption` | MLS key-package, welcome, group-state, commit endpoints |
| Notifications | `internal/notifications` | Notifications, preferences, push subscriptions |
| Workers | `internal/workers` | Background jobs for media, search, notifications, retention, scheduled messages, bans |

## HTTP API

The main REST API is mounted at `/api/v1` in `internal/api/server.go`. Public health and metrics routes are mounted at the root:

- `/health`
- `/health/deep`
- `/metrics`

Federation peer-to-peer routes are mounted at the root by `cmd/amityvox/main.go`:

- `/.well-known/amityvox`
- `/federation/v1/*`

The public file, widget, and webhook execution routes are mounted below `/api/v1`.

## Response Envelopes

Most API success responses use:

```json
{ "data": {} }
```

Most API errors use:

```json
{ "error": { "code": "string", "message": "string" } }
```

Some handlers use raw JSON for protocol-specific responses, health details, downloads, file bodies, or external protocol compatibility.

## Authentication

HTTP API auth uses bearer tokens:

```http
Authorization: Bearer <session-token>
```

`auth.RequireAuth` rejects missing or invalid tokens with 401. `auth.OptionalAuth` allows anonymous requests and injects user context when a valid token is present. Admin routes additionally check `models.UserFlagAdmin`.

## Feature Flags

Feature gates are enforced through `features.Resolve` and middleware in `internal/api/server.go` plus the federation setup in `cmd/amityvox/main.go`. Gates can be instance-wide or guild-scoped depending on the route. Admins can inspect and update feature flags through `/api/v1/admin/features` and guild feature routes.

## WebSocket Gateway

The gateway listens on configured `websocket.listen` and serves `/ws`. The wire format is:

```json
{ "op": 0, "t": "EVENT_NAME", "d": {}, "s": 1 }
```

Opcodes implemented in `internal/gateway/gateway.go`:

| Op | Name | Direction |
|---|---|---|
| 0 | Dispatch | Server to client |
| 1 | Heartbeat | Client to server |
| 2 | Identify | Client to server |
| 3 | Presence update | Client to server |
| 4 | Voice state update | Client to server |
| 5 | Resume | Client to server |
| 6 | Reconnect | Server to client |
| 7 | Request members | Client to server |
| 8 | Typing | Client to server |
| 9 | Subscribe | Client to server |
| 10 | Hello | Server to client |
| 11 | Heartbeat ACK | Server to client |

The server sends HELLO with heartbeat interval and build version, then clients identify with a session token. Events are consumed from NATS subjects and dispatched to connected users based on guild, channel, DM, friend, and subscription scope.

## Federation

Federation has two surfaces:

- Peer routes under `/federation/v1/*` for instance-to-instance traffic.
- Local proxy routes under `/api/v1/federation/*` for authenticated local users accessing remote guild, user, invite, peer, and voice resources.

The local instance keypair is stored in the `instances` table. Peer routes verify Ed25519 signatures where required. Signed federation routes intentionally avoid IP rate limiting because signature verification is the trust boundary for peer delivery.

Configured federation modes are `open`, `allowlist`, and `closed`. The config loader normalizes legacy `public` to `open` and `disabled` to `closed`.

## Data Storage

- SQL schema changes live in `internal/database/migrations`.
- Migrations are embedded and run automatically during `serve`.
- Migration count as of this doc rewrite: 72 up/down migration pairs.
- Uploaded media content is stored in S3-compatible object storage. File metadata, tags, ownership, and permissions remain in PostgreSQL.
- Meilisearch indexes are optional and initialized when search is enabled and reachable.

## Frontend

The frontend is a SvelteKit 2/Svelte 5 app in `web/`.

- `web/src/routes` contains pages and layouts.
- `web/src/lib/api/client.ts` contains API client calls.
- `web/src/lib/api/ws.ts` contains WebSocket client helpers.
- `web/src/lib/stores` contains application state stores.
- `web/src/lib/components` contains reusable UI components.
- `web/src/lib/encryption` contains browser-side encryption helpers.

The Docker build produces static frontend assets and serves them through Caddy and/or the deployed web initialization flow.
