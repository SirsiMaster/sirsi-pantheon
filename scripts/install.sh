#!/bin/bash
# 𓁢 Sirsi Pantheon — Binary Installer
# bash <(curl -fsSL https://raw.githubusercontent.com/SirsiMaster/sirsi-pantheon/main/scripts/install.sh)
# No Go toolchain required — downloads pre-built binary from GitHub Releases.
set -e

REPO="SirsiMaster/sirsi-pantheon"
GOLD='\033[0;33m'
GREEN='\033[0;32m'
RED='\033[0;31m'
DIM='\033[0;90m'
NC='\033[0m'
BOLD='\033[1m'

echo -e "${GOLD}${BOLD}"
echo "  𓁢  Sirsi Pantheon"
echo "  ═══════════════════════════════"
echo "  Unified DevOps Intelligence Platform"
echo -e "${NC}"

# 1. Detect OS/Arch
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *)
        echo -e "${RED}Unsupported architecture: $ARCH${NC}"
        exit 1
        ;;
esac

case "$OS" in
    darwin) EXT="tar.gz" ;;
    linux)  EXT="tar.gz" ;;
    *)
        echo -e "${RED}Unsupported OS: $OS${NC}"
        exit 1
        ;;
esac

echo -e "${DIM}  Platform: ${OS}/${ARCH}${NC}"

install_executable() {
    src="$1"
    dest="$2"
    tmp="${dest}.$$.$RANDOM.new"

    cp "$src" "$tmp"
    chmod +x "$tmp"

    if [ "$OS" = "darwin" ] && command -v codesign &>/dev/null; then
        if ! codesign --force --sign - "$tmp" >/dev/null 2>&1; then
            rm -f "$tmp"
            echo -e "${RED}  codesign failed for ${dest}; keeping existing binary in place.${NC}"
            exit 1
        fi
    fi

    mv -f "$tmp" "$dest"
}

# 2. Determine install directory
INSTALL_DIR="${SIRSI_INSTALL_DIR:-$HOME/.local/bin}"
mkdir -p "$INSTALL_DIR"

# 3. Fetch latest release tag
echo -e "${DIM}  Fetching latest release...${NC}"
if command -v curl &>/dev/null; then
    LATEST=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
