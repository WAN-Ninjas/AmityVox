#!/usr/bin/env bash
# AmityVox Interactive Setup
# Usage: curl -fsSL https://raw.githubusercontent.com/WAN-Ninjas/AmityVox/main/install.sh | bash
#
# This script:
#   1. Checks prerequisites (Docker, Docker Compose, git, openssl)
#   2. Clones the repository
#   3. Walks you through configuration questions
#   4. Generates secure passwords and fills out .env
#   5. Builds and starts all services
#   6. Bootstraps Garage S3 storage (bucket + key)
#   7. Creates your admin account
#
# Non-interactive mode (for automation):
#   AMITYVOX_NONINTERACTIVE=1 \
#   AMITYVOX_DOMAIN=example.com \
#   AMITYVOX_NAME="My Server" \
#   AMITYVOX_ADMIN_USER=admin \
#   AMITYVOX_ADMIN_EMAIL=admin@example.com \
#   AMITYVOX_ADMIN_PASS=secretpassword \
#   curl -fsSL .../install.sh | bash

set -euo pipefail

# ============================================================
# Configuration & Defaults
# ============================================================
REPO_URL="${AMITYVOX_REPO:-https://github.com/WAN-Ninjas/AmityVox.git}"
INSTALL_DIR="${AMITYVOX_DIR:-$HOME/amityvox}"
BRANCH="${AMITYVOX_BRANCH:-main}"
COMPOSE_FILE="deploy/docker/docker-compose.yml"
NONINTERACTIVE="${AMITYVOX_NONINTERACTIVE:-0}"

# ============================================================
# Colors & Output Helpers
# ============================================================
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

log()  { echo -e "${GREEN}[AmityVox]${NC} $*"; }
warn() { echo -e "${YELLOW}[AmityVox]${NC} $*"; }
err()  { echo -e "${RED}[AmityVox]${NC} $*" >&2; }
info() { echo -e "${BLUE}[AmityVox]${NC} $*"; }
hr()   { echo -e "${CYAN}──────────────────────────────────────────────────${NC}"; }

# ============================================================
# Utility Functions
# ============================================================

# Generate a cryptographically random hex string.
gen_hex() {
    local len="${1:-32}"
    openssl rand -hex "$len" 2>/dev/null || head -c "$((len * 2))" /dev/urandom | od -An -tx1 | tr -d ' \n' | head -c "$((len * 2))"
}

# Generate a random alphanumeric string.
gen_alnum() {
    local len="${1:-32}"
    openssl rand -base64 "$((len * 2))" 2>/dev/null | tr -dc 'A-Za-z0-9' | head -c "$len" || \
        head -c "$((len * 2))" /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c "$len"
}

# Prompt with a default value. In non-interactive mode, use the default or env var.
ask() {
    local prompt="$1"
    local default="$2"
    local varname="${3:-}"

    # Check if an environment variable override is set.
    if [ -n "$varname" ] && [ -n "${!varname:-}" ]; then
        REPLY="${!varname}"
        return
    fi

    if [ "$NONINTERACTIVE" = "1" ]; then
        REPLY="$default"
        return
    fi

    echo -en "${BOLD}$prompt${NC}"
    if [ -n "$default" ]; then
        echo -en " ${CYAN}[$default]${NC}"
    fi
    echo -n ": "
    read -r REPLY
    REPLY="${REPLY:-$default}"
}

# Prompt for a yes/no question. Returns 0 for yes, 1 for no.
ask_yn() {
    local prompt="$1"
    local default="${2:-n}"

    if [ "$NONINTERACTIVE" = "1" ]; then
        [ "$default" = "y" ] && return 0 || return 1
    fi

    local hint="y/N"
    [ "$default" = "y" ] && hint="Y/n"

    echo -en "${BOLD}$prompt${NC} ${CYAN}[$hint]${NC}: "
    read -r REPLY
    REPLY="${REPLY:-$default}"
    [[ "$REPLY" =~ ^[Yy] ]]
}

