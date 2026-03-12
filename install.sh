#!/usr/bin/env bash
# AmityVox Interactive Setup
# Usage: curl -fsSL https://raw.githubusercontent.com/WAN-Ninjas/AmityVox/main/install.sh | bash
#   or:  ./install.sh              (from the repo directory)
#   or:  /path/to/install.sh       (from anywhere — operates in the script's directory)
#
# This script:
#   1. Detects your OS and architecture
#   2. Installs Docker if missing (Debian, Ubuntu, Raspberry Pi OS, Armbian)
#   3. Fixes permissions if needed (docker group, systemd)
#   4. Clones the repository (if run via curl) or uses the existing checkout
#   5. Walks you through configuration questions
#   6. Generates secure passwords and fills out .env
#   7. Builds and starts all services
#   8. Bootstraps Garage S3 storage (bucket + key)
#   9. Creates your admin account
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
# Resolve Script Directory
# ============================================================
# If run from a local file (not piped), operate from the script's own directory.
# This ensures the script works regardless of the caller's CWD.
SCRIPT_IS_PIPED=false
if [ -n "${BASH_SOURCE[0]:-}" ] && [ "${BASH_SOURCE[0]}" != "-" ] && [ -f "${BASH_SOURCE[0]}" ]; then
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
else
    SCRIPT_IS_PIPED=true
    SCRIPT_DIR=""
fi

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
# Disable colors if stdout is not a terminal.
if [ -t 1 ]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[1;33m'
    BLUE='\033[0;34m'
    CYAN='\033[0;36m'
    BOLD='\033[1m'
    DIM='\033[2m'
    NC='\033[0m'
else
    RED='' GREEN='' YELLOW='' BLUE='' CYAN='' BOLD='' DIM='' NC=''
fi

log()     { echo -e "${GREEN}[AmityVox]${NC} $*"; }
warn()    { echo -e "${YELLOW}[WARNING]${NC} $*"; }
err()     { echo -e "${RED}[ERROR]${NC}   $*" >&2; }
info()    { echo -e "${BLUE}[INFO]${NC}    $*"; }
debug()   { echo -e "${DIM}[DEBUG]${NC}   $*"; }
hr()      { echo -e "${CYAN}──────────────────────────────────────────────────${NC}"; }

# Print a command before running it so the user sees exactly what happens.
run_verbose() {
    echo -e "  ${DIM}\$ $*${NC}"
    "$@"
}

# ============================================================
# Error Trap — verbose diagnostics on failure
# ============================================================
on_error() {
    local exit_code=$?
    local line_no=${1:-unknown}
    echo
    err "Installation failed at line $line_no (exit code $exit_code)."
    err ""
    err "Diagnostic information:"
    err "  OS:           $(uname -srm)"
    err "  Shell:        $BASH_VERSION"
    err "  User:         $(whoami)"
    err "  Working dir:  $(pwd)"
    err "  Docker:       $(docker --version 2>/dev/null || echo 'not installed')"
    err "  Compose:      $(docker compose version 2>/dev/null || echo 'not available')"
    err "  Disk free:    $(df -h . 2>/dev/null | tail -1 | awk '{print $4}' || echo 'unknown')"
    err "  Memory free:  $(free -h 2>/dev/null | awk '/^Mem:/{print $7}' || echo 'unknown')"
    err ""
    err "If services were partially started, check logs with:"
    err "  docker compose -f deploy/docker/docker-compose.yml logs --tail=50"
    err ""
    err "For help, visit: https://github.com/WAN-Ninjas/AmityVox/issues"
    exit "$exit_code"
}
trap 'on_error $LINENO' ERR

# ============================================================
# OS Detection
# ============================================================
detect_os() {
    ARCH="$(uname -m)"
    case "$ARCH" in
        x86_64)  DOCKER_ARCH="amd64" ;;
        aarch64) DOCKER_ARCH="arm64" ;;
        armv7l)  DOCKER_ARCH="armhf" ;;
        *)       DOCKER_ARCH="$ARCH" ;;
    esac

    DISTRO_ID=""
    DISTRO_VERSION=""
    DISTRO_CODENAME=""
    DISTRO_LABEL="unknown"
    IS_RASPBERRY_PI=false

    if [ -f /etc/os-release ]; then
        # shellcheck source=/dev/null
        . /etc/os-release
        DISTRO_ID="${ID:-unknown}"
        DISTRO_VERSION="${VERSION_ID:-}"
        DISTRO_CODENAME="${VERSION_CODENAME:-}"
        DISTRO_LABEL="${PRETTY_NAME:-$DISTRO_ID $DISTRO_VERSION}"
    fi

    # Detect Raspberry Pi (RPi OS reports as "debian" with Raspberry Pi model).
    if [ -f /proc/device-tree/model ] && grep -qi "raspberry" /proc/device-tree/model 2>/dev/null; then
        IS_RASPBERRY_PI=true
    fi

    # Armbian reports as its upstream (debian/ubuntu) but sets ID=armbian or
    # includes "armbian" in ID_LIKE.
    IS_ARMBIAN=false
    if [ "$DISTRO_ID" = "armbian" ] || echo "${ID_LIKE:-}" | grep -qi armbian; then
        IS_ARMBIAN=true
    fi

    info "Detected: $DISTRO_LABEL ($ARCH)"
    $IS_RASPBERRY_PI && info "  Hardware: Raspberry Pi"
    $IS_ARMBIAN && info "  Hardware: Armbian SBC"
}

