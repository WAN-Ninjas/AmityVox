#!/usr/bin/env bash
# shellcheck disable=SC1090,SC2034,SC2329
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
INSTALL_SH="$ROOT_DIR/install.sh"

load_installer_functions() {
    local tmp
    tmp="$(mktemp)"
    sed '/^main "\$@"/,$d' "$INSTALL_SH" > "$tmp"
    # shellcheck source=/dev/null
    source "$tmp"
    rm -f "$tmp"
}

assert_eq() {
    local got="$1"
    local want="$2"
    local label="$3"
    if [ "$got" != "$want" ]; then
        echo "FAIL: $label: got '$got', want '$want'" >&2
        exit 1
    fi
}

test_detect_os_does_not_abort_under_errexit() {
    load_installer_functions
    info() { :; }

    detect_os

    if [ -z "${DISTRO_FAMILY:-}" ]; then
        echo "FAIL: detect_os did not set DISTRO_FAMILY" >&2
        exit 1
    fi
}

test_install_package_uses_pacman_on_arch() {
    load_installer_functions
    DISTRO_FAMILY=arch
    NONINTERACTIVE=1
    commands=()

    command() {
        if [ "$1" = "-v" ] && [ "${2:-}" = "pacman" ]; then
            return 0
        fi
        builtin command "$@"
    }
    run_sudo() {
        commands+=("$*")
    }

    install_package git "testing"
    assert_eq "${commands[0]}" "install git pacman -Sy --needed --noconfirm git" "arch package command"
}

test_install_package_uses_apt_on_debian() {
    load_installer_functions
    DISTRO_FAMILY=debian
    NONINTERACTIVE=1
    commands=()

    command() {
        if [ "$1" = "-v" ] && [ "${2:-}" = "apt-get" ]; then
            return 0
        fi
        builtin command "$@"
    }
    run_sudo() {
        commands+=("$*")
    }

    install_package openssl "testing"
    assert_eq "${commands[0]}" "install openssl apt-get update -qq" "apt update command"
    assert_eq "${commands[1]}" "install openssl apt-get install -y -qq openssl" "apt install command"
}

test_detect_arch_family_from_derivative() {
    load_installer_functions
    DISTRO_ID=cachyos
    ID_LIKE=arch

    classify_distro_family

    assert_eq "$DISTRO_FAMILY" "arch" "CachyOS family"
}

test_detect_debian_and_ubuntu_arm_families() {
    load_installer_functions

    DISTRO_ID=raspbian
    ID_LIKE=""
    classify_distro_family
    assert_eq "$DISTRO_FAMILY" "debian" "Raspberry Pi OS family"

    DISTRO_ID=armbian
    ID_LIKE="debian"
    classify_distro_family
    assert_eq "$DISTRO_FAMILY" "debian" "Armbian Debian family"

    DISTRO_ID=armbian
    ID_LIKE="ubuntu debian"
    classify_distro_family
    assert_eq "$DISTRO_FAMILY" "ubuntu" "Armbian Ubuntu family"
}

test_compose_uses_explicit_env_file() {
    load_installer_functions
    local tmpdir
    tmpdir="$(mktemp -d)"
    (
        cd "$tmpdir"
        : > .env
        COMPOSE_CMD="docker compose"
        COMPOSE_FILE="deploy/docker/docker-compose.yml"
        commands=()
        docker() {
            commands+=("docker $*")
        }
        compose build --no-cache
        assert_eq "${commands[0]}" "docker compose --env-file .env -f deploy/docker/docker-compose.yml build --no-cache" "compose env-file command"
        assert_eq "$(compose_display)" "docker compose --env-file .env -f deploy/docker/docker-compose.yml" "compose display"
    )
    rm -rf "$tmpdir"
}

test_garage_parsers_handle_v1_output() {
    load_installer_functions

    local status_output
    status_output='2026-05-26T16:51:54.210240Z INFO garage_net::netapp: Connection established to f052b1327942dcc8
==== HEALTHY NODES ====
ID                Hostname      Address          Tags  Zone  Capacity          DataAvail
f052b1327942dcc8  d6cae5335b57  172.19.0.7:3901              NO ROLE ASSIGNED'
    assert_eq "$(echo "$status_output" | garage_node_id_from_status)" "f052b1327942dcc8" "garage v1 short node id"

    local layout_output
    layout_output='==== CURRENT CLUSTER LAYOUT ====
Current cluster layout version: 3'
    assert_eq "$(echo "$layout_output" | garage_layout_version_from_show)" "3" "garage layout version"

    local key_output
    key_output='Key name: amityvox-key
Key ID: GKbb3fe42f858269cd3d4ea9fd
Secret key: ab4532ff1e7906c5a60754c5d6680da94a4c4ea38dde1992dec31b5cc2e6a7fd'
    assert_eq "$(echo "$key_output" | garage_key_access_from_info)" "GKbb3fe42f858269cd3d4ea9fd" "garage access key"
    assert_eq "$(echo "$key_output" | garage_key_secret_from_info)" "ab4532ff1e7906c5a60754c5d6680da94a4c4ea38dde1992dec31b5cc2e6a7fd" "garage secret key"

    local redacted_output
    redacted_output='Secret key: (redacted)'
    assert_eq "$(echo "$redacted_output" | garage_key_secret_from_info)" "" "garage redacted secret"
}

test_detect_os_does_not_abort_under_errexit
test_install_package_uses_pacman_on_arch
test_install_package_uses_apt_on_debian
test_detect_arch_family_from_derivative
test_detect_debian_and_ubuntu_arm_families
test_compose_uses_explicit_env_file
test_garage_parsers_handle_v1_output

echo "installer prerequisite tests passed"
