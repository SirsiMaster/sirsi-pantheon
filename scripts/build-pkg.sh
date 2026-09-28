#!/bin/bash
# build-pkg.sh — Package the already-built Pantheon.app as a macOS installer.
#
# The DMG builder is the single bundle producer.  This script deliberately
# consumes that bundle, so the drag-and-drop and installer artifacts cannot
# silently contain different engines, Stack Lab contracts, or version bytes.
set -euo pipefail

# See build-dmg.sh: a package must not depend on ambient project PATH entries
# for compiler or macOS packaging tool resolution.
export PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"

VERSION=""
ARCH="arm64"
MODE=""
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
APP_PATH="${PROJECT_ROOT}/Pantheon.app"
BUILD_DIR="${PROJECT_ROOT}/bin"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --development)
            [[ -z "$MODE" ]] || { echo "ERROR: choose exactly one of --development or --release" >&2; exit 2; }
            MODE="development"; shift ;;
        --release)
            [[ -z "$MODE" ]] || { echo "ERROR: choose exactly one of --development or --release" >&2; exit 2; }
            MODE="release"; shift ;;
        --version) VERSION="$2"; shift 2 ;;
        --arch) ARCH="$2"; shift 2 ;;
        --app) APP_PATH="$2"; shift 2 ;;
        *) echo "Usage: $0 (--development | --release) --version VERSION [--arch ARCH] [--app PATH]" >&2; exit 2 ;;
    esac
done

[[ -n "$VERSION" ]] || { echo "ERROR: --version is required" >&2; exit 2; }
[[ -n "$MODE" ]] || { echo "ERROR: choose --development or --release explicitly" >&2; exit 2; }
[[ "$(uname -s)" == "Darwin" ]] || { echo "ERROR: PKG creation requires macOS" >&2; exit 1; }
[[ -d "$APP_PATH" && ! -L "$APP_PATH" ]] || { echo "ERROR: expected a real Pantheon.app at $APP_PATH" >&2; exit 1; }
[[ -x "$APP_PATH/Contents/MacOS/sirsi" ]] || { echo "ERROR: Pantheon.app is missing sirsi" >&2; exit 1; }
[[ -x "$APP_PATH/Contents/MacOS/sirsi-menubar" ]] || { echo "ERROR: Pantheon.app is missing sirsi-menubar" >&2; exit 1; }
[[ -d "$APP_PATH/Contents/Resources/StackLab" ]] || { echo "ERROR: Pantheon.app is missing Stack Lab contracts" >&2; exit 1; }

if [[ "$MODE" == "release" ]]; then
    for required in DEVELOPER_ID_INSTALLER APPLE_ID APPLE_TEAM_ID APPLE_APP_PASSWORD; do
        [[ -n "${!required:-}" ]] || { echo "ERROR: --release requires ${required}" >&2; exit 2; }
    done
    PKG_NAME="SirsiPantheon-${VERSION}-${ARCH}.pkg"
    ARTIFACT_LABEL="Commercial release"
else
    PKG_NAME="SirsiPantheon-${VERSION}-dev-${ARCH}.pkg"
    ARTIFACT_LABEL="Development"
fi

mkdir -p "$BUILD_DIR"
PAYLOAD_ROOT="$(mktemp -d /private/tmp/pantheon-pkg-payload.XXXXXX)"
PAYLOAD_ROOT_ID="$(/usr/bin/stat -f '%d:%i' "$PAYLOAD_ROOT")"

# The staging tree is this invocation's only mutable namespace. Retain its
# device/inode so an unrelated path substituted at the predictable temp name
# can never be removed by the EXIT handler. The cleanup deliberately uses
# find's no-follow default; symlinks are unlinked rather than traversed.
cleanup_payload_root() {
    local current_id=""
    if [[ -n "${PAYLOAD_ROOT:-}" && -n "${PAYLOAD_ROOT_ID:-}" && -d "$PAYLOAD_ROOT" ]]; then
        current_id="$(/usr/bin/stat -f '%d:%i' "$PAYLOAD_ROOT" 2>/dev/null || true)"
        if [[ "$current_id" == "$PAYLOAD_ROOT_ID" ]]; then
            /usr/bin/find "$PAYLOAD_ROOT" -depth -delete || \
                echo "WARNING: retained package staging cleanup debt at $PAYLOAD_ROOT" >&2
        else
            echo "WARNING: refusing to remove substituted package staging path $PAYLOAD_ROOT" >&2
        fi
    fi
}
trap cleanup_payload_root EXIT

PAYLOAD_APP_DIR="$PAYLOAD_ROOT/Applications"
PKG_PATH="$BUILD_DIR/$PKG_NAME"
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
if [[ "$MODE" == "release" ]]; then
    PKGBUILD_ARGS+=(--sign "$DEVELOPER_ID_INSTALLER")
else
    echo "Creating unsigned development PKG (not distributable)." >&2
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

# pkgutil intentionally exits nonzero for an unsigned package. That is useful
# local-development evidence. In commercial release mode signature,
# notarization, and stapling are all mandatory and any failure is fatal.
if [[ "$MODE" == "release" ]]; then
    /usr/sbin/pkgutil --check-signature "$PKG_PATH"
    xcrun notarytool submit "$PKG_PATH" \
        --apple-id "${APPLE_ID}" \
        --team-id "${APPLE_TEAM_ID}" \
        --password "${APPLE_APP_PASSWORD}" \
        --timeout 20m \
        --wait
    xcrun stapler staple "$PKG_PATH"
    xcrun stapler validate "$PKG_PATH"
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
    echo "Development PKG is unsigned and must not be distributed as a commercial installer." >&2
fi
echo "${ARTIFACT_LABEL} PKG created: $PKG_PATH"
