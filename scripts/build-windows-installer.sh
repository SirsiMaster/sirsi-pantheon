#!/usr/bin/env bash
# build-windows-installer.sh — Build Windows NSIS installer
# Requires: NSIS (makensis), Go toolchain
# Usage: scripts/build-windows-installer.sh [--version VERSION]
set -euo pipefail

VERSION="0.17.2"
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BUILD_DIR="${PROJECT_ROOT}/bin"
WIN_DIR="${BUILD_DIR}/windows"
GO_LDFLAGS="-s -w -X github.com/SirsiMaster/sirsi-pantheon/internal/version.Version=v${VERSION}"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --version) VERSION="$2"; GO_LDFLAGS="-s -w -X github.com/SirsiMaster/sirsi-pantheon/internal/version.Version=v${VERSION}"; shift 2 ;;
        *) echo "Unknown: $1"; exit 1 ;;
    esac
done

echo "Building Sirsi Pantheon Windows Installer"
echo "  Version: ${VERSION}"

# Build all Windows binaries
mkdir -p "${WIN_DIR}"
# Standalone deity binaries removed 2026-06-06 — redundant with `sirsi`
# subcommands (see .goreleaser.yaml note). Ship sirsi + sirsi-agent only.
for cmd in sirsi sirsi-agent; do
    dir="./cmd/${cmd}/"
    echo "  Compiling ${cmd}..."
    GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
        go build -ldflags="${GO_LDFLAGS}" -o "${WIN_DIR}/${cmd}.exe" "${dir}"
done

echo "  All binaries compiled."

# Build installer with NSIS
# NSIS EnvVarUpdate plugin is needed — download if missing
NSIS_PLUGINS="/usr/local/share/nsis/Plugins"
if [ -d "/usr/share/nsis" ]; then
    NSIS_PLUGINS="/usr/share/nsis/Plugins"
fi

echo "  Running makensis..."
makensis -DVERSION="${VERSION}" "${PROJECT_ROOT}/scripts/windows-installer.nsi"

echo ""
echo "Installer: ${BUILD_DIR}/SirsiPantheon-${VERSION}-windows-setup.exe"
ls -lh "${BUILD_DIR}/SirsiPantheon-${VERSION}-windows-setup.exe"
