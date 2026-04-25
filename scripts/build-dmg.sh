#!/usr/bin/env bash
# build-dmg.sh — Create a macOS DMG installer for Sirsi Pantheon
# Usage: scripts/build-dmg.sh [--version VERSION] [--arch ARCH]
# Requires macOS (hdiutil is macOS-specific)

set -euo pipefail

# --- Defaults ---
VERSION="0.17.0"
ARCH="arm64"
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BUILD_DIR="${PROJECT_ROOT}/bin"
APP_NAME="Pantheon.app"
BUNDLE_DIR="${PROJECT_ROOT}/${APP_NAME}"
DMG_VOLUME="Sirsi Pantheon"
GO_LDFLAGS="-s -w -X main.version=v${VERSION}"

# --- Parse flags ---
while [[ $# -gt 0 ]]; do
    case "$1" in
        --version)
            VERSION="$2"
            GO_LDFLAGS="-s -w -X main.version=v${VERSION}"
            shift 2
            ;;
        --arch)
            ARCH="$2"
            shift 2
            ;;
        *)
            echo "Unknown flag: $1"
            echo "Usage: $0 [--version VERSION] [--arch ARCH]"
            exit 1
            ;;
    esac
done

DMG_NAME="SirsiPantheon-${VERSION}-${ARCH}.dmg"
DMG_PATH="${BUILD_DIR}/${DMG_NAME}"
STAGING_DIR="${BUILD_DIR}/dmg-staging"

echo "Building Sirsi Pantheon DMG"
echo "  Version: ${VERSION}"
echo "  Arch:    ${ARCH}"
echo "  Output:  ${DMG_PATH}"
echo ""

# --- Verify macOS ---
if [[ "$(uname -s)" != "Darwin" ]]; then
    echo "ERROR: DMG creation requires macOS (hdiutil is macOS-specific)."
    exit 1
fi

# --- Build binaries ---
echo "Compiling sirsi-menubar..."
mkdir -p "${BUILD_DIR}"
CGO_ENABLED=1 GOARCH="${ARCH}" go build -ldflags="${GO_LDFLAGS}" -o "${BUILD_DIR}/sirsi-menubar" ./cmd/sirsi-menubar/

echo "Compiling sirsi CLI..."
CGO_ENABLED=0 GOARCH="${ARCH}" go build -ldflags="${GO_LDFLAGS}" -o "${BUILD_DIR}/sirsi" ./cmd/sirsi/

# --- Create .app bundle ---
echo "Assembling ${APP_NAME}..."
rm -rf "${BUNDLE_DIR}"
mkdir -p "${BUNDLE_DIR}/Contents/MacOS"
mkdir -p "${BUNDLE_DIR}/Contents/Resources"

cp "${BUILD_DIR}/sirsi-menubar" "${BUNDLE_DIR}/Contents/MacOS/sirsi-menubar"
cp "${BUILD_DIR}/sirsi"         "${BUNDLE_DIR}/Contents/MacOS/sirsi"
cp "${PROJECT_ROOT}/cmd/sirsi-menubar/bundle/Info.plist" "${BUNDLE_DIR}/Contents/Info.plist"
cp "${PROJECT_ROOT}/cmd/sirsi-menubar/bundle/PkgInfo"    "${BUNDLE_DIR}/Contents/PkgInfo"
cp "${PROJECT_ROOT}/cmd/sirsi-menubar/bundle/ai.sirsi.pantheon.plist" "${BUNDLE_DIR}/Contents/Resources/ai.sirsi.pantheon.plist"

# --- Ad-hoc code signing ---
echo "Signing ${APP_NAME} (ad-hoc)..."
codesign --force --deep --sign - "${BUNDLE_DIR}"

# --- Prepare DMG staging area ---
echo "Staging DMG contents..."
rm -rf "${STAGING_DIR}"
mkdir -p "${STAGING_DIR}"

cp -R "${BUNDLE_DIR}" "${STAGING_DIR}/"
ln -s /Applications "${STAGING_DIR}/Applications"

cat > "${STAGING_DIR}/README.txt" <<'READMEEOF'
Sirsi Pantheon — Unified DevOps Intelligence Platform

INSTALLATION
  1. Drag Pantheon.app into the Applications folder.
  2. Launch Pantheon from your Applications folder or Spotlight.

The Pantheon.app bundle includes:
  - sirsi-menubar: Menu bar app for macOS status monitoring
  - sirsi: CLI binary (also available at Pantheon.app/Contents/MacOS/sirsi)

To use the CLI from your terminal, add an alias:
  alias sirsi="/Applications/Pantheon.app/Contents/MacOS/sirsi"

Or install the CLI separately via Homebrew:
  brew tap SirsiMaster/tools && brew install sirsi-pantheon

For more information: https://sirsi.ai/pantheon
READMEEOF

# --- Create DMG ---
echo "Creating DMG..."
rm -f "${DMG_PATH}"

hdiutil create \
    -volname "${DMG_VOLUME}" \
    -srcfolder "${STAGING_DIR}" \
    -ov \
    -format UDZO \
    "${DMG_PATH}"

# --- Cleanup ---
rm -rf "${STAGING_DIR}"

echo ""
echo "DMG created: ${DMG_PATH}"
ls -lh "${DMG_PATH}"
