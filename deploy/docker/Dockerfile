# AmityVox multi-stage Docker build.
# Stage 1: Build the Go binary.
# Stage 2: Create a minimal runtime image.
# Supports linux/amd64 and linux/arm64.

# ============================================================
# Stage 1: Build
# ============================================================
FROM golang:1.23-alpine AS builder

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
# Stage 2: Runtime
# ============================================================
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -g 1000 amityvox && \
    adduser -u 1000 -G amityvox -D amityvox

COPY --from=builder /amityvox /usr/local/bin/amityvox
COPY amityvox.example.toml /etc/amityvox/amityvox.toml

USER amityvox
WORKDIR /home/amityvox

ENV AMITYVOX_CONFIG_PATH=/etc/amityvox/amityvox.toml

EXPOSE 8080 8081 9090

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:8080/health || exit 1

ENTRYPOINT ["amityvox"]
CMD ["serve"]
