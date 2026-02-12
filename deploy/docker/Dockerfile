# AmityVox multi-stage Docker build.
# Stage 1: Build the SvelteKit frontend.
# Stage 2: Build the Go binary.
# Stage 3: Create a minimal runtime image.
# Supports linux/amd64 and linux/arm64.

# ============================================================
# Stage 1: Frontend Build (Node.js 24 LTS)
# ============================================================
FROM node:24-alpine AS frontend

WORKDIR /web

# Cache npm dependencies.
COPY web/package.json web/package-lock.json ./
RUN npm ci

# Copy frontend source and build.
COPY web/ .
RUN npm run build

# ============================================================
# Stage 2: Backend Build (Go 1.26)
# ============================================================
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /build

# Cache Go module downloads.
COPY go.mod go.sum ./
RUN go mod download

# Copy source code.
COPY . .

# Build static binary with version info.
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildDate=${BUILD_DATE}" \
    -o /amityvox \
    ./cmd/amityvox

# ============================================================
# Stage 3: Runtime
# ============================================================
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -g 1000 amityvox && \
    adduser -u 1000 -G amityvox -D amityvox

COPY --from=builder /amityvox /usr/local/bin/amityvox
COPY --from=frontend /web/build /srv/web
COPY amityvox.example.toml /etc/amityvox/amityvox.toml

USER amityvox
WORKDIR /home/amityvox

ENV AMITYVOX_CONFIG_PATH=/etc/amityvox/amityvox.toml

EXPOSE 8080 8081 9090

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:8080/health || exit 1

ENTRYPOINT ["amityvox"]
CMD ["serve"]