# Prompt for a password (hidden input).
ask_pass() {
    local prompt="$1"
    local varname="${2:-}"

    # Check environment variable override.
    if [ -n "$varname" ] && [ -n "${!varname:-}" ]; then
        REPLY="${!varname}"
        return
    fi

    if [ "$NONINTERACTIVE" = "1" ]; then
        if [ -n "$varname" ] && [ -n "${!varname:-}" ]; then
            REPLY="${!varname}"
        else
            REPLY="$(gen_alnum 16)"
            warn "Generated random password (no $varname set): $REPLY"
        fi
        return
    fi

    while true; do
        echo -en "${BOLD}$prompt${NC}: "
        read -rs REPLY
        echo
        if [ ${#REPLY} -lt 8 ]; then
            warn "Password must be at least 8 characters. Try again."
            continue
        fi
        echo -en "${BOLD}Confirm password${NC}: "
        read -rs REPLY2
        echo
        if [ "$REPLY" != "$REPLY2" ]; then
            warn "Passwords don't match. Try again."
            continue
        fi
        break
    done
}

# Prompt for a choice from a numbered list.
ask_choice() {
    local prompt="$1"
    shift
    local options=("$@")
    local default=1

    if [ "$NONINTERACTIVE" = "1" ]; then
        REPLY="${options[0]}"
        return
    fi

    echo -e "${BOLD}$prompt${NC}"
    for i in "${!options[@]}"; do
        echo -e "  ${CYAN}$((i+1)))${NC} ${options[$i]}"
    done
    echo -n "Choice [1]: "
    read -r choice
    choice="${choice:-$default}"
    if [[ "$choice" =~ ^[0-9]+$ ]] && [ "$choice" -ge 1 ] && [ "$choice" -le ${#options[@]} ]; then
        REPLY="${options[$((choice-1))]}"
    else
        REPLY="${options[0]}"
    fi
}

# ============================================================
# Step 1: Banner & Prerequisite Check
# ============================================================
banner() {
    echo
    echo -e "${BOLD}${CYAN}"
    echo "    _              _ _        __     __        "
    echo "   / \\   _ __ ___ (_) |_ _   _\\ \\   / /__  _  _"
    echo "  / _ \\ | '_ \` _ \\| | __| | | |\\ \\ / / _ \\| \\/ |"
    echo " / ___ \\| | | | | | | |_| |_| | \\ V / (_) >  < "
    echo "/_/   \\_\\_| |_| |_|_|\\__|\\__, |  \\_/ \\___/_/\\_\\"
    echo "                         |___/                  "
    echo -e "${NC}"
    echo -e "  ${BOLD}Self-hosted, federated communication platform${NC}"
    echo -e "  ${CYAN}https://github.com/WAN-Ninjas/AmityVox${NC}"
    echo
    hr
}

check_prerequisites() {
    log "Checking prerequisites..."
    local missing=()

    command -v git    >/dev/null 2>&1 || missing+=(git)
    command -v docker >/dev/null 2>&1 || missing+=(docker)
    command -v openssl >/dev/null 2>&1 || missing+=("openssl (for generating secrets)")

    if [ ${#missing[@]} -gt 0 ]; then
        err "Missing required tools:"
        for tool in "${missing[@]}"; do
            err "  - $tool"
        done
        echo
        err "Install them and try again."
        err "Docker: https://docs.docker.com/engine/install/"
        exit 1
    fi

    # Check Docker Compose v2.
    if docker compose version >/dev/null 2>&1; then
        COMPOSE_CMD="docker compose"
    elif command -v docker-compose >/dev/null 2>&1; then
        COMPOSE_CMD="docker-compose"
    else
        err "Docker Compose v2 is required but not found."
        err "Install it: https://docs.docker.com/compose/install/"
        exit 1
    fi

    # Check Docker is running.
    if ! docker info >/dev/null 2>&1; then
        err "Docker is installed but not running. Start Docker and try again."
        exit 1
    fi

    log "All prerequisites satisfied."
}

# ============================================================
# Step 2: Clone Repository
# ============================================================
setup_repo() {
    if [ -d "$INSTALL_DIR/.git" ]; then
        log "Found existing installation at $INSTALL_DIR"
        if ask_yn "Update to the latest version?"; then
            cd "$INSTALL_DIR"
            git pull origin "$BRANCH" 2>/dev/null || warn "Could not pull latest changes (continuing with existing)"
        else
            cd "$INSTALL_DIR"
        fi
    else
        log "Cloning AmityVox to $INSTALL_DIR..."
        git clone --depth 1 -b "$BRANCH" "$REPO_URL" "$INSTALL_DIR"
        cd "$INSTALL_DIR"
    fi
}

# ============================================================
# Step 3: Interactive Configuration
# ============================================================
collect_config() {
    hr
    echo
    echo -e "${BOLD}Instance Settings${NC}"
    echo -e "These define your server's identity and public URL."
    echo

    # Domain
    ask "Domain name (e.g. chat.example.com, or localhost for testing)" "localhost" "AMITYVOX_DOMAIN"
    DOMAIN="$REPLY"

    # Instance name
    ask "Instance name (shown in the UI)" "AmityVox" "AMITYVOX_NAME"
    INSTANCE_NAME="$REPLY"

    # Instance description
    ask "Short description" "A self-hosted communication platform" "AMITYVOX_DESCRIPTION"
    INSTANCE_DESCRIPTION="$REPLY"

    echo
    hr
    echo
    echo -e "${BOLD}Registration & Access${NC}"
    echo

    # Registration mode
    ask_choice "Registration mode:" \
        "Open (anyone can register)" \
        "Invite-only (require an invite code)" \
        "Closed (admin creates accounts manually)"
    case "$REPLY" in
        "Open"*)       REG_ENABLED=true;  INVITE_ONLY=false ;;
        "Invite"*)     REG_ENABLED=true;  INVITE_ONLY=true  ;;
        "Closed"*)     REG_ENABLED=false; INVITE_ONLY=false ;;
    esac

    echo
    hr
    echo
    echo -e "${BOLD}Federation${NC}"
    echo -e "Federation lets users on other AmityVox instances interact with yours."
    echo

    ask_choice "Federation mode:" \
        "Disabled (standalone, no federation)" \
        "Closed (require key exchange to federate)" \
        "Open (anyone who knows your domain can federate)" \
        "Public (listed on the federation directory)"
    case "$REPLY" in
        "Disabled"*)  FEDERATION_MODE="disabled" ;;
        "Closed"*)    FEDERATION_MODE="closed"   ;;
        "Open"*)      FEDERATION_MODE="open"     ;;
        "Public"*)    FEDERATION_MODE="public"   ;;
    esac

    echo
    hr
    echo
    echo -e "${BOLD}Optional Features${NC}"
    echo

    # Giphy
    GIPHY_ENABLED=false
    GIPHY_API_KEY=""
    if ask_yn "Enable GIF search (Giphy)? Requires a free API key from https://developers.giphy.com/dashboard/" "n"; then
        GIPHY_ENABLED=true
        ask "Giphy API key" "" "AMITYVOX_GIPHY_KEY"
        GIPHY_API_KEY="$REPLY"
    fi

    # Push notifications
    VAPID_PUBLIC=""
    VAPID_PRIVATE=""
    VAPID_EMAIL=""
    if ask_yn "Enable push notifications? (Requires Node.js for one-time key generation)" "n"; then
        if command -v npx >/dev/null 2>&1; then
            info "Generating VAPID keys..."
            VAPID_OUTPUT=$(npx --yes web-push generate-vapid-keys 2>/dev/null || true)
            if [ -n "$VAPID_OUTPUT" ]; then
                VAPID_PUBLIC=$(echo "$VAPID_OUTPUT" | grep "Public Key:" | sed 's/.*Public Key: *//')
                VAPID_PRIVATE=$(echo "$VAPID_OUTPUT" | grep "Private Key:" | sed 's/.*Private Key: *//')
                ask "Contact email for push notifications" "admin@$DOMAIN" "AMITYVOX_PUSH_EMAIL"
                VAPID_EMAIL="$REPLY"
                log "VAPID keys generated."
            else
                warn "Could not generate VAPID keys. You can set them later in .env."
            fi
        else
            warn "npx not found. Install Node.js to generate VAPID keys, or set them manually in .env later."
        fi
    fi

    # Max upload size
    ask "Maximum file upload size" "50MB" "AMITYVOX_UPLOAD_SIZE"
    MAX_UPLOAD_SIZE="$REPLY"

    echo
    hr
    echo
    echo -e "${BOLD}Admin Account${NC}"
    echo -e "This will be your first user with full admin privileges."
    echo

    ask "Admin username" "admin" "AMITYVOX_ADMIN_USER"
    ADMIN_USER="$REPLY"

    ask "Admin email" "admin@$DOMAIN" "AMITYVOX_ADMIN_EMAIL"
    ADMIN_EMAIL="$REPLY"

    ask_pass "Admin password (min 8 characters)" "AMITYVOX_ADMIN_PASS"
    ADMIN_PASS="$REPLY"

    echo
    hr
}

