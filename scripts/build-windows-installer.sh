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
for cmd in sirsi sirsi-agent; do
    dir="./cmd/${cmd}/"
    # sirsi-anubis etc. have different cmd paths
    echo "  Compiling ${cmd}..."
    GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
        go build -ldflags="${GO_LDFLAGS}" -o "${WIN_DIR}/${cmd}.exe" "${dir}"
done

# The deity binaries have different source paths
declare -A DEITY_PATHS=(
    ["sirsi-anubis"]="./cmd/anubis/"
    ["sirsi-maat"]="./cmd/maat/"
    ["sirsi-thoth"]="./cmd/thoth/"
    ["sirsi-scarab"]="./cmd/scarab/"
    ["sirsi-guard"]="./cmd/guard/"
)

for cmd in "${!DEITY_PATHS[@]}"; do
    echo "  Compiling ${cmd}..."
    GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
        go build -ldflags="${GO_LDFLAGS}" -o "${WIN_DIR}/${cmd}.exe" "${DEITY_PATHS[$cmd]}"
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
