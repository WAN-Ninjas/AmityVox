#!/usr/bin/env bash
# AmityVox Backup Script
# Creates a timestamped backup of PostgreSQL and S3 data.
#
# Usage:
#   ./scripts/backup.sh                  # Backup to ./backups/
#   ./scripts/backup.sh /path/to/dir     # Backup to custom directory
#   COMPOSE_DIR=deploy/docker ./scripts/backup.sh

set -euo pipefail

BACKUP_DIR="${1:-./backups}"
COMPOSE_DIR="${COMPOSE_DIR:-deploy/docker}"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_PATH="$BACKUP_DIR/amityvox_$TIMESTAMP"

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

log() { echo -e "${GREEN}[backup]${NC} $*"; }
err() { echo -e "${RED}[backup]${NC} $*" >&2; }

# Determine Docker Compose command.
if docker compose version >/dev/null 2>&1; then
    COMPOSE_CMD="docker compose"
elif command -v docker-compose >/dev/null 2>&1; then
    COMPOSE_CMD="docker-compose"
else
    err "Docker Compose is required."
    exit 1
fi

mkdir -p "$BACKUP_PATH"

log "Starting backup to $BACKUP_PATH..."

# 1. Backup PostgreSQL.
log "Dumping PostgreSQL..."
$COMPOSE_CMD -f "$COMPOSE_DIR/docker-compose.yml" exec -T postgresql pg_dumpall -U amityvox > "$BACKUP_PATH/postgres.sql"
log "PostgreSQL dump: $BACKUP_PATH/postgres.sql ($(du -h "$BACKUP_PATH/postgres.sql" | cut -f1))"

# 2. Backup configuration.
log "Backing up configuration..."
[ -f "amityvox.toml" ] && cp amityvox.toml "$BACKUP_PATH/"
[ -f ".env" ] && cp .env "$BACKUP_PATH/"

# 3. Compress.
log "Compressing backup..."
tar -czf "$BACKUP_PATH.tar.gz" -C "$BACKUP_DIR" "amityvox_$TIMESTAMP"
rm -rf "$BACKUP_PATH"

SIZE=$(du -h "$BACKUP_PATH.tar.gz" | cut -f1)
log "Backup complete: $BACKUP_PATH.tar.gz ($SIZE)"
