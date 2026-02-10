.PHONY: build test lint vet clean docker run migrate-up migrate-down help

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

## test: Run all tests
test:
	go test -race -count=1 ./...

## test-cover: Run tests with coverage report
test-cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

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

## help: Show this help message
help:
	@echo "AmityVox — self-hosted federated communication platform"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /' | sort
