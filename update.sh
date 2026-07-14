#!/usr/bin/env bash
# AmityVox safe update script.
#
# Updates an existing Docker Compose installation without deleting .env or
# Docker volumes. The script creates a pre-update backup, pulls the selected
# branch with fast-forward only, rebuilds the app/frontend init services, and
# recreates containers with docker compose up -d.
#
# Usage:
#   ./update.sh
#   AMITYVOX_BRANCH=main ./update.sh
#   AMITYVOX_SKIP_BACKUP=1 ./update.sh
#   curl -fsSL https://raw.githubusercontent.com/WAN-Ninjas/AmityVox/main/update.sh | bash

set -Eeuo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

REPO_URL="${AMITYVOX_REPO:-https://github.com/WAN-Ninjas/AmityVox.git}"
INSTALL_DIR="${AMITYVOX_DIR:-$HOME/amityvox}"
BRANCH="${AMITYVOX_BRANCH:-main}"
COMPOSE_FILE="deploy/docker/docker-compose.yml"
COMPOSE_OVERRIDE=""
NONINTERACTIVE="${AMITYVOX_NONINTERACTIVE:-0}"
SKIP_BACKUP="${AMITYVOX_SKIP_BACKUP:-0}"
BACKUP_ROOT="${AMITYVOX_UPDATE_BACKUP_DIR:-./backups}"
NO_PULL="${AMITYVOX_NO_PULL:-0}"
ALLOW_DIRTY="${AMITYVOX_ALLOW_DIRTY:-0}"
NO_CACHE="${AMITYVOX_NO_CACHE:-0}"

COMPOSE_CMD=""
BACKUP_PATH=""
PREVIOUS_COMMIT=""
TARGET_DIR=""

log() { echo -e "${GREEN}[update]${NC} $*"; }
info() { echo -e "${BLUE}[info]${NC} $*"; }
warn() { echo -e "${YELLOW}[warn]${NC} $*" >&2; }
err() { echo -e "${RED}[error]${NC} $*" >&2; }
hr() { echo -e "${BLUE}----------------------------------------${NC}"; }

usage() {
    cat <<EOF
AmityVox safe update script

Usage:
  ./update.sh [options]

Options:
  -h, --help      Show this help text

Environment:
  AMITYVOX_DIR                Existing install directory when running remotely
                              (default: $HOME/amityvox)
  AMITYVOX_BRANCH             Git branch to update from (default: main)
  AMITYVOX_REPO               Git remote used for piped installs
  AMITYVOX_SKIP_BACKUP=1      Skip pre-update database/config backup
  AMITYVOX_UPDATE_BACKUP_DIR  Backup root directory (default: ./backups)
  AMITYVOX_NO_PULL=1          Skip git fetch/merge, rebuild current checkout
  AMITYVOX_ALLOW_DIRTY=1      Continue with local tracked file changes
  AMITYVOX_NO_CACHE=1         Build app images with --no-cache
  AMITYVOX_NONINTERACTIVE=1   Fail instead of prompting

This script never runs docker compose down, docker compose down -v, or deletes
Docker volumes. Existing data remains in the named volumes managed by Compose.
EOF
}

ask_yn() {
    local prompt="$1"
    local default="${2:-y}"
    local suffix="[Y/n]"

    if [ "$default" = "n" ]; then
        suffix="[y/N]"
    fi

    if [ "$NONINTERACTIVE" = "1" ]; then
        [ "$default" = "y" ]
        return $?
    fi

    local answer
    read -r -p "$(echo -e "${YELLOW}?${NC} $prompt $suffix ")" answer
    answer="${answer:-$default}"
    [[ "$answer" =~ ^[Yy]$ ]]
}

run_verbose() {
    echo -e "${BLUE}$ $*${NC}"
    "$@"
}

compose() {
    local args=("-f" "$COMPOSE_FILE")
    if [ -n "$COMPOSE_OVERRIDE" ]; then
        args+=("-f" "$COMPOSE_OVERRIDE")
    fi
    if [ -f ".env" ]; then
        $COMPOSE_CMD --env-file .env "${args[@]}" "$@"
    else
        $COMPOSE_CMD "${args[@]}" "$@"
    fi
}