# ============================================================
# Step 4: Generate Secrets & Write .env
# ============================================================
generate_config() {
    log "Generating secure configuration..."

    # Generate all secrets.
    POSTGRES_PASSWORD="$(gen_alnum 32)"
    MEILI_MASTER_KEY="$(gen_hex 16)"
    LIVEKIT_API_KEY="$(gen_alnum 12)"
    LIVEKIT_API_SECRET="$(gen_alnum 32)"
    GARAGE_RPC_SECRET="$(gen_hex 32)"

    # Determine LiveKit public URL.
    if [ "$DOMAIN" = "localhost" ]; then
        LIVEKIT_PUBLIC_URL="ws://localhost:7880"
    else
        LIVEKIT_PUBLIC_URL="wss://$DOMAIN"
    fi

    # Write .env file.
    cat > .env <<EOF
# AmityVox Configuration
# Generated by install.sh on $(date -u +%Y-%m-%dT%H:%M:%SZ)
# Documentation: https://github.com/WAN-Ninjas/AmityVox

# ============================================================
# Instance
# ============================================================
AMITYVOX_INSTANCE_DOMAIN=$DOMAIN
AMITYVOX_INSTANCE_NAME="$INSTANCE_NAME"
AMITYVOX_INSTANCE_DESCRIPTION="$INSTANCE_DESCRIPTION"
AMITYVOX_INSTANCE_FEDERATION_MODE=$FEDERATION_MODE

# ============================================================
# PostgreSQL
# ============================================================
POSTGRES_USER=amityvox
POSTGRES_PASSWORD=$POSTGRES_PASSWORD
POSTGRES_DB=amityvox

# ============================================================
# S3 Storage (Garage) — populated automatically after first boot
# ============================================================
AMITYVOX_STORAGE_ACCESS_KEY=
AMITYVOX_STORAGE_SECRET_KEY=
AMITYVOX_STORAGE_BUCKET=amityvox
GARAGE_RPC_SECRET=$GARAGE_RPC_SECRET

# ============================================================
# LiveKit (voice/video)
# ============================================================
LIVEKIT_API_KEY=$LIVEKIT_API_KEY
LIVEKIT_API_SECRET=$LIVEKIT_API_SECRET
AMITYVOX_LIVEKIT_PUBLIC_URL=$LIVEKIT_PUBLIC_URL

# ============================================================
# Meilisearch (full-text search)
# ============================================================
MEILI_MASTER_KEY=$MEILI_MASTER_KEY

# ============================================================
# Push Notifications
# ============================================================
AMITYVOX_PUSH_VAPID_PUBLIC_KEY=$VAPID_PUBLIC
AMITYVOX_PUSH_VAPID_PRIVATE_KEY=$VAPID_PRIVATE
AMITYVOX_PUSH_VAPID_CONTACT_EMAIL=$VAPID_EMAIL

# ============================================================
# Auth
# ============================================================
AMITYVOX_AUTH_REGISTRATION_ENABLED=$REG_ENABLED
AMITYVOX_AUTH_INVITE_ONLY=$INVITE_ONLY

# ============================================================
# Giphy (GIF search)
# ============================================================
AMITYVOX_GIPHY_ENABLED=$GIPHY_ENABLED
AMITYVOX_GIPHY_API_KEY=$GIPHY_API_KEY

# ============================================================
# Translation (LibreTranslate)
# ============================================================
LT_LOAD_ONLY=en,es,fr,de,it,pt,ru,ja,ko,zh,ar,hi,nl,pl,tr,uk
LIBRETRANSLATE_TAG=v1.8.4

# ============================================================
# Media
# ============================================================
AMITYVOX_MEDIA_MAX_UPLOAD_SIZE=$MAX_UPLOAD_SIZE

# ============================================================
# Logging
# ============================================================
AMITYVOX_LOGGING_LEVEL=info
AMITYVOX_LOGGING_FORMAT=json
EOF

    log "Configuration written to .env"
}