# ============================================================
# Sudo Helper — always prompts before elevating
# ============================================================
# Wraps sudo with an explanation of WHY and WHAT will be run.
run_sudo() {
    local reason="$1"
    shift

    if [ "$(id -u)" -eq 0 ]; then
        # Already root — just run it.
        "$@"
        return
    fi

    echo >/dev/tty
    echo -e "${BLUE}[INFO]${NC}    Elevated privileges needed: $reason" >/dev/tty
    echo -e "  ${DIM}Command: sudo $*${NC}" >/dev/tty
    echo >/dev/tty

    if [ "$NONINTERACTIVE" != "1" ]; then
        echo -en "${BOLD}Run this command with sudo?${NC} ${CYAN}[Y/n]${NC}: " >/dev/tty
        read -r confirm < /dev/tty
        confirm="${confirm:-y}"
        if [[ ! "$confirm" =~ ^[Yy] ]]; then
            warn "Skipped. You may need to run this manually:"
            warn "  sudo $*"
            return 1
        fi
    fi

    sudo "$@"
}

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
    read -r REPLY < /dev/tty
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
    read -r REPLY < /dev/tty
    REPLY="${REPLY:-$default}"
    [[ "$REPLY" =~ ^[Yy] ]]
}

# Prompt for a password (hidden input).
ask_pass() {
    local prompt="$1"
    local varname="${2:-}"

    if [ -n "$varname" ] && [ -n "${!varname:-}" ]; then
        REPLY="${!varname}"
        return
    fi

    if [ "$NONINTERACTIVE" = "1" ]; then
        REPLY="$(gen_alnum 16)"
        warn "Generated random password (no $varname set): $REPLY"
        return
    fi

    while true; do
        echo -en "${BOLD}$prompt${NC}: "
        read -rs REPLY < /dev/tty
        echo
        if [ ${#REPLY} -lt 8 ]; then
            warn "Password must be at least 8 characters. Try again."
            continue
        fi
        echo -en "${BOLD}Confirm password${NC}: "
        read -rs REPLY2 < /dev/tty
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
    read -r choice < /dev/tty
    choice="${choice:-$default}"
    if [[ "$choice" =~ ^[0-9]+$ ]] && [ "$choice" -ge 1 ] && [ "$choice" -le ${#options[@]} ]; then
        REPLY="${options[$((choice-1))]}"
    else
        REPLY="${options[0]}"
    fi
}

# ============================================================
# Step 1: Banner
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

# ============================================================
# Step 2: Install Docker if missing
# ============================================================
install_docker() {
    # Determine the upstream repo distro (armbian/rpios use debian/ubuntu repos).
    local repo_distro="$DISTRO_ID"
    local repo_codename="$DISTRO_CODENAME"

    # Armbian and Raspberry Pi OS are Debian or Ubuntu derivatives.
    case "$DISTRO_ID" in
        armbian)
            # Armbian's ID_LIKE tells us the upstream.
            if echo "${ID_LIKE:-}" | grep -qi ubuntu; then
                repo_distro="ubuntu"
            else
                repo_distro="debian"
            fi
            ;;
        raspbian)
            repo_distro="debian"
            ;;
    esac

    # Validate we're on a supported distro.
    case "$repo_distro" in
        debian|ubuntu) ;;
        *)
            err "Automatic Docker installation is only supported on Debian, Ubuntu,"
            err "Raspberry Pi OS, and Armbian."
            err ""
            err "Your distro: $DISTRO_LABEL ($DISTRO_ID)"
            err ""
            err "Install Docker manually: https://docs.docker.com/engine/install/"
            err "Then re-run this script."
            return 1
            ;;
    esac

    if [ -z "$repo_codename" ]; then
        err "Could not determine distribution codename from /etc/os-release."
        err "VERSION_CODENAME is empty. Install Docker manually:"
        err "  https://docs.docker.com/engine/install/"
        return 1
    fi

    echo
    log "Docker is not installed. This script can install it for you."
    info "This will:"
    info "  1. Install prerequisite packages (ca-certificates, curl, gnupg)"
    info "  2. Add Docker's official GPG key and APT repository"
    info "  3. Install docker-ce, docker-ce-cli, containerd, and compose plugin"
    info ""
    info "Repository: https://download.docker.com/linux/$repo_distro"
    info "Codename:   $repo_codename"
    echo

    if ! ask_yn "Install Docker now?" "y"; then
        err "Docker is required. Install it manually and re-run this script:"
        err "  https://docs.docker.com/engine/install/"
        return 1
    fi

    log "Installing Docker from official repository..."

    # Step 1: Install prerequisites.
    run_sudo "install prerequisite packages for Docker's APT repository" \
        apt-get update -qq

    run_sudo "install ca-certificates, curl, and gnupg" \
        apt-get install -y -qq ca-certificates curl gnupg >/dev/null

    # Step 2: Add Docker's GPG key.
    run_sudo "create keyrings directory" \
        install -m 0755 -d /etc/apt/keyrings

    info "Downloading Docker GPG key..."
    curl -fsSL "https://download.docker.com/linux/$repo_distro/gpg" | \
        run_sudo "add Docker's official GPG signing key" \
            tee /etc/apt/keyrings/docker.asc >/dev/null

    run_sudo "set GPG key permissions" \
        chmod a+r /etc/apt/keyrings/docker.asc

    # Step 3: Add Docker APT repository.
    local repo_line="deb [arch=$DOCKER_ARCH signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/$repo_distro $repo_codename stable"
    echo "$repo_line" | run_sudo "add Docker APT repository" \
        tee /etc/apt/sources.list.d/docker.list >/dev/null

    info "Added repository: $repo_line"

    # Step 4: Install Docker packages.
    run_sudo "update package lists with Docker repository" \
        apt-get update -qq

    run_sudo "install Docker Engine, CLI, containerd, and Compose plugin" \
        apt-get install -y -qq docker-ce docker-ce-cli containerd.io \
            docker-buildx-plugin docker-compose-plugin >/dev/null

    # Step 5: Start and enable Docker.
    run_sudo "start Docker service" \
        systemctl start docker

    run_sudo "enable Docker to start on boot" \
        systemctl enable docker >/dev/null 2>&1

    log "Docker installed successfully: $(docker --version 2>/dev/null || echo 'unknown version')"
}