compose_display() {
    local extra=""
    if [ -n "$COMPOSE_OVERRIDE" ]; then
        extra=" -f $COMPOSE_OVERRIDE"
    fi
    if [ -f ".env" ]; then
        echo "$COMPOSE_CMD --env-file .env -f $COMPOSE_FILE$extra"
    else
        echo "$COMPOSE_CMD -f $COMPOSE_FILE$extra"
    fi
}

detect_compose() {
    if docker compose version >/dev/null 2>&1; then
        COMPOSE_CMD="docker compose"
    elif command -v docker-compose >/dev/null 2>&1; then
        COMPOSE_CMD="docker-compose"
    else
        err "Docker Compose is required."
        exit 1
    fi
}

resolve_target_dir() {
    local source_path="${BASH_SOURCE[0]}"
    local script_dir=""

    if [ "$source_path" != "bash" ] && [ "$source_path" != "-" ]; then
        script_dir="$(cd "$(dirname "$source_path")" && pwd)"
        if [ -f "$script_dir/$COMPOSE_FILE" ]; then
            TARGET_DIR="$script_dir"
            return
        fi
    fi

    if [ -f "$PWD/$COMPOSE_FILE" ]; then
        TARGET_DIR="$PWD"
        return
    fi

    if [ -f "$INSTALL_DIR/$COMPOSE_FILE" ]; then
        TARGET_DIR="$INSTALL_DIR"
        return
    fi

    err "Could not find an existing AmityVox install."
    err "Run from the repo root, or set AMITYVOX_DIR=/path/to/amityvox."
    exit 1
}

preflight() {
    command -v git >/dev/null 2>&1 || { err "git is required."; exit 1; }
    command -v docker >/dev/null 2>&1 || { err "docker is required."; exit 1; }

    detect_compose

    if ! docker info >/dev/null 2>&1; then
        err "Docker is not running or this user cannot access it."
        exit 1
    fi

    if [ ! -d ".git" ]; then
        err "This update script must run from a Git checkout."
        exit 1
    fi

    if [ ! -f ".env" ]; then
        err "No .env file found. This script updates existing installs only."
        err "Run install.sh first, then use update.sh for future updates."
        exit 1
    fi

    if [ ! -f "$COMPOSE_FILE" ]; then
        err "Missing Compose file: $COMPOSE_FILE"
        exit 1
    fi

    local tracked_changes
    tracked_changes="$(git status --porcelain --untracked-files=no)"
    if [ -n "$tracked_changes" ] && [ "$ALLOW_DIRTY" != "1" ]; then
        warn "Tracked local changes are present:"
        echo "$tracked_changes" >&2
        if ! ask_yn "Continue without touching those files?" "n"; then
            err "Aborting. Commit, stash, or set AMITYVOX_ALLOW_DIRTY=1 to continue."
            exit 1
        fi
    fi

    PREVIOUS_COMMIT="$(git rev-parse HEAD)"

    # Detect proxy mode from .env and set compose override accordingly.
    local proxy_mode
    proxy_mode="$(sed -n 's/^AMITYVOX_PROXY_MODE=//p' .env | head -1)"
    if [ "$proxy_mode" = "external" ]; then
        if [ -f "deploy/docker/docker-compose.external-proxy.yml" ]; then
            COMPOSE_OVERRIDE="deploy/docker/docker-compose.external-proxy.yml"
            log "External proxy mode detected — using compose override."
        else
            warn "AMITYVOX_PROXY_MODE=external but override file is missing."
            warn "Falling back to default compose (Caddy mode)."
        fi
    fi
}

create_backup() {
    local timestamp
    timestamp="$(date +%Y%m%d_%H%M%S)"
    BACKUP_PATH="$BACKUP_ROOT/update_$timestamp"

    mkdir -p "$BACKUP_PATH/config"

    log "Saving pre-update metadata to $BACKUP_PATH"
    printf '%s\n' "$PREVIOUS_COMMIT" > "$BACKUP_PATH/commit-before.txt"
    compose ps > "$BACKUP_PATH/compose-ps-before.txt" || true

    [ -f ".env" ] && cp .env "$BACKUP_PATH/config/.env"
    [ -f "amityvox.toml" ] && cp amityvox.toml "$BACKUP_PATH/config/"
    [ -f "deploy/caddy/Caddyfile" ] && cp deploy/caddy/Caddyfile "$BACKUP_PATH/config/Caddyfile"
    [ -d "deploy/caddy/custom" ] && cp -a deploy/caddy/custom "$BACKUP_PATH/config/caddy-custom"
    [ -f "deploy/garage/garage.toml" ] && cp deploy/garage/garage.toml "$BACKUP_PATH/config/garage.toml"
    [ -f "deploy/livekit/livekit.yaml" ] && cp deploy/livekit/livekit.yaml "$BACKUP_PATH/config/livekit.yaml"

    if [ "$SKIP_BACKUP" = "1" ]; then
        warn "Skipping database backup because AMITYVOX_SKIP_BACKUP=1."
        return
    fi

    if [ -x "scripts/backup.sh" ]; then
        log "Creating database/config backup before update..."
        COMPOSE_DIR=deploy/docker run_verbose ./scripts/backup.sh "$BACKUP_PATH"
    else
        warn "scripts/backup.sh is not executable; metadata/config snapshot only."
    fi
}