# ============================================================
# Step 5: Build & Start Services
# ============================================================
build_and_start() {
    log "Building AmityVox (this may take a few minutes on first run)..."
    echo

    $COMPOSE_CMD -f "$COMPOSE_FILE" build --no-cache 2>&1 | while IFS= read -r line; do
        # Show progress and errors without flooding the terminal.
        case "$line" in
            *"DONE"*|*"exporting"*|*"FINISHED"*|*"Successfully"*)
                echo -e "  ${GREEN}$line${NC}"
                ;;
            *"ERROR"*|*"error"*|*"FAILED"*|*"failed"*|*"CANCELED"*)
                echo -e "  ${RED}$line${NC}"
                ;;
        esac
    done

    echo
    log "Starting services..."
    $COMPOSE_CMD -f "$COMPOSE_FILE" up -d

    # Wait for the backend to become healthy.
    log "Waiting for AmityVox to become ready..."
    local attempts=0
    local max_attempts=60
    while [ $attempts -lt $max_attempts ]; do
        if docker exec amityvox wget -qO- http://localhost:8080/health >/dev/null 2>&1; then
            break
        fi
        attempts=$((attempts + 1))
        sleep 2
    done

    if [ $attempts -ge $max_attempts ]; then
        warn "AmityVox is taking longer than expected to start."
        warn "Check logs with: $COMPOSE_CMD -f $COMPOSE_FILE logs -f amityvox"
        warn "Continuing with setup — some steps may fail if services aren't ready."
    else
        log "AmityVox is running."
    fi
}

