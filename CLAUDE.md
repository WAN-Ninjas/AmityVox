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

**Phase 4: UI/QoL Overhaul** — See `TODO.md` for detailed sprint tasks.

## Development Environment — Docker First

**All development, building, and testing happens inside Docker.** Do not rely on locally installed Node.js, Go, or other tools. The Docker images contain the correct versions.

### Docker Build

The Dockerfile is a 3-stage multi-stage build:
1. **Stage 1 (frontend):** `node:24-alpine` — Installs npm deps, builds SvelteKit to static files
2. **Stage 2 (builder):** `golang:1.26-alpine` — Compiles Go binary with version info
3. **Stage 3 (runtime):** `alpine:3.21` — Minimal image with binary + frontend assets

Target architectures: `linux/amd64` and `linux/arm64`

### Key Commands
```bash
make docker-up              # Start all services
make docker-down            # Stop all services
make docker-restart         # Rebuild and restart (backend + frontend)
make docker-build           # Build all images without starting
make docker-test-frontend   # Run frontend tests in Docker
make docker-test            # Run all tests (Go + frontend)
make docker-logs            # Follow all service logs
```

### Rules
1. **Never assume local tool versions.** The Dockerfile pins Node.js, Go, and Alpine versions. All builds use Docker images.
2. **Frontend is built inside Docker.** The `web-init` service copies the built SvelteKit output to a shared volume that Caddy serves.
3. **Test in Docker.** Use `make docker-test-frontend` to run frontend tests in the correct Node.js version. Use `make test` for Go tests.
4. **Version changes happen in Dockerfile.** To upgrade Node, Go, or Alpine, update the `FROM` lines in `deploy/docker/Dockerfile`.

## Minimum Hardware Target

Raspberry Pi 5 with 8GB RAM. The Go binary should target 50-100MB RSS. Be careful with goroutine counts, connection pools, and buffer sizes.

## Feature Completion Rules

These rules prevent half-done features and orphaned code. Follow them strictly.

1. **End-to-end verification required.** Every feature must work from UI click through API call to database and back. No "TODO" or "placeholder" implementations allowed in committed code.
2. **Every backend handler must be wired to a route.** After writing a handler, verify it appears in the chi router setup in `internal/api/server.go`. Grep for the handler name to confirm.
3. **Every frontend API method must match a real backend route.** After adding a method to `api/client.ts`, verify the endpoint exists in the backend router. No dead API methods.
4. **Batch-load related data.** When fetching lists (messages, members, guilds), load related entities (authors, attachments, roles) in batch using `WHERE id = ANY($1)`. Never N+1 query.
5. **Error handling at every layer.** API client: catch and display errors. Backend: return proper error envelope `{"error": {"code": "...", "message": "..."}}`. Never silently swallow errors.
6. **Loading states for async operations.** Every button that triggers an API call shows loading state. Every list shows loading indicator while fetching.
7. **Frontend tests for new code.** Every new component gets a `*.test.ts` file. Every new store gets a `*.test.ts` file. No exceptions.
8. **WebSocket events for real-time features.** If data can change (messages, presence, typing), updates must flow through WebSocket events, not polling.
9. **Check TODO.md after completing work.** Check off completed items. If a feature is partially done, do not check it off.
10. **Read before write.** Always read existing code before modifying. Understand current patterns. Reuse existing utilities (writeJSON, writeError, ApiRequestError, etc).

## Frontend Conventions

See `docs/ui-standards.md` for complete frontend patterns including:
- Svelte 5 runes usage ($state, $derived, $effect, $props)
- Store patterns (Map-based, mutation functions)
- API error handling
- Context menu, modal, and toast patterns
- CSS theme colors and utility classes
- Testing standards

## Version Policy — Always Use Latest Stable

**All dependencies, tools, and infrastructure must use the latest stable versions.** Before writing code, verify you are targeting current releases. Do not pin to old versions.

### Current Target Versions (update when new releases ship):
- **Node.js:** 24.x LTS (Krypton)
- **Go:** 1.26.x
- **PostgreSQL:** 18.x
- **Meilisearch:** v1.35+
- **Svelte:** 5.x (latest)
- **SvelteKit:** 2.x (latest)
- **Vite:** 7.x
- **Tailwind CSS:** 4.x
- **TypeScript:** 5.9+
- **Caddy:** 2.x (latest)
- **NATS:** 2.x (latest)
- **Alpine (Docker base):** 3.21

### Rules:
1. When adding a new dependency, use the latest stable version — never copy old version numbers.
2. When modifying `package.json`, `go.mod`, `Dockerfile`, or `docker-compose.yml`, check if any specified versions are outdated and update them.
3. Use semver ranges (`^` prefix) in `package.json` for patch/minor auto-updates.
4. Pin major versions in Docker image tags (e.g., `postgres:18-alpine`, not `postgres:latest`).
5. Run `npm update` periodically to pull latest within semver ranges.

## What NOT To Do

- Don't use MongoDB, SQLite, or any database other than PostgreSQL
- Don't use an ORM (no GORM, no ent, no sqlx struct scanning magic)
- Don't use Gin, Echo, Fiber, or other heavy HTTP frameworks (use chi)
- Don't use Electron for anything
- Don't add dependencies without justification
- Don't skip error handling
- Don't store secrets in code — everything goes through config/env vars
- Don't break the API versioning — all routes under /api/v1/
- Don't use outdated package versions — always target latest stable releases
