# ============================================================
# KICKOFF PROMPT — Paste this into Claude Code
# ============================================================
# DO NOT commit this file. It's just the prompt text to copy/paste.
# ============================================================

Read CLAUDE.md and docs/architecture.md completely before doing anything else.

This is AmityVox — a federated Discord alternative. You are building Phase 1: Foundation.

Here is your task list for this session. Do them in order:

## Task 1: Go Project Scaffold

Initialize the Go module and create the full directory structure from CLAUDE.md. Create placeholder files where needed so the structure is navigable. Use Go 1.23+.

```
go mod init github.com/amityvox/amityvox
```

Add these initial dependencies:
- github.com/go-chi/chi/v5 (HTTP router)
- github.com/jackc/pgx/v5 (PostgreSQL driver)
- github.com/oklog/ulid/v2 (ULID generation)
- github.com/nats-io/nats.go (NATS client)
- github.com/minio/minio-go/v7 (S3 client)
- github.com/redis/go-redis/v9 (DragonflyDB/Redis client)
- github.com/pelletier/go-toml/v2 (config parsing)
- github.com/alexedwards/argon2id (password hashing)
- nhooyr.io/websocket (WebSocket)
- github.com/golang-migrate/migrate/v4 (DB migrations)
- golang.org/x/crypto (for various crypto needs)

## Task 2: Configuration System

Implement `internal/config/config.go`:
- Parse `amityvox.toml` (use amityvox.example.toml as the schema reference)
- Support environment variable overrides (AMITYVOX_DATABASE_URL, AMITYVOX_NATS_URL, etc.)
- Validate required fields
- Provide sane defaults for everything

## Task 3: Database Connection & Migrations

Implement `internal/database/database.go`:
- PostgreSQL connection pool using pgx
- Connection health check
- Graceful shutdown

Create `migrations/001_initial_schema.up.sql` and `migrations/001_initial_schema.down.sql`:
- Use the COMPLETE schema from docs/architecture.md Section 5.2
- Include ALL tables, indexes, and constraints exactly as specified

Implement migration runner that applies migrations on startup.

## Task 4: Core Models

Implement `internal/models/` with Go structs for all core entities:
- User, Guild, Channel, Message, Member, Role, Attachment, Embed, Reaction, Invite, etc.
- Include JSON tags for API serialization
- Include helper methods (e.g., ULID generation, timestamp helpers)
- Match the PostgreSQL schema exactly

## Task 5: ULID Helper

Implement `internal/models/ulid.go`:
- Thread-safe ULID generation
- ULID-to-time extraction
- String/binary conversion helpers

## Task 6: Permission System

Implement `internal/permissions/permissions.go`:
- All permission constants from docs/architecture.md Section 5.3
- The CalculatePermissions function from Section 5.4
- Helper functions: HasPermission, HasAnyPermission, HasAllPermissions
- Permission set display/debug helpers

## Task 7: CLI Entrypoint

Implement `cmd/amityvox/main.go`:
- Subcommands: serve, migrate, version
- `serve`: loads config, connects to database, runs migrations, starts HTTP + WS servers
- `migrate`: runs migrations only (up/down/status)
- `version`: prints version string
- Graceful shutdown on SIGINT/SIGTERM

## Task 8: HTTP Server Skeleton

Implement `internal/api/server.go` and route registration:
- Chi router with middleware (logging, recovery, CORS, request ID)
- Health check endpoint: GET /health
- API version prefix: /api/v1/
- Stub route groups for users, guilds, channels, messages, admin
- JSON error response helper
- JSON success response helper

## Task 9: Docker Setup

Create `deploy/docker/Dockerfile`:
- Multi-stage build (Go builder → distroless/alpine runtime)
- Build for linux/amd64 and linux/arm64
- Copy binary + migration files + default config

Create `deploy/docker/docker-compose.yml`:
- All services: amityvox, postgresql, nats, dragonflydb, minio, livekit, meilisearch, caddy
- Proper networking, volumes, health checks, depends_on
- Environment variable configuration

Create `deploy/caddy/Caddyfile`:
- Reverse proxy to amityvox API on /api/*
- Reverse proxy to amityvox WebSocket on /ws
- Serve static web client files on /
- Auto-TLS when domain is configured

## Task 10: Verify Everything Compiles

Run `go build ./...` and fix any issues. Run `go vet ./...`. The project should compile cleanly with zero errors.

---

IMPORTANT RULES:
- Write COMPLETE files, not stubs with "TODO" comments (except for route handlers that will be implemented in Phase 2)
- Follow all conventions in CLAUDE.md
- Use pgx directly — NO ORMs
- All SQL in migrations, not hardcoded in Go files (except queries)
- Test that `go build ./cmd/amityvox` produces a working binary
- Every file should have a package doc comment explaining its purpose