# ============================================================
# Step 3: Check & fix Docker permissions
# ============================================================
fix_docker_permissions() {
    # Already root — no permission issues.
    if [ "$(id -u)" -eq 0 ]; then
        return 0
    fi

    # Check if the current user can talk to the Docker socket.
    if docker info >/dev/null 2>&1; then
        return 0
    fi

    # Docker is installed but the user can't access it.
    local current_user
    current_user="$(whoami)"

    echo
    warn "Your user ($current_user) does not have permission to use Docker."
    info ""
    info "This usually means your user is not in the 'docker' group."
    info "Without this, every Docker command would require 'sudo'."
    info ""
    info "The fix is:"
    info "  1. Add $current_user to the 'docker' group"
    info "  2. Log out and back in (or use 'newgrp docker')"
    echo

    if ! ask_yn "Add $current_user to the docker group now?"; then
        err "Cannot continue without Docker access."
        err "Fix manually: sudo usermod -aG docker $current_user"
        err "Then log out and back in, and re-run this script."
        return 1
    fi

    run_sudo "add $current_user to the docker group" \
        usermod -aG docker "$current_user"

    log "Added $current_user to the docker group."

    # Re-exec the entire script under the docker group so all docker/compose/exec
    # calls work without individual wrappers.
    if [ "${AMITYVOX_SG_REEXEC:-}" != "1" ]; then
        info "Re-launching installer under the docker group..."
        export AMITYVOX_SG_REEXEC=1
        exec sg docker -c "bash $0 $*" || true
    fi

    # If re-exec failed or we're already in the re-exec, check again.
    if ! docker info >/dev/null 2>&1; then
        echo
        warn "The group change requires a new login session to take effect."
        warn ""
        warn "Please do one of the following:"
        warn "  1. Log out and log back in, then re-run this script"
        warn "  2. Run: newgrp docker && bash $0"
        warn "  3. Run this script with sudo (not recommended for daily use)"
        return 1
    fi
}

