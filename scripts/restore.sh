#!/usr/bin/env bash
# AmityVox Restore Script
# Restores a backup created by backup.sh.
#
# Usage:
#   ./scripts/restore.sh ./backups/amityvox_20260211_120000.tar.gz

set -euo pipefail

if [ $# -lt 1 ]; then
    echo "Usage: $0 <backup-file.tar.gz>"
    exit 1
fi

BACKUP_FILE="$1"
COMPOSE_DIR="${COMPOSE_DIR:-deploy/docker}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log()  { echo -e "${GREEN}[restore]${NC} $*"; }
warn() { echo -e "${YELLOW}[restore]${NC} $*"; }
err()  { echo -e "${RED}[restore]${NC} $*" >&2; }

if [ ! -f "$BACKUP_FILE" ]; then
    err "Backup file not found: $BACKUP_FILE"
    exit 1
fi

# Determine Docker Compose command.
if docker compose version >/dev/null 2>&1; then
    COMPOSE_CMD="docker compose"
elif command -v docker-compose >/dev/null 2>&1; then
    COMPOSE_CMD="docker-compose"
else
    err "Docker Compose is required."
    exit 1
fi

warn "This will OVERWRITE the current database. Are you sure? (y/N)"
read -r confirm
if [ "$confirm" != "y" ] && [ "$confirm" != "Y" ]; then
    log "Restore cancelled."
    exit 0
fi

# Extract backup.
TEMP_DIR=$(mktemp -d)
log "Extracting backup..."
tar -xzf "$BACKUP_FILE" -C "$TEMP_DIR"
BACKUP_DIR=$(ls "$TEMP_DIR")

# Restore PostgreSQL.
if [ -f "$TEMP_DIR/$BACKUP_DIR/postgres.sql" ]; then
    log "Restoring PostgreSQL..."
    cd "$COMPOSE_DIR"
    $COMPOSE_CMD exec -T postgresql psql -U amityvox < "../../$TEMP_DIR/$BACKUP_DIR/postgres.sql"
    cd ../..
    log "PostgreSQL restored."
else
    warn "No PostgreSQL dump found in backup."
fi

# Restore config if present and user wants it.
if [ -f "$TEMP_DIR/$BACKUP_DIR/amityvox.toml" ]; then
    warn "Backup contains amityvox.toml. Restore it? (y/N)"
    read -r confirm_config
    if [ "$confirm_config" = "y" ] || [ "$confirm_config" = "Y" ]; then
        cp "$TEMP_DIR/$BACKUP_DIR/amityvox.toml" .
        log "Configuration restored."
    fi
fi

# Cleanup.
rm -rf "$TEMP_DIR"

log "Restore complete. Restart services with: cd $COMPOSE_DIR && $COMPOSE_CMD restart"
