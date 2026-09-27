#!/usr/bin/env bash
# build-pkg.sh — Package the already-built Pantheon.app as a macOS installer.
#
# The DMG builder is the single bundle producer.  This script deliberately
# consumes that bundle, so the drag-and-drop and installer artifacts cannot
# silently contain different engines, Stack Lab contracts, or version bytes.
set -euo pipefail

VERSION=""
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
APP_PATH="${PROJECT_ROOT}/Pantheon.app"
BUILD_DIR="${PROJECT_ROOT}/bin"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --version) VERSION="$2"; shift 2 ;;
        --app) APP_PATH="$2"; shift 2 ;;
        *) echo "Usage: $0 --version VERSION [--app PATH]" >&2; exit 2 ;;
    esac
done

[[ -n "$VERSION" ]] || { echo "ERROR: --version is required" >&2; exit 2; }
[[ "$(uname -s)" == "Darwin" ]] || { echo "ERROR: PKG creation requires macOS" >&2; exit 1; }
[[ -d "$APP_PATH" && ! -L "$APP_PATH" ]] || { echo "ERROR: expected a real Pantheon.app at $APP_PATH" >&2; exit 1; }
[[ -x "$APP_PATH/Contents/MacOS/sirsi" ]] || { echo "ERROR: Pantheon.app is missing sirsi" >&2; exit 1; }
[[ -x "$APP_PATH/Contents/MacOS/sirsi-menubar" ]] || { echo "ERROR: Pantheon.app is missing sirsi-menubar" >&2; exit 1; }
[[ -d "$APP_PATH/Contents/Resources/StackLab" ]] || { echo "ERROR: Pantheon.app is missing Stack Lab contracts" >&2; exit 1; }

mkdir -p "$BUILD_DIR"
PAYLOAD_ROOT="$(mktemp -d /private/tmp/pantheon-pkg-payload.XXXXXX)"
PAYLOAD_APP_DIR="$PAYLOAD_ROOT/Applications"
PKG_PATH="$BUILD_DIR/SirsiPantheon-${VERSION}-arm64.pkg"
mkdir -p "$PAYLOAD_APP_DIR"
/usr/bin/ditto "$APP_PATH" "$PAYLOAD_APP_DIR/Pantheon.app"

PKGBUILD_ARGS=(
    --root "$PAYLOAD_ROOT"
    --install-location /
    --identifier ai.sirsi.pantheon
    --version "$VERSION"
)
if [[ -n "${DEVELOPER_ID_INSTALLER:-}" ]]; then
    PKGBUILD_ARGS+=(--sign "$DEVELOPER_ID_INSTALLER")
else
    echo "WARNING: creating unsigned PKG (DEVELOPER_ID_INSTALLER is not configured)." >&2
fi
PKGBUILD_ARGS+=("$PKG_PATH")
/usr/bin/pkgbuild "${PKGBUILD_ARGS[@]}"
/usr/sbin/pkgutil --check-signature "$PKG_PATH"
echo "PKG created: $PKG_PATH"