# ============================================================
# Step 4: Prerequisites Check
# ============================================================
check_prerequisites() {
    log "Checking prerequisites..."

    detect_os

    # --- Git ---
    if ! command -v git >/dev/null 2>&1; then
        warn "git is not installed."
        if ask_yn "Install git now?" "y"; then
            run_sudo "install git" apt-get install -y -qq git >/dev/null
            log "git installed."
        else
            err "git is required to clone the AmityVox repository."
            err "Install it: sudo apt-get install git"
            exit 1
        fi
    fi

    # --- OpenSSL ---
    if ! command -v openssl >/dev/null 2>&1; then
        warn "openssl is not installed (needed for generating secrets)."
        if ask_yn "Install openssl now?" "y"; then
            run_sudo "install openssl" apt-get install -y -qq openssl >/dev/null
            log "openssl installed."
        else
            err "openssl is required for generating secure passwords and keys."
            err "Install it: sudo apt-get install openssl"
            exit 1
        fi
    fi

    # --- Docker ---
    if ! command -v docker >/dev/null 2>&1; then
        install_docker
    fi

    # --- Docker permissions ---
    fix_docker_permissions

    # --- Docker Compose v2 ---
    if docker compose version >/dev/null 2>&1; then
        COMPOSE_CMD="docker compose"
    elif command -v docker-compose >/dev/null 2>&1; then
        COMPOSE_CMD="docker-compose"
    else
        err "Docker Compose v2 is required but not found."
        err ""
        err "Docker Compose should have been installed as a plugin with Docker."
        err "Try reinstalling Docker, or install the plugin manually:"
        err "  sudo apt-get install docker-compose-plugin"
        exit 1
    fi

    # --- Docker daemon running ---
    if ! docker info >/dev/null 2>&1; then
        warn "Docker is installed but the daemon is not running."
        if ask_yn "Start the Docker service now?" "y"; then
            run_sudo "start Docker daemon" systemctl start docker
            # Wait for it to come up.
            local attempts=0
            while [ $attempts -lt 10 ]; do
                if docker info >/dev/null 2>&1; then
                    break
                fi
                attempts=$((attempts + 1))
                sleep 1
            done
            if ! docker info >/dev/null 2>&1; then
                err "Docker daemon did not start. Check: sudo systemctl status docker"
                exit 1
            fi
            log "Docker daemon started."
        else
            err "Docker must be running. Start it with: sudo systemctl start docker"
            exit 1
        fi
    fi

    # --- Disk space check (warn below 5GB) ---
    local free_kb
    free_kb=$(df --output=avail . 2>/dev/null | tail -1 | tr -d ' ' || echo "0")
    if [ "$free_kb" -lt 5242880 ] 2>/dev/null; then
        local free_human
        free_human=$(df -h --output=avail . 2>/dev/null | tail -1 | tr -d ' ' || echo "unknown")
        warn "Low disk space: $free_human free. AmityVox needs at least 5 GB for"
        warn "Docker images, database, and media storage."
        if ! ask_yn "Continue anyway?" "n"; then
            exit 1
        fi
    fi

    # --- Memory check (warn below 2GB) ---
    local mem_total_kb
    mem_total_kb=$(awk '/^MemTotal:/{print $2}' /proc/meminfo 2>/dev/null || echo "0")
    if [ "$mem_total_kb" -lt 2097152 ] 2>/dev/null; then
        local mem_human
        mem_human=$(awk '/^MemTotal:/{printf "%.0f MB", $2/1024}' /proc/meminfo 2>/dev/null || echo "unknown")
        warn "Low memory: $mem_human total. AmityVox recommends at least 2 GB RAM."
        warn "On Raspberry Pi, performance may be limited."
        if ! ask_yn "Continue anyway?" "y"; then
            exit 1
        fi
    fi

    echo
    log "All prerequisites satisfied."
    info "  Docker:   $(docker --version 2>/dev/null | sed 's/Docker version //')"
    info "  Compose:  $($COMPOSE_CMD version 2>/dev/null | sed 's/Docker Compose version //')"
    info "  Git:      $(git --version 2>/dev/null | sed 's/git version //')"
    info "  Arch:     $ARCH ($DOCKER_ARCH)"
}