# ============================================================
# Step 6: Bootstrap Garage S3 Storage
# ============================================================
setup_garage() {
    log "Setting up S3 storage (Garage)..."

    # Wait for Garage to be ready.
    local attempts=0
    while [ $attempts -lt 30 ]; do
        if docker exec amityvox-garage /garage status >/dev/null 2>&1; then
            break
        fi
        attempts=$((attempts + 1))
        sleep 2
    done

    if [ $attempts -ge 30 ]; then
        warn "Garage is not ready yet. You'll need to set up S3 storage manually."
        warn "See the README for instructions."
        return 1
    fi

    # Get node ID.
    local node_id
    node_id=$(docker exec amityvox-garage /garage status 2>&1 | grep -oP '[a-f0-9]{64}' | head -1 || true)
    if [ -z "$node_id" ]; then
        warn "Could not determine Garage node ID. Manual S3 setup required."
        return 1
    fi

    # Assign layout and apply.
    docker exec amityvox-garage /garage layout assign -z dc1 -c 1G "$node_id" >/dev/null 2>&1 || true

    # Get current layout version and apply next.
    local layout_version
    layout_version=$(docker exec amityvox-garage /garage layout show 2>&1 | grep -oP 'version \K[0-9]+' | head -1 || echo "0")
    local next_version=$((layout_version + 1))
    docker exec amityvox-garage /garage layout apply --version "$next_version" >/dev/null 2>&1 || true

    # Create bucket.
    docker exec amityvox-garage /garage bucket create amityvox >/dev/null 2>&1 || true

    # Create key.
    docker exec amityvox-garage /garage key create amityvox-key >/dev/null 2>&1 || true

    # Allow key to access bucket.
    docker exec amityvox-garage /garage bucket allow amityvox --read --write --key amityvox-key >/dev/null 2>&1 || true

    # Extract key credentials.
    local key_info
    key_info=$(docker exec amityvox-garage /garage key info amityvox-key 2>&1)
    local access_key
    access_key=$(echo "$key_info" | grep -oP 'Key ID: \K\S+' || echo "")
    local secret_key
    secret_key=$(echo "$key_info" | grep -oP 'Secret key: \K\S+' || echo "")

    if [ -n "$access_key" ] && [ -n "$secret_key" ]; then
        # Update .env with the real credentials.
        sed -i "s|^AMITYVOX_STORAGE_ACCESS_KEY=.*|AMITYVOX_STORAGE_ACCESS_KEY=$access_key|" .env
        sed -i "s|^AMITYVOX_STORAGE_SECRET_KEY=.*|AMITYVOX_STORAGE_SECRET_KEY=$secret_key|" .env

        # Restart amityvox to pick up the new S3 credentials.
        $COMPOSE_CMD -f "$COMPOSE_FILE" restart amityvox >/dev/null 2>&1

        log "S3 storage configured (key: ${access_key:0:8}...)"
    else
        warn "Could not extract Garage credentials. Check manually with:"
        warn "  docker exec amityvox-garage /garage key info amityvox-key"
        return 1
    fi
}

# ============================================================
# Step 7: Create Admin Account
# ============================================================
create_admin() {
    log "Creating admin account..."

    # Wait for backend to be ready after the S3 restart.
    sleep 5
    local attempts=0
    while [ $attempts -lt 30 ]; do
        if docker exec amityvox wget -qO- http://localhost:8080/health >/dev/null 2>&1; then
            break
        fi
        attempts=$((attempts + 1))
        sleep 2
    done

    if docker exec amityvox amityvox admin create-user "$ADMIN_USER" "$ADMIN_EMAIL" "$ADMIN_PASS" >/dev/null 2>&1; then
        docker exec amityvox amityvox admin set-admin "$ADMIN_USER" >/dev/null 2>&1
        log "Admin account created: $ADMIN_USER ($ADMIN_EMAIL)"
    else
        warn "Could not create admin account (user may already exist)."
        warn "Create one manually with:"
        warn "  docker exec amityvox amityvox admin create-user <user> <email> <password>"
        warn "  docker exec amityvox amityvox admin set-admin <user>"
    fi
}

