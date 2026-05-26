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

test_detect_os_does_not_abort_under_errexit
test_install_package_uses_pacman_on_arch
test_install_package_uses_apt_on_debian
test_detect_arch_family_from_derivative
test_detect_debian_and_ubuntu_arm_families

echo "installer prerequisite tests passed"
