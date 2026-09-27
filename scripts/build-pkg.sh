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
# The payload staging tree is generated for this package only.  Do not carry
# Finder/resource-fork metadata across volumes: pkgbuild otherwise serializes
# it as visible AppleDouble `._*` files in the installer payload.
COPYFILE_DISABLE=1 /usr/bin/ditto "$APP_PATH" "$PAYLOAD_APP_DIR/Pantheon.app"
/usr/bin/xattr -cr "$PAYLOAD_ROOT"
if /usr/bin/find "$PAYLOAD_ROOT" -type f -name '._*' -print -quit | /usr/bin/grep -q .; then
    echo "ERROR: refusing PKG payload containing AppleDouble metadata." >&2
    exit 1
fi

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
COPYFILE_DISABLE=1 /usr/bin/pkgbuild "${PKGBUILD_ARGS[@]}"

# `pkgutil --payload-files` includes metadata records that look like
# AppleDouble paths.  Inspect the expanded archive instead: this proves what
# Installer will actually unpack and verifies the Stack Lab release contract
# survived the package boundary.
EXPANDED_ROOT="$PAYLOAD_ROOT/expanded-payload"
/usr/sbin/pkgutil --expand-full "$PKG_PATH" "$EXPANDED_ROOT"
if /usr/bin/find "$EXPANDED_ROOT" -type f -name '._*' -print -quit | /usr/bin/grep -q .; then
    echo "ERROR: refusing PKG with AppleDouble files in its expanded payload." >&2
    exit 1
fi
if [[ ! -f "$EXPANDED_ROOT/Payload/Applications/Pantheon.app/Contents/Resources/StackLab/ra-horus-fabric-wing-v1.json" ]]; then
    echo "ERROR: expanded PKG payload is missing the Ra Stack Lab contract." >&2
    exit 1
fi

# pkgutil intentionally exits nonzero for an unsigned package.  That is a
# useful signal for distribution, but not a reason to throw away a clearly
# labelled local release candidate.  A configured installer identity is the
# opposite: signature verification is mandatory and any failure is fatal.
if [[ -n "${DEVELOPER_ID_INSTALLER:-}" ]]; then
    /usr/sbin/pkgutil --check-signature "$PKG_PATH"
else
    set +e
    SIGNATURE_REPORT="$(/usr/sbin/pkgutil --check-signature "$PKG_PATH" 2>&1)"
    SIGNATURE_STATUS=$?
    set -e
    printf '%s\n' "$SIGNATURE_REPORT"
    if [[ "$SIGNATURE_REPORT" != *"Status: no signature"* ]]; then
        echo "ERROR: unsigned PKG did not report the expected unsigned state (pkgutil=$SIGNATURE_STATUS)." >&2
        exit 1
    fi
    echo "WARNING: unsigned PKG is a release candidate only; do not distribute it as a commercial installer." >&2
fi
echo "PKG created: $PKG_PATH"