# ============================================================
# Step 8: Print Summary
# ============================================================
print_summary() {
    echo
    hr
    echo
    echo -e "${GREEN}${BOLD}AmityVox is up and running!${NC}"
    echo

    if [ "$DOMAIN" = "localhost" ]; then
        echo -e "  ${BOLD}Open in browser:${NC}  http://localhost"
    else
        echo -e "  ${BOLD}Open in browser:${NC}  https://$DOMAIN"
    fi
    echo -e "  ${BOLD}Admin account:${NC}   $ADMIN_USER / $ADMIN_EMAIL"
    echo

    echo -e "  ${BOLD}Useful commands:${NC}"
    echo -e "    View logs:     cd $INSTALL_DIR && $COMPOSE_CMD -f $COMPOSE_FILE logs -f"
    echo -e "    Stop:          cd $INSTALL_DIR && $COMPOSE_CMD -f $COMPOSE_FILE down"
    echo -e "    Start:         cd $INSTALL_DIR && $COMPOSE_CMD -f $COMPOSE_FILE up -d"
    echo -e "    Update:        cd $INSTALL_DIR && git pull && $COMPOSE_CMD -f $COMPOSE_FILE build --no-cache amityvox web-init && $COMPOSE_CMD -f $COMPOSE_FILE up -d amityvox web-init && $COMPOSE_CMD -f $COMPOSE_FILE restart caddy"
    echo -e "    Backup:        cd $INSTALL_DIR && ./scripts/backup.sh"
    echo -e "    Create user:   docker exec amityvox amityvox admin create-user <user> <email> <pass>"
    echo

    echo -e "  ${BOLD}Configuration:${NC}   $INSTALL_DIR/.env"
    echo -e "  ${BOLD}Documentation:${NC}   https://github.com/WAN-Ninjas/AmityVox"
    echo

    if [ "$DOMAIN" != "localhost" ]; then
        info "Caddy will automatically provision a TLS certificate for $DOMAIN."
        info "Make sure ports 80 and 443 are open and DNS points to this server."
        echo
    fi

    if [ "$GIPHY_ENABLED" = "true" ] && [ -z "$GIPHY_API_KEY" ]; then
        warn "Giphy is enabled but no API key was set. Get one at https://developers.giphy.com/dashboard/"
        warn "Then add it to .env as AMITYVOX_GIPHY_API_KEY and restart."
        echo
    fi

    if [ -z "$VAPID_PUBLIC" ]; then
        info "Push notifications are not configured. To enable them later:"
        info "  npx web-push generate-vapid-keys"
        info "  Then add the keys to .env and restart."
        echo
    fi

    echo -e "${CYAN}Questions or feedback? Visit https://amityvox.chat/ or https://discord.gg/VvxgUpF3uQ${NC}"
    echo
}

# ============================================================
# Main
# ============================================================
main() {
    banner
    check_prerequisites
    setup_repo

    # Skip interactive config if .env already exists and user doesn't want to overwrite.
    if [ -f ".env" ]; then
        if ask_yn "An existing .env was found. Reconfigure from scratch?"; then
            collect_config
            generate_config
        else
            log "Keeping existing .env configuration."
            # Still need admin credentials for account creation.
            echo
            echo -e "${BOLD}Admin Account${NC}"
            ask "Admin username (skip with Enter to skip account creation)" "" "AMITYVOX_ADMIN_USER"
            ADMIN_USER="$REPLY"
            if [ -n "$ADMIN_USER" ]; then
                ask "Admin email" "admin@localhost" "AMITYVOX_ADMIN_EMAIL"
                ADMIN_EMAIL="$REPLY"
                ask_pass "Admin password (min 8 characters)" "AMITYVOX_ADMIN_PASS"
                ADMIN_PASS="$REPLY"
            fi
            # Read domain from existing .env for summary.
            DOMAIN=$(grep -oP '^AMITYVOX_INSTANCE_DOMAIN=\K.*' .env 2>/dev/null || echo "localhost")
        fi
    else
        collect_config
        generate_config
    fi

    echo
    build_and_start
    setup_garage || true
    if [ -n "${ADMIN_USER:-}" ]; then
        create_admin
    fi
    print_summary
}

main "$@"