# ============================================================
# Step 5: Clone or Locate Repository
# ============================================================
setup_repo() {
    # If the script is run from inside a checkout (not piped), use that directory.
    if [ "$SCRIPT_IS_PIPED" = "false" ] && [ -f "$SCRIPT_DIR/deploy/docker/docker-compose.yml" ]; then
        INSTALL_DIR="$SCRIPT_DIR"
        log "Using existing checkout at $INSTALL_DIR"
        cd "$INSTALL_DIR"
        return
    fi

    # Piped or run from outside a checkout — clone or update.
    if [ -d "$INSTALL_DIR/.git" ]; then
        log "Found existing installation at $INSTALL_DIR"
        if ask_yn "Update to the latest version?"; then
            cd "$INSTALL_DIR"
            run_verbose git pull origin "$BRANCH" 2>/dev/null || warn "Could not pull latest changes (continuing with existing)"
        else
            cd "$INSTALL_DIR"
        fi
    else
        log "Cloning AmityVox to $INSTALL_DIR..."
        run_verbose git clone --depth 1 -b "$BRANCH" "$REPO_URL" "$INSTALL_DIR"
        cd "$INSTALL_DIR"
    fi
}

# ============================================================
# Step 6: Interactive Configuration
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
# Step 7: Generate Secrets & Write .env
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
# Step 8: Build & Start Services
# ============================================================
build_and_start() {
    log "Building AmityVox (this may take a few minutes on first run)..."
    info "Architecture: $ARCH — building for $DOCKER_ARCH"
    echo

    # Run build with full output so failures are visible.
    if ! $COMPOSE_CMD -f "$COMPOSE_FILE" build --no-cache 2>&1 | while IFS= read -r line; do
        case "$line" in
            *"ERROR"*|*"error"*|*"FAILED"*|*"failed"*|*"CANCELED"*)
                echo -e "  ${RED}$line${NC}"
                ;;
            *"DONE"*|*"exporting"*|*"FINISHED"*|*"Successfully"*|*"Built"*)
                echo -e "  ${GREEN}$line${NC}"
                ;;
            *"#"*"RUN"*|*"#"*"COPY"*|*"#"*"FROM"*)
                echo -e "  ${DIM}$line${NC}"
                ;;
        esac
    done; then
        err "Docker build failed. See the output above for details."
        err ""
        err "Common causes:"
        err "  - Not enough disk space (need ~5 GB free)"
        err "  - Not enough RAM (need ~2 GB, 4 GB recommended)"
        err "  - Network issues downloading base images"
        err "  - On ARM devices, some images may take longer to build"
        err ""
        err "Retry with verbose output:"
        err "  $COMPOSE_CMD -f $COMPOSE_FILE build --no-cache 2>&1 | tee build.log"
        return 1
    fi

    echo
    log "Starting services..."
    if ! $COMPOSE_CMD -f "$COMPOSE_FILE" up -d 2>&1; then
        err "Failed to start services."
        err ""
        err "Check what went wrong:"
        err "  $COMPOSE_CMD -f $COMPOSE_FILE logs --tail=50"
        err "  $COMPOSE_CMD -f $COMPOSE_FILE ps"
        return 1
    fi

    # Wait for the backend to become healthy.
    log "Waiting for AmityVox to become ready..."
    local attempts=0
    local max_attempts=60
    while [ $attempts -lt $max_attempts ]; do
        if docker exec amityvox wget -qO- http://localhost:8080/health >/dev/null 2>&1; then
            break
        fi
        attempts=$((attempts + 1))
        # Show progress every 10 seconds.
        if [ $((attempts % 5)) -eq 0 ]; then
            info "  Still waiting... (${attempts}/${max_attempts})"
        fi
        sleep 2
    done

    if [ $attempts -ge $max_attempts ]; then
        warn "AmityVox is taking longer than expected to start (waited 2 minutes)."
        warn ""
        warn "This is common on first run — the database may still be migrating."
        warn "Check what's happening:"
        warn "  $COMPOSE_CMD -f $COMPOSE_FILE logs -f amityvox"
        warn "  $COMPOSE_CMD -f $COMPOSE_FILE ps"
        warn ""
        warn "Continuing with setup — some steps may fail if services aren't ready."
    else
        log "AmityVox is running."
    fi
}

