# CLAUDE.md — AmityVox Project Context

## What Is This Project?

AmityVox is a self-hosted, federated, optionally-encrypted communication platform (like Discord but open source, federated, and self-hostable). Written in Go (backend) with SvelteKit (frontend). Licensed AGPL-3.0.

## Architecture Reference

**READ THIS FIRST:** `docs/architecture.md` is the master specification. All implementation decisions derive from that document. Read it fully before writing any code.

## Tech Stack

- **Backend:** Go (single binary, subcommands: serve, migrate, admin, version)
- **Frontend:** SvelteKit (builds to static files served by Caddy)
- **Database:** PostgreSQL (always — no SQLite, no MongoDB)
- **Message Broker:** NATS with JetStream
- **Cache/Sessions:** DragonflyDB (Redis-compatible)
- **Voice/Video:** LiveKit
- **File Storage:** Garage (S3-compatible) — swappable to MinIO/AWS S3/Wasabi
- **Search:** Meilisearch
- **Reverse Proxy:** Caddy
- **Deployment:** Docker Compose (PostgreSQL, NATS, DragonflyDB, Garage, LiveKit, Meilisearch, Caddy)

## Project Structure

```
amityvox/
├── cmd/amityvox/           # CLI entrypoint (main.go)
├── internal/
│   ├── config/             # TOML config parsing
│   ├── database/           # PostgreSQL connection, migrations, queries
│   ├── models/             # Shared data types
│   ├── auth/               # Authentication (Argon2id, TOTP, WebAuthn, sessions)
│   ├── permissions/        # Bitfield permission system
│   ├── api/                # REST API handlers (/api/v1/*)
│   │   ├── users/
│   │   ├── guilds/
│   │   ├── channels/
│   │   ├── messages/
│   │   └── admin/
│   ├── gateway/            # WebSocket gateway
│   ├── federation/         # Instance-to-instance protocol
│   ├── media/              # File upload, S3 operations
│   ├── encryption/         # MLS key management
│   ├── search/             # Meilisearch integration
│   ├── presence/           # Online/offline tracking
│   ├── events/             # NATS pub/sub event bus
│   └── workers/            # Background jobs
├── web/                    # SvelteKit frontend
├── bridges/
│   ├── matrix/
│   └── discord/
├── migrations/             # SQL migration files (numbered)
├── deploy/
│   ├── docker/
│   │   ├── Dockerfile
│   │   └── docker-compose.yml
│   └── caddy/
│       └── Caddyfile
├── docs/
│   └── architecture.md     # THE master spec — read this first
├── amityvox.example.toml   # Default configuration template
├── CLAUDE.md               # This file
├── go.mod
├── go.sum
└── LICENSE
```

## Code Style & Conventions

- **Go version:** 1.23+ (use latest stable)
- **No ORMs.** Use `pgx` (jackc/pgx) for PostgreSQL. Write SQL directly.
- **No heavy frameworks.** Use `chi` (go-chi/chi) for HTTP routing. Standard library for everything else possible.
- **IDs:** ULID format everywhere (use oklog/ulid)
- **Errors:** Wrap with context using `fmt.Errorf("doing X: %w", err)`. No panic in library code.
- **Logging:** Use `slog` (standard library structured logging)
- **Config:** TOML format, parsed with BurntSushi/toml or pelletier/go-toml
- **Testing:** Standard `go test`. Table-driven tests preferred. Test files next to source.
- **SQL migrations:** Numbered files in `migrations/` directory (e.g., `001_initial_schema.up.sql`, `001_initial_schema.down.sql`). Use golang-migrate/migrate.
- **API responses:** JSON. Use consistent envelope: `{"data": ...}` for success, `{"error": {"code": "...", "message": "..."}}` for errors.
- **HTTP status codes:** 200 OK, 201 Created, 204 No Content, 400 Bad Request, 401 Unauthorized, 403 Forbidden, 404 Not Found, 409 Conflict, 429 Rate Limited, 500 Internal Server Error.
- **Authentication:** Bearer token in `Authorization` header. Session tokens stored in `user_sessions` table.
- **WebSocket:** Use gorilla/websocket or nhooyr.io/websocket (coder/websocket).
- **NATS:** Use nats-io/nats.go
- **S3:** Use minio/minio-go (generic S3 client — works with Garage, MinIO, AWS, any S3-compatible backend)
- **Password hashing:** Argon2id via alexedwards/argon2id

## Key Design Principles

1. **PostgreSQL is the source of truth.** Everything persists there. DragonflyDB is cache only.
2. **NATS is the nervous system.** All real-time events flow through NATS. REST handlers publish to NATS; WebSocket gateway subscribes from NATS.
3. **Stateless core.** The Go binary holds no critical state in memory. Multiple instances can run behind a load balancer.
4. **Every service communicates via network.** No shared filesystem, no shared memory. Enables multi-machine distribution.
5. **Permission checks happen server-side.** Client receives computed permissions, never raw bitfields.
6. **Federation is designed-in from day one** even though the implementation comes in v0.2.0. Data model and events account for remote users and instances.

## Current Phase

**Phase 1: Foundation** — Project scaffold, database schema, config loading, basic auth, REST API skeleton.

## Docker Build

The Dockerfile should be a multi-stage build:
1. Stage 1: Go build (produces single static binary)
2. Stage 2: Minimal runtime image (distroless or alpine)

Target architectures: `linux/amd64` and `linux/arm64`

## Minimum Hardware Target

Raspberry Pi 5 with 8GB RAM. The Go binary should target 50-100MB RSS. Be careful with goroutine counts, connection pools, and buffer sizes.

## What NOT To Do

- Don't use MongoDB, SQLite, or any database other than PostgreSQL
- Don't use an ORM (no GORM, no ent, no sqlx struct scanning magic)
- Don't use Gin, Echo, Fiber, or other heavy HTTP frameworks (use chi)
- Don't use Electron for anything
- Don't add dependencies without justification
- Don't skip error handling
- Don't store secrets in code — everything goes through config/env vars
- Don't break the API versioning — all routes under /api/v1/
