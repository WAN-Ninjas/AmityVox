# Versioning

AmityVox currently uses version `0.5.0`.

## Canonical Files

The release version must stay aligned in:

- `VERSION`
- `web/package.json`
- `web/package-lock.json`

The Go default version is also declared in `cmd/amityvox/main.go` and should match the release version unless build tooling overrides it.

## Build Metadata

The Makefile injects build metadata with linker flags:

```make
VERSION ?= $(shell cat VERSION 2>/dev/null || echo "0.5.0")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS  = -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(DATE)
```

The runtime build version format is:

```text
version+commit.sanitizedBuildDate
```

The value is exposed by:

- `amityvox version`
- health/client-config responses where wired
- WebSocket gateway HELLO payload

## Release Checklist

1. Choose the new semantic version.
2. Update `VERSION`.
3. Update `web/package.json`.
4. Update `web/package-lock.json`.
5. Confirm `cmd/amityvox/main.go` fallback version is aligned.
6. Run backend and frontend checks appropriate for the change.
7. Build Docker images with the intended version metadata.
8. Update docs when install, update, API, or compatibility behavior changes.

## Compatibility Notes

- Database compatibility is controlled by migrations in `internal/database/migrations`.
- API compatibility is not currently expressed through multiple URL versions beyond `/api/v1`.
- Federation protocol routes are under `/federation/v1`.
- WebSocket protocol compatibility is opcode/event based; breaking changes should be documented in `docs/api-reference.md`.
