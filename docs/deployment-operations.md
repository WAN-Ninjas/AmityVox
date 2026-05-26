# Deployment and Operations

AmityVox is deployed with Docker Compose. The main deployment files live in `deploy/docker/`; the prebuilt-image bundle lives in `docker_deploy/`.

## Install

Recommended interactive install:

```bash
curl -fsSL https://raw.githubusercontent.com/WAN-Ninjas/AmityVox/main/install.sh | bash
```

Manual source deploy:

```bash
git clone https://github.com/WAN-Ninjas/AmityVox.git amityvox
cd amityvox
cp .env.example .env
docker compose --env-file .env -f deploy/docker/docker-compose.yml up -d --build
```

Prebuilt-image deploy:

```bash
mkdir amityvox && cd amityvox
curl -O https://raw.githubusercontent.com/WAN-Ninjas/AmityVox/main/docker_deploy/docker-compose.yml
curl -O https://raw.githubusercontent.com/WAN-Ninjas/AmityVox/main/docker_deploy/.env.example
cp .env.example .env
docker compose up -d
```

Manual deployments must bootstrap Garage/S3 storage and create an admin user. The interactive installer does this automatically.

## Useful Commands

Set the install directory first:

```bash
export APP_DIR=/home/user/amityvox
```

| Task | Command |
|---|---|
| View logs | `cd "$APP_DIR" && docker compose --env-file .env -f deploy/docker/docker-compose.yml logs -f` |
| Stop | `cd "$APP_DIR" && docker compose --env-file .env -f deploy/docker/docker-compose.yml down` |
| Start | `cd "$APP_DIR" && docker compose --env-file .env -f deploy/docker/docker-compose.yml up -d` |
| Update | `cd "$APP_DIR" && ./update.sh` |
| Backup | `cd "$APP_DIR" && ./scripts/backup.sh` |
| Create user | `docker exec amityvox amityvox admin create-user <user> <email> <pass>` |

`down` keeps Docker volumes unless `-v` is added.

## Update

Source install update:

```bash
cd /home/user/amityvox
./update.sh
```

The update script preserves `.env` and Docker volumes, creates a pre-update backup under `./backups/`, pulls with fast-forward only, rebuilds `amityvox` and `web-init`, runs Compose, and restarts Caddy. Migrations run on server startup.

Flags:

```bash
AMITYVOX_SKIP_BACKUP=1 ./update.sh
AMITYVOX_NO_CACHE=1 ./update.sh
AMITYVOX_BRANCH=main ./update.sh
```

Prebuilt-image update:

```bash
docker compose pull
docker compose up -d
```

## Backup and Restore

Backup:

```bash
./scripts/backup.sh
./scripts/backup.sh /path/to/output
```

Restore:

```bash
./scripts/restore.sh ./backups/amityvox_YYYYMMDD_HHMMSS.tar.gz
```

Manual database-only backup:

```bash
docker exec amityvox-postgresql pg_dumpall -U amityvox > backup.sql
```

Manual database-only restore:

```bash
cat backup.sql | docker exec -i amityvox-postgresql psql -U amityvox
```

## Health and Metrics

| Endpoint | Purpose |
|---|---|
| `GET /health` | Basic API health |
| `GET /health/deep` | Database, NATS, cache, storage, search, voice, and runtime health |
| `GET /metrics` | Prometheus metrics when enabled |

## Configuration

Config is loaded from `amityvox.toml` or `AMITYVOX_CONFIG_PATH`, then overridden by `AMITYVOX_` environment variables. Docker deployments primarily use `.env`.

Important environment groups:

- `AMITYVOX_INSTANCE_*`
- `AMITYVOX_DATABASE_*`
- `AMITYVOX_NATS_*`
- `AMITYVOX_CACHE_*`
- `AMITYVOX_STORAGE_*`
- `AMITYVOX_LIVEKIT_*`
- `AMITYVOX_SEARCH_*`
- `AMITYVOX_AUTH_*`
- `AMITYVOX_MEDIA_*`
- `AMITYVOX_PUSH_*`
- `AMITYVOX_HTTP_*`
- `AMITYVOX_WEBSOCKET_*`
- `AMITYVOX_LOGGING_*`
- `AMITYVOX_FEDERATION_*`
- `AMITYVOX_GIPHY_*`
- `AMITYVOX_METRICS_*`

Federation mode accepts `open`, `allowlist`, and `closed`. Legacy `public` normalizes to `open`; legacy `disabled` normalizes to `closed`.

## First Admin User

```bash
docker exec amityvox amityvox admin create-user <username> <email> <password>
docker exec amityvox amityvox admin set-admin <username>
```

## Garage/S3 Bootstrap

Manual installs must initialize Garage storage before uploads work:

```bash
NODE_ID=$(docker exec amityvox-garage /garage status 2>&1 | grep -oP '[a-f0-9]{64}' | head -1)
docker exec amityvox-garage /garage layout assign -z dc1 -c 1G "$NODE_ID"
docker exec amityvox-garage /garage layout apply --version 1
docker exec amityvox-garage /garage bucket create amityvox
docker exec amityvox-garage /garage key create amityvox-key
docker exec amityvox-garage /garage bucket allow amityvox --read --write --key amityvox-key
docker exec amityvox-garage /garage key info amityvox-key
```

Copy the generated key ID and secret into `.env` as `AMITYVOX_STORAGE_ACCESS_KEY` and `AMITYVOX_STORAGE_SECRET_KEY`, then restart `amityvox`.
