# Code Standards

These standards reflect the current codebase and should guide future changes.

## General

- Prefer small, direct changes that fit existing package boundaries.
- Keep behavior close to the module that owns it.
- Do not introduce a new abstraction unless it removes real duplication or matches an existing pattern.
- Do not edit old applied migrations. Add new numbered migration pairs.
- Keep docs current when changing routes, deployment behavior, versioning, or public workflows.

## Go

- Use standard Go formatting with `gofmt`.
- Keep handlers thin where practical: parse input, call service/database logic, emit response.
- Use `apiutil.WriteJSON`, `apiutil.WriteError`, and `apiutil.WriteNoContent` for normal API responses.
- Return standard error envelopes unless a protocol route requires raw JSON or file/body output.
- Use request context for database calls.
- Prefer transactions through existing helpers when multiple writes must commit together.
- Enforce auth, admin, permission, and feature gates at route/middleware boundaries.
- Publish events after successful writes when realtime clients or workers must react.

## API

- REST API routes belong under `/api/v1` unless they are health, metrics, federation peer routes, or protocol-specific public endpoints.
- Use bearer auth for authenticated user routes.
- Use admin middleware for instance admin routes.
- Use feature-gate middleware for optional features.
- Keep route names resource-oriented and consistent with existing groups.
- Document new endpoints in `docs/api-reference.md`.

## Database

- Use ULID-compatible IDs where existing tables do.
- Add `created_at` and `updated_at` consistently with surrounding tables.
- Add down migrations that reverse the up migration.
- Keep data migrations explicit and idempotent where possible.

## Frontend

- Use SvelteKit routes for page-level concerns.
- Put API calls in `web/src/lib/api/client.ts` unless a domain-specific client already exists.
- Put shared state in `web/src/lib/stores`.
- Keep component text concise and task-focused.
- Reuse existing layout, modal, toast, and confirmation patterns.
- Avoid duplicating backend permission logic in the UI. The UI can hide controls, but the API must enforce authorization.

## Testing

- Backend: use `go test ./...` or narrower package tests while iterating.
- Frontend type checks: `cd web && npm run check`.
- Frontend unit tests: `cd web && npm test`.
- Docker frontend tests: `make docker-test-frontend`.
- Full local check target: `make check`.

## Security

- Do not commit `.env`, credentials, access tokens, private keys, database dumps, or production config.
- Keep SSRF-sensitive outbound webhook behavior inside existing guarded helpers.
- Treat federation signatures as protocol security boundaries.
- Keep upload limits, content scanning, media permissions, and EXIF stripping in mind for media changes.
