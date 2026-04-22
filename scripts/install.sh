#!/bin/bash
# 𓁢 Sirsi Pantheon — Binary Installer
# curl -fsSL https://sirsi.ai/install.sh | sh
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

# 6. Install binary
if [ -f "${TMPDIR}/sirsi" ]; then
    cp "${TMPDIR}/sirsi" "${INSTALL_DIR}/sirsi"
    chmod +x "${INSTALL_DIR}/sirsi"
else
    echo -e "${RED}  Binary not found in archive.${NC}"
    exit 1
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
echo -e "${DIM}  Run 'sirsi' to begin.${NC}"