# ============================================================
# Step 9: Bootstrap Garage S3 Storage
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
        warn "Garage is not ready after 60 seconds."
        warn ""
        warn "Check Garage status:"
        warn "  docker logs amityvox-garage --tail=20"
        warn "  docker exec amityvox-garage /garage status"
        warn ""
        warn "You'll need to set up S3 storage manually. See the README."
        return 1
    fi

    # Get node ID.
    local garage_status
    garage_status=$(docker exec amityvox-garage /garage status 2>&1)
    local node_id
    node_id=$(echo "$garage_status" | grep -oE '[a-f0-9]{64}' | head -1 || true)
    if [ -z "$node_id" ]; then
        warn "Could not determine Garage node ID."
        warn "Garage status output:"
        warn "$garage_status"
        warn ""
        warn "Manual S3 setup required. See the README."
        return 1
    fi

    debug "Garage node ID: ${node_id:0:16}..."

    # Assign layout and apply.
    local assign_output
    assign_output=$(docker exec amityvox-garage /garage layout assign -z dc1 -c 1G "$node_id" 2>&1) || true
    debug "Layout assign: $assign_output"

    # Get current layout version and apply next.
    local layout_output
    layout_output=$(docker exec amityvox-garage /garage layout show 2>&1)
    local layout_version
    layout_version=$(echo "$layout_output" | sed -n 's/.*version \([0-9]\{1,\}\).*/\1/p' | head -1)
    layout_version="${layout_version:-0}"
    local next_version=$((layout_version + 1))

    local apply_output
    apply_output=$(docker exec amityvox-garage /garage layout apply --version "$next_version" 2>&1) || true
    debug "Layout apply (v$next_version): $apply_output"

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
    access_key=$(echo "$key_info" | sed -n 's/.*Key ID: \(\S\{1,\}\).*/\1/p' | head -1)
    local secret_key
    secret_key=$(echo "$key_info" | sed -n 's/.*Secret key: \(\S\{1,\}\).*/\1/p' | head -1)

    if [ -n "$access_key" ] && [ -n "$secret_key" ]; then
        # Update .env with the real credentials.
        sed -i "s|^AMITYVOX_STORAGE_ACCESS_KEY=.*|AMITYVOX_STORAGE_ACCESS_KEY=$access_key|" .env
        sed -i "s|^AMITYVOX_STORAGE_SECRET_KEY=.*|AMITYVOX_STORAGE_SECRET_KEY=$secret_key|" .env

        # Restart amityvox to pick up the new S3 credentials.
        $COMPOSE_CMD -f "$COMPOSE_FILE" restart amityvox >/dev/null 2>&1

        log "S3 storage configured (key: ${access_key:0:8}...)"
    else
        warn "Could not extract Garage credentials from key info:"
        warn "$key_info"
        warn ""
        warn "Check manually:"
        warn "  docker exec amityvox-garage /garage key info amityvox-key"
        return 1
    fi
}