elif command -v wget &>/dev/null; then
    LATEST=$(wget -qO- "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
else
    echo -e "${RED}Neither curl nor wget found. Install one and retry.${NC}"
    exit 1
fi

if [ -z "$LATEST" ]; then
    echo -e "${DIM}  Could not fetch latest release. Trying build from source...${NC}"
    if command -v go &>/dev/null; then
        echo -e "${DIM}  Building from source with Go...${NC}"
        go install "github.com/${REPO}/cmd/sirsi@latest"
        echo -e "${GREEN}  ✅ Installed via go install${NC}"
        exit 0
    else
        echo -e "${RED}  No release found and Go not installed.${NC}"
        echo -e "${DIM}  Install Go from https://go.dev or check GitHub Releases manually.${NC}"
        exit 1
    fi
fi

echo -e "${DIM}  Latest: ${LATEST}${NC}"

# 4. Download binary
TARBALL="sirsi-pantheon_${LATEST#v}_${OS}_${ARCH}.${EXT}"
URL="https://github.com/${REPO}/releases/download/${LATEST}/${TARBALL}"

TMPDIR=$(mktemp -d)
trap "rm -rf $TMPDIR" EXIT

echo -e "${DIM}  Downloading ${TARBALL}...${NC}"
if command -v curl &>/dev/null; then
    curl -fsSL -o "${TMPDIR}/${TARBALL}" "$URL"
else
    wget -q -O "${TMPDIR}/${TARBALL}" "$URL"
fi

# 5. Extract
echo -e "${DIM}  Extracting...${NC}"
tar xzf "${TMPDIR}/${TARBALL}" -C "$TMPDIR"

# 5b. Schema ceiling gate — refuse to install a candidate that cannot open the
# live router store. This is the 2026-08-06 fleet-lockout vector: a binary
# whose migration ceiling was below the live schema replaced the running one and
# fail-closed every agent on next launch. Probe with the new binary; it exits 0
# if compatible, non-zero (with a diagnostic) if not.
DB_PATH="${SIRSI_ROUTER_DB:-$HOME/.sirsi/router.db}"
if [ -f "$DB_PATH" ]; then
    echo -e "${DIM}  Checking router schema compatibility...${NC}"
    if ! "${TMPDIR}/sirsi" schema-check --db "$DB_PATH" 2>/dev/null; then
        echo -e "${RED}  ✗ Schema ceiling gate: this release cannot open your live router store.${NC}"
        echo -e "${DIM}  Your router.db is at a higher schema version than this release supports.${NC}"
        echo -e "${DIM}  Wait for a compatible release, or back up and reset: ${DB_PATH}${NC}"
        exit 1
    fi
    echo -e "${DIM}  Schema compatible ✓${NC}"
fi

# 6. Install binaries (sirsi always; sirsi-menubar on macOS when present)
if [ -f "${TMPDIR}/sirsi" ]; then
    install_executable "${TMPDIR}/sirsi" "${INSTALL_DIR}/sirsi"
else
    echo -e "${RED}  Binary not found in archive.${NC}"
    exit 1
fi

# Menubar is installed by default on macOS, so `sirsi setup` can load the
# LaunchAgent. The CGO/Cocoa menubar can't ride in the cross-compiled
# goreleaser archive, so it's published as a standalone darwin/arm64 asset
# (Developer-ID signed; Intel runs it via Rosetta). Prefer the archive copy if
# present, else fetch the standalone asset. Failure here is non-fatal — the
# wizard reports a missing menubar rather than aborting the whole install.
if [ "$OS" = "darwin" ]; then
    if [ -f "${TMPDIR}/sirsi-menubar" ]; then
        install_executable "${TMPDIR}/sirsi-menubar" "${INSTALL_DIR}/sirsi-menubar"
        echo -e "${DIM}  Installed sirsi-menubar${NC}"
    else
        MB_ARCHIVE="sirsi-menubar_${LATEST#v}_darwin_arm64.tar.gz"
        MB_URL="https://github.com/${REPO}/releases/download/${LATEST}/${MB_ARCHIVE}"
        echo -e "${DIM}  Fetching menubar (${MB_ARCHIVE})...${NC}"
        if curl -fsSL -o "${TMPDIR}/${MB_ARCHIVE}" "$MB_URL" 2>/dev/null &&
            tar xzf "${TMPDIR}/${MB_ARCHIVE}" -C "$TMPDIR" 2>/dev/null &&
            [ -f "${TMPDIR}/sirsi-menubar" ]; then
            install_executable "${TMPDIR}/sirsi-menubar" "${INSTALL_DIR}/sirsi-menubar"
            echo -e "${DIM}  Installed sirsi-menubar${NC}"
        else
            echo -e "${DIM}  Menubar asset not found for ${LATEST} — 'sirsi setup' will note it.${NC}"
        fi
    fi
fi

# 7. Check PATH
if [[ ":$PATH:" != *":${INSTALL_DIR}:"* ]]; then
    echo ""
    echo -e "${GOLD}  Add this to your shell profile:${NC}"
    echo -e "${DIM}  export PATH=\"\$PATH:${INSTALL_DIR}\"${NC}"
    echo ""
fi

# 8. Verify
VERSION=$("${INSTALL_DIR}/sirsi" version 2>/dev/null | head -1 || echo "unknown")
echo ""
echo -e "${GREEN}${BOLD}  ✅ Sirsi Pantheon installed${NC}"
echo -e "${DIM}  Binary: ${INSTALL_DIR}/sirsi${NC}"
echo -e "${DIM}  ${VERSION}${NC}"
echo ""

# 9. Hand off to the setup wizard — choose surfaces (CLI/TUI/IDE; menubar
# default on macOS) and grant permissions once, so nothing stops you later.
# Only when attached to a terminal; piped installs print the next step instead.
if [ -t 0 ] && [ -t 1 ]; then
    echo -e "${GOLD}  Launching the setup wizard...${NC}"
    echo ""
    "${INSTALL_DIR}/sirsi" setup || true
else
    echo -e "${DIM}  Next: run 'sirsi setup' to choose your surfaces and grant permissions.${NC}"
fi
