.PHONY: build test lint vet clean docker run migrate-up migrate-down web-install web-build web-check docker-test docker-test-frontend docker-build help

# Build variables
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS  = -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(DATE)

# Default target
all: build

## build: Compile the AmityVox binary
build:
	go build -ldflags "$(LDFLAGS)" -o amityvox ./cmd/amityvox

## test: Run all Go tests
test:
	go test -race -count=1 ./...

## test-cover: Run tests with coverage report
test-cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## test-integration: Run integration tests (requires Docker)
test-integration:
	go test -race -count=1 -v ./internal/integration/

## vet: Run go vet
vet:
	go vet ./...

## lint: Run go vet and staticcheck (if installed)
lint: vet
	@which staticcheck > /dev/null 2>&1 && staticcheck ./... || echo "staticcheck not installed, skipping"

## clean: Remove build artifacts
clean:
	rm -f amityvox coverage.out coverage.html
	go clean -cache -testcache
	rm -rf web/build web/.svelte-kit

## docker: Build Docker image
docker:
	docker build -t amityvox:$(VERSION) -f deploy/docker/Dockerfile \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(DATE) .

## docker-up: Start all services with Docker Compose
docker-up:
	docker compose -f deploy/docker/docker-compose.yml up -d

## docker-down: Stop all services
docker-down:
	docker compose -f deploy/docker/docker-compose.yml down

## docker-logs: Follow logs from all services
docker-logs:
	docker compose -f deploy/docker/docker-compose.yml logs -f

## docker-restart: Rebuild and restart AmityVox (keeps other services)
docker-restart:
	docker compose -f deploy/docker/docker-compose.yml up -d --build amityvox web-init

## docker-build: Build all Docker images (backend + frontend)
docker-build:
	docker compose -f deploy/docker/docker-compose.yml build amityvox web-init

## docker-test-frontend: Run frontend tests in Docker
docker-test-frontend:
	docker compose -f deploy/docker/docker-compose.yml --profile test run --rm test-frontend

## docker-test: Run all tests in Docker (Go + frontend)
docker-test: test docker-test-frontend

## run: Build and run the server locally
run: build
	./amityvox serve

## migrate-up: Run database migrations
migrate-up: build
	./amityvox migrate up

## migrate-down: Rollback database migrations
migrate-down: build
	./amityvox migrate down

## deps: Download and tidy Go dependencies
deps:
	go mod download
	go mod tidy

## web-install: Install frontend dependencies
web-install:
	cd web && npm install

## web-build: Build the frontend for production
web-build:
	cd web && npm run build

## web-check: Type-check the frontend
web-check:
	cd web && npm run check

## web-dev: Start frontend dev server
web-dev:
	cd web && npm run dev

## check: Run all checks (Go tests + lint + frontend type-check)
check: test lint web-check

## setup: First-time setup (install all deps, build everything)
setup: deps web-install build web-build
	@echo ""
	@echo "Setup complete! Next steps:"
	@echo "  1. cp amityvox.example.toml amityvox.toml"
	@echo "  2. Edit amityvox.toml with your connection details"
	@echo "  3. make migrate-up"
	@echo "  4. make run"
	@echo ""

## help: Show this help message
help:
	@echo "AmityVox — self-hosted federated communication platform"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /' | sort
