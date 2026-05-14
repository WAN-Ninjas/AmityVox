# AmityVox Deployment Guide

This guide covers deploying AmityVox with Docker Compose.

## Prerequisites

- Docker Engine 24+ and Docker Compose v2+
- A domain name (for production with TLS)
- 2GB+ RAM (runs on Raspberry Pi 5 with 8GB)

## Quick Start

```bash
git clone https://github.com/WAN-Ninjas/AmityVox.git
cd AmityVox
cp .env.example .env
```

Edit `.env` with your settings, then:

```bash
make docker-up
```

## Initial Setup

### 1. Start the stack

```bash
make docker-up
```

Wait for all services to be healthy:

```bash
docker compose -f deploy/docker/docker-compose.yml ps
```

### 2. Configure Garage (S3 storage)

Garage requires one-time setup to create a bucket and API keys.

```bash
# Get the Garage node ID
docker exec amityvox-garage garage status

# Assign a zone layout (replace NODE_ID with the actual ID)
docker exec amityvox-garage garage layout assign -z dc1 -c 1G NODE_ID
docker exec amityvox-garage garage layout apply --version 1

# Create the storage bucket
docker exec amityvox-garage garage bucket create amityvox

# Create an API key
docker exec amityvox-garage garage key create amityvox-key

# Grant access to the bucket
docker exec amityvox-garage garage bucket allow amityvox --read --write --key amityvox-key

# Get the access and secret keys
docker exec amityvox-garage garage key info amityvox-key
```

Copy the `Key ID` and `Secret key` from the output and add them to your `.env`:

```bash
AMITYVOX_STORAGE_ACCESS_KEY=<Key ID>
AMITYVOX_STORAGE_SECRET_KEY=<Secret key>
```

Then restart AmityVox:

```bash
make docker-restart
```

### 3. Create an admin user

```bash
docker exec amityvox amityvox admin create-user \
  --username admin \
  --email admin@example.com \
  --password 'YourSecurePassword'

docker exec amityvox amityvox admin set-admin --username admin
```

### 4. Build the web client

If you haven't built the frontend yet:

```bash
make web-install
make web-build
```

The built files are mounted into Caddy at `/srv` via docker-compose.

### 5. Access the web client

Navigate to `http://localhost` and log in with your admin credentials.

## Production Deployment

### TLS with Caddy

Edit `deploy/caddy/Caddyfile` — replace `:80` with your domain:

```
chat.example.com {
    handle /api/* {
        reverse_proxy amityvox:8080
    }
    handle /health {
        reverse_proxy amityvox:8080
    }
    handle /ws {
        reverse_proxy amityvox:8081
    }
    handle /.well-known/amityvox {
        reverse_proxy amityvox:8080
    }
    handle /federation/* {
        reverse_proxy amityvox:8080
    }
    handle {
        root * /srv
        try_files {path} /index.html
        file_server
    }
}
```

Caddy automatically provisions Let's Encrypt TLS certificates. Ensure ports 80 and 443 are open.

### Update `.env` for production

```bash
AMITYVOX_INSTANCE_DOMAIN=chat.example.com
AMITYVOX_INSTANCE_NAME="My Community"
POSTGRES_PASSWORD=<strong-random-password>
MEILI_MASTER_KEY=<random-32-char-key>
AMITYVOX_LOGGING_LEVEL=info
```

### WebAuthn configuration

For WebAuthn (hardware key login) to work, the domain and origins must match:

In `amityvox.toml` or via environment:
```toml
[auth.webauthn]
rp_display_name = "My Community"
rp_id = "chat.example.com"
rp_origins = ["https://chat.example.com"]
```

### Push notifications

Generate VAPID keys:

```bash
npx web-push generate-vapid-keys
```

Add to `.env`:

```bash
AMITYVOX_PUSH_VAPID_PUBLIC_KEY=<public key>
AMITYVOX_PUSH_VAPID_PRIVATE_KEY=<private key>
AMITYVOX_PUSH_VAPID_CONTACT_EMAIL=admin@example.com
```

## Service Architecture

```
                    Internet
                       │
                  ┌────▼────┐
                  │  Caddy   │ :80/:443 (auto-TLS)
                  └────┬────┘
              ┌────────┼────────┐
              │        │        │
         /api/*    /ws     static files
              │        │      (/srv)
         ┌────▼────┐ ┌─▼──┐
         │ AmityVox │ │ WS │  :8080/:8081
         └────┬────┘ └─┬──┘
              │        │
    ┌─────────┼────────┼─────────┐
    │         │        │         │
┌───▼──┐ ┌───▼──┐ ┌───▼───┐ ┌──▼───┐
│Postgres│ │ NATS │ │Dragonfly│ │Garage│
│  :5432 │ │:4222 │ │ :6379  │ │:3900 │
└────────┘ └──────┘ └────────┘ └──────┘
```

## Updating

```bash
git pull
make docker-restart   # Rebuilds only AmityVox, keeps data
make web-build        # Rebuild frontend if changed
```

## Backup

### Database
```bash
docker exec amityvox-postgresql pg_dump -U amityvox amityvox > backup.sql
```

### Restore
```bash
docker exec -i amityvox-postgresql psql -U amityvox amityvox < backup.sql
```

## Troubleshooting

### View logs
```bash
make docker-logs                                          # All services
docker logs amityvox -f                                    # Just AmityVox
docker logs amityvox-postgresql -f                         # Just PostgreSQL
```

### Check service health
```bash
curl http://localhost:8080/health
docker compose -f deploy/docker/docker-compose.yml ps
```

### Reset everything
```bash
make docker-down
docker volume rm $(docker volume ls -q | grep amityvox)
make docker-up
```

### Common issues

**"connection refused" on startup** — PostgreSQL may not be ready yet. The `depends_on` with health check handles this, but if AmityVox starts before PostgreSQL is fully initialized, restart it: `make docker-restart`

**Garage "Access Denied"** — Ensure you've run the Garage setup steps (bucket create, key create, bucket allow). The access and secret keys in `.env` must match.

**WebSocket connection fails** — Ensure your reverse proxy (Caddy) is forwarding WebSocket connections. The Caddyfile handles this by default, but custom nginx/Apache configs may need explicit WebSocket upgrade headers.