pull_latest() {
    if [ "$NO_PULL" = "1" ]; then
        warn "Skipping git pull because AMITYVOX_NO_PULL=1."
        return
    fi

    log "Fetching latest $BRANCH from origin..."

    if ! git remote get-url origin >/dev/null 2>&1; then
        git remote add origin "$REPO_URL"
    fi

    run_verbose git fetch origin "$BRANCH"

    log "Applying update with fast-forward only..."
    if ! run_verbose git merge --ff-only "origin/$BRANCH"; then
        err "Could not fast-forward to origin/$BRANCH."
        err "No containers were changed. Resolve local divergence, then rerun update.sh."
        exit 1
    fi
}

update_services() {
    local build_args=()
    if [ "$NO_CACHE" = "1" ]; then
        build_args+=(--no-cache)
    fi

    log "Building updated application images..."
    if [ "${#build_args[@]}" -gt 0 ]; then
        run_verbose compose build "${build_args[@]}" amityvox web-init
    else
        run_verbose compose build amityvox web-init
    fi

    log "Recreating changed services without removing volumes..."
    run_verbose compose up -d

    if [ -z "$COMPOSE_OVERRIDE" ]; then
        log "Refreshing Caddy so updated web assets are served..."
        run_verbose compose restart caddy
    else
        log "External proxy mode — skipping Caddy restart."
    fi
}

health_check() {
    local container="amityvox"
    local status=""
    local running=""

    log "Waiting for AmityVox to be healthy..."
    for _ in $(seq 1 60); do
        status="$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{end}}' "$container" 2>/dev/null || true)"
        running="$(docker inspect -f '{{.State.Running}}' "$container" 2>/dev/null || true)"

        if [ "$status" = "healthy" ] || { [ -z "$status" ] && [ "$running" = "true" ]; }; then
            log "AmityVox is running."
            return
        fi

        if [ "$status" = "unhealthy" ]; then
            warn "Container reported unhealthy. Continuing to collect status."
            break
        fi

        sleep 2
    done

    warn "AmityVox did not report healthy within the wait window."
    warn "Check logs with: cd $TARGET_DIR && $(compose_display) logs -f amityvox"
}

on_error() {
    local line_no="$1"
    local exit_code="$2"
    err "Update failed at line $line_no (exit code $exit_code)."
    if [ -n "$BACKUP_PATH" ]; then
        err "Pre-update backup/metadata: $BACKUP_PATH"
    fi
    if [ -n "$PREVIOUS_COMMIT" ]; then
        err "Manual rollback outline:"
        err "  cd $TARGET_DIR"
        err "  git checkout $PREVIOUS_COMMIT"
        err "  $(compose_display) build amityvox web-init"
        err "  $(compose_display) up -d"
        if [ -z "$COMPOSE_OVERRIDE" ]; then
            err "  $(compose_display) restart caddy"
        fi
    fi
}

main() {
    if [ "${1:-}" = "-h" ] || [ "${1:-}" = "--help" ]; then
        usage
        exit 0
    fi

    resolve_target_dir
    cd "$TARGET_DIR"

    trap 'on_error $LINENO $?' ERR

    echo
    hr
    echo -e "${GREEN}${BOLD}AmityVox Safe Update${NC}"
    hr
    echo

    preflight
    create_backup
    pull_latest
    update_services
    health_check

    echo
    hr
    log "Update complete."
    info "Previous commit: $PREVIOUS_COMMIT"
    info "Current commit:  $(git rev-parse HEAD)"
    info "Backup path:     $BACKUP_PATH"
    info "Status command:  cd $TARGET_DIR && $(compose_display) ps"
    hr
}

main "$@"