# ============================================================
# Step 10: Create Admin Account
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

    if [ $attempts -ge 30 ]; then
        warn "Backend did not become healthy after restart."
        warn "Check logs: $COMPOSE_CMD -f $COMPOSE_FILE logs -f amityvox"
        warn ""
        warn "Create admin manually once the server is running:"
        warn "  docker exec amityvox amityvox admin create-user $ADMIN_USER $ADMIN_EMAIL <password>"
        warn "  docker exec amityvox amityvox admin set-admin $ADMIN_USER"
        return 1
    fi

    local create_output
    if create_output=$(docker exec amityvox amityvox admin create-user "$ADMIN_USER" "$ADMIN_EMAIL" "$ADMIN_PASS" 2>&1); then
        docker exec amityvox amityvox admin set-admin "$ADMIN_USER" >/dev/null 2>&1
        log "Admin account created: $ADMIN_USER ($ADMIN_EMAIL)"
    else
        warn "Could not create admin account."
        warn "Output: $create_output"
        warn ""
        warn "The user may already exist. Create one manually with:"
        warn "  docker exec amityvox amityvox admin create-user <user> <email> <password>"
        warn "  docker exec amityvox amityvox admin set-admin <user>"
    fi
}

# ============================================================
# Step 11: Print Summary
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
    if [ -n "${ADMIN_USER:-}" ]; then
        echo -e "  ${BOLD}Admin account:${NC}   $ADMIN_USER / $ADMIN_EMAIL"
    fi
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

    if [ "${GIPHY_ENABLED:-false}" = "true" ] && [ -z "${GIPHY_API_KEY:-}" ]; then
        warn "Giphy is enabled but no API key was set. Get one at https://developers.giphy.com/dashboard/"
        warn "Then add it to .env as AMITYVOX_GIPHY_API_KEY and restart."
        echo
    fi

    if [ -z "${VAPID_PUBLIC:-}" ]; then
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
            DOMAIN=$(sed -n 's/^AMITYVOX_INSTANCE_DOMAIN=//p' .env 2>/dev/null | head -1)
            DOMAIN="${DOMAIN:-localhost}"
        fi
    else
        collect_config
        generate_config
    fi

    echo
    build_and_start
    setup_garage || true
    if [ -n "${ADMIN_USER:-}" ]; then
        create_admin || true
    fi
    print_summary
}

main "$@"
