# Contributing to AmityVox

Thank you for your interest in contributing to AmityVox! This guide covers everything you need to get started.

## Development Setup

### Prerequisites

- **Go 1.23+** — Backend
- **Node.js 20+** — Frontend (SvelteKit)
- **PostgreSQL 16** — Database
- **NATS 2.x** — Message broker (with JetStream enabled)
- **DragonflyDB** or **Redis 7+** — Cache/sessions
- **Docker** — For integration tests and full-stack testing

### Getting Started

```bash
# Clone the repository
git clone https://github.com/WAN-Ninjas/AmityVox.git
cd AmityVox

# Install Go dependencies
make deps

# Copy config template and edit for your local setup
cp amityvox.example.toml amityvox.toml
# Edit amityvox.toml with your local PostgreSQL/NATS/Redis connection details

# Run database migrations
make migrate-up

# Build and run the backend
make run

# In a separate terminal, start the frontend dev server
cd web
npm install
npm run dev
```

### Running with Docker Compose

For a complete local environment with all services:

```bash
cp .env.example .env
make docker-up
```

## Code Style

### Go Backend

- **Go 1.23+** — Use latest stable features
- **No ORMs** — Use `pgx` (jackc/pgx/v5) for PostgreSQL. Write SQL directly.
- **No heavy frameworks** — Use `chi` (go-chi/chi/v5) for HTTP routing. Standard library for everything else.
- **IDs** — ULID format everywhere (`oklog/ulid`)
- **Errors** — Wrap with context: `fmt.Errorf("doing X: %w", err)`. No `panic` in library code.
- **Logging** — Use `slog` (standard library structured logging)
- **Testing** — Standard `go test`. Table-driven tests preferred. Test files next to source.

### Frontend

- **Svelte 5** — Use runes (`$state`, `$derived`, `$effect`, `$props`)
- **TypeScript** — Strict mode enabled
- **TailwindCSS** — Utility-first styling. Use the design tokens in `tailwind.config.js`.
- **Formatting** — Prettier with tabs, single quotes. Run `npm run format` before committing.

### API Conventions

- JSON response envelope: `{"data": ...}` for success, `{"error": {"code": "...", "message": "..."}}` for errors
- HTTP status codes: 200/201/204 for success, 400/401/403/404/409/429/500 for errors
- Bearer token authentication in `Authorization` header
- All routes under `/api/v1/`

## Project Structure

```
internal/
├── api/            # REST handlers — thin layer, calls services
├── auth/           # Auth service — registration, login, TOTP, WebAuthn
├── automod/        # AutoMod engine — filters, rules, actions
├── config/         # Config parsing — TOML + env overrides
├── database/       # DB connection + embedded migrations
├── encryption/     # MLS key management
├── events/         # NATS event bus — publish/subscribe
├── federation/     # Federation protocol — signed messages, sync
├── gateway/        # WebSocket gateway — opcodes, events
├── media/          # File handling — S3, thumbnails, blurhash
├── models/         # Shared types — User, Guild, Channel, Message, etc.
├── notifications/  # WebPush service
├── permissions/    # Bitfield permission system
├── presence/       # DragonflyDB presence/cache client
├── search/         # Meilisearch integration
├── voice/          # LiveKit integration
└── workers/        # Background jobs via NATS subscriptions
```

### Key Design Principles

1. **PostgreSQL is the source of truth.** Everything persists there. DragonflyDB is cache only.
2. **NATS is the nervous system.** REST handlers publish events; WebSocket gateway and workers subscribe.
3. **Stateless core.** The Go binary holds no critical state in memory. Multiple instances can run behind a load balancer.
4. **No circular imports.** Be careful with package dependencies — check `go vet ./...`.

## Database Migrations

Migrations live in `internal/database/migrations/` and are embedded into the binary via `//go:embed`.

### Adding a migration

1. Create two files:
   ```
   internal/database/migrations/NNN_description.up.sql
   internal/database/migrations/NNN_description.down.sql
   ```
2. Number sequentially (next after the highest existing number)
3. The `up.sql` creates/alters tables; the `down.sql` reverses the change
4. Test with `make migrate-up` and `make migrate-down`

### Current migrations

| # | Name | Tables |
|---|---|---|
| 001 | initial_schema | users, guilds, channels, messages, roles, members, reactions, pins, invites, bans, audit_log, webhooks, emoji, federation_peers, read_state, ... |
| 002 | user_notes_settings_vanity | user_notes, user_settings, vanity_urls |
| 003 | message_edit_history | message_edits |
| 004 | guild_member_count | guild member count denormalization |
| 005 | backup_codes | backup_codes |
| 006 | mls_encryption | mls_key_packages, mls_groups, mls_commits, mls_welcome_messages |
| 007 | automod | automod_rules, automod_actions |
| 008 | push_subscriptions | push_subscriptions, notification_preferences |

## Testing

### Unit tests

```bash
make test                    # Run all tests
make test-cover              # Generate HTML coverage report
go test ./internal/automod/  # Run specific package tests
```

### Integration tests

Integration tests use `dockertest` to spin up real PostgreSQL, NATS, and Redis containers:

```bash
go test ./internal/integration/ -v
```

These tests automatically skip if Docker is not available.

### Frontend checks

```bash
cd web
npm run check    # TypeScript + Svelte type checking
npm run lint     # Prettier + ESLint
```

## Making Changes

### Adding a new API endpoint

1. Add the route in `internal/api/server.go`
2. Create the handler in the appropriate sub-package (e.g., `internal/api/guilds/`)
3. Use `api.WriteJSON()` / `api.WriteError()` for responses
4. Add permission checks where needed
5. Publish events to NATS if the action should be broadcast via WebSocket

### Adding a new event type

1. Add the subject constant in `internal/events/events.go`
2. Publish from the REST handler or worker
3. Handle in `internal/gateway/` for WebSocket dispatch
4. Handle in relevant workers if background processing is needed

### Adding a frontend page

1. Create the route in `web/src/routes/`
2. Add API methods to `web/src/lib/api/client.ts` if needed
3. Use existing stores (`auth`, `guilds`, `channels`, `messages`, `presence`)
4. Follow existing component patterns and TailwindCSS conventions

## Pull Request Process

1. Fork the repo and create a feature branch
2. Write tests for new functionality
3. Ensure `make test` and `make lint` pass
4. Ensure `cd web && npm run check` passes
5. Write a clear PR description explaining what and why
6. Keep PRs focused — one feature or fix per PR

## Architecture Reference

Read [`docs/architecture.md`](docs/architecture.md) before making significant changes. It is the master specification for all design decisions.

## License

By contributing, you agree that your contributions will be licensed under the AGPL-3.0 license.
