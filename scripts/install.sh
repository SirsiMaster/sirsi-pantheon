#!/bin/bash
# 𓁢 Sirsi Pantheon — Production Installer
# "One Install. All Deities."

set -e

GOLD='\033[0;33m'
NC='\033[0m' # No Color
BOLD='\033[1m'

echo -e "${GOLD}${BOLD}"
echo "  𓁢  Sirsi Pantheon"
echo "  ═══════════════════════════════"
echo "  Unified DevOps Intelligence Platform"
echo "  Establishing The Ritual of Access..."
echo -e "${NC}"

# 1. Detect OS/Arch
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

if [[ "$OS" != "darwin" ]]; then
    echo "⚠️  Pantheon is currently optimized for macOS."
fi

# 2. Build local binary (or download in future)
echo "📦 Building Pantheon release binary..."
go build -o ./dist/sirsi ./cmd/sirsi/

# 3. Install to ~/go/bin or /usr/local/bin
INSTALL_DIR="$HOME/go/bin"
mkdir -p "$INSTALL_DIR"

echo "🚚 Installing to $INSTALL_DIR/sirsi..."
cp ./dist/pantheon "$INSTALL_DIR/sirsi"

# 4. Check PATH
if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
    echo "⚠️  $INSTALL_DIR is not in your PATH."
    echo "   Add this to your .zshrc or .bash_profile:"
    echo "   export PATH=\$PATH:$INSTALL_DIR"
fi

echo -e "\n${GOLD}✅ Pantheon is now real.${NC}"
echo "Run 'sirsi' to begin the weighing."
echo "Run 'sirsi initiate' to grant deep access."
