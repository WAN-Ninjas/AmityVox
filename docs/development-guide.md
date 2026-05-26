# Development Guide

## Requirements

- Go 1.26
- Node.js compatible with the frontend toolchain in `web/package.json`
- Docker Engine 24+ with Docker Compose v2
- npm for frontend dependencies

## Common Commands

| Task | Command |
|---|---|
| Build backend | `make build` |
| Run backend tests | `make test` |
| Run backend integration tests | `make test-integration` |
| Run vet/staticcheck wrapper | `make lint` |
| Install frontend deps | `make web-install` |
| Build frontend | `make web-build` |
| Check frontend | `make web-check` |
| Start Docker stack | `make docker-up` |
| Stop Docker stack | `make docker-down` |
| Follow Docker logs | `make docker-logs` |
| Rebuild app containers | `make docker-restart` |
| Run Docker frontend tests | `make docker-test-frontend` |
| Run all standard checks | `make check` |

## Local Backend Run

```bash
cp amityvox.example.toml amityvox.toml
# Edit amityvox.toml for local service URLs.
make build
./amityvox serve
```

`serve` runs migrations automatically before accepting traffic.

## Docker Development

```bash
docker compose -f deploy/docker/docker-compose.yml up -d
docker compose -f deploy/docker/docker-compose.yml logs -f
```

Rebuild only the app/frontend init containers:

```bash
docker compose -f deploy/docker/docker-compose.yml up -d --build amityvox web-init
```

## Frontend Development

```bash
cd web
npm install
npm run dev
npm run check
npm test
```

The frontend API client lives in `web/src/lib/api/client.ts`; gateway helpers live in `web/src/lib/api/ws.ts`.

## Migrations

Migration files live in `internal/database/migrations`.

Rules:

- Add a new numbered `.up.sql` and `.down.sql` pair.
- Do not rewrite migrations that may have already run in user deployments.
- Keep migration names descriptive.
- Backend startup runs pending migrations automatically.
- CLI migration commands are available through `amityvox migrate`.

## Adding API Routes

1. Put handler code in the closest `internal/api/*` package.
2. Register the route in `internal/api/server.go`.
3. Add auth, admin, permission, rate-limit, and feature-gate middleware as needed.
4. Use standard response helpers from `internal/api/apiutil`.
5. Update `docs/api-reference.md`.
6. Add backend tests near the package being changed.

Federation peer routes are registered in `cmd/amityvox/main.go` because they are wired after federation services are initialized.

## Adding Frontend Features

1. Add API client methods in `web/src/lib/api/client.ts`.
2. Add or extend stores in `web/src/lib/stores`.
3. Add UI in `web/src/routes` or reusable components under `web/src/lib/components`.
4. Keep backend authorization authoritative even if UI hides unauthorized controls.
5. Run `npm run check` and relevant tests.

## Test Expectations

For backend-only changes:

```bash
go test ./...
```

For frontend changes:

```bash
cd web
npm run check
npm test
```

For Docker/deployment changes:

```bash
docker compose --env-file .env -f deploy/docker/docker-compose.yml config
```

Run broader checks before release or major refactors.

## Commit Guidance

- Keep commits focused.
- Use conventional commit messages.
- Do not commit secrets, `.env`, database dumps, generated backups, or host-specific safety copies.
- Force-add new `docs/` files when needed because the repository ignore rules ignore new docs by default.
