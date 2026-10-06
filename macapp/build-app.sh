#!/usr/bin/env bash
# Build a complete local Pantheon.app from this checkout.
#
# This is deliberately a developer build, never a commercial release: it
# creates the same app/CLI/recipe payload shape as scripts/build-dmg.sh, but
# does not notarize, staple, publish, or install a LaunchAgent. Keeping this
# tool honest prevents a second, partial "Sirsi Menubar.app" from competing
# with Pantheon in the user's menu bar.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$HERE/.." && pwd)"
VERSION="$(tr -d '\n' < "$PROJECT_ROOT/VERSION")"
DEST="${1:-$HOME/Applications}"
APP="$DEST/Pantheon.app"
SWIFT_BIN="$HERE/.build/release/SirsiMenubar"
CLI_BIN="$HERE/.build/release/sirsi"
INFO_TEMPLATE="$PROJECT_ROOT/cmd/sirsi-menubar/bundle/Info.plist"
PKG_INFO="$PROJECT_ROOT/cmd/sirsi-menubar/bundle/PkgInfo"
LAUNCH_AGENT="$PROJECT_ROOT/cmd/sirsi-menubar/bundle/ai.sirsi.pantheon.plist"
BRAND_LOGO="$PROJECT_ROOT/docs/assets/sirsi-logo-white.png"
GO_LDFLAGS="-s -w -X github.com/SirsiMaster/sirsi-pantheon/internal/version.Version=v${VERSION}"

[[ -f "$INFO_TEMPLATE" && -f "$PKG_INFO" && -f "$LAUNCH_AGENT" && -f "$BRAND_LOGO" ]] || {
    echo "✘ canonical bundle resources are incomplete in this checkout" >&2
    exit 1
}

echo "▸ building native Pantheon workspace…"
( cd "$HERE" && swift build -c release )
echo "▸ building bundled sirsi CLI…"
( cd "$PROJECT_ROOT" && CGO_ENABLED=1 GOARCH="$(uname -m)" go build -ldflags="$GO_LDFLAGS" -o "$CLI_BIN" ./cmd/sirsi/ )

echo "▸ assembling complete developer payload at $APP"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp "$SWIFT_BIN" "$APP/Contents/MacOS/sirsi-menubar"
cp "$CLI_BIN" "$APP/Contents/MacOS/sirsi"
cp "$INFO_TEMPLATE" "$APP/Contents/Info.plist"
cp "$PKG_INFO" "$APP/Contents/PkgInfo"
cp "$LAUNCH_AGENT" "$APP/Contents/Resources/ai.sirsi.pantheon.plist"
cp "$BRAND_LOGO" "$APP/Contents/Resources/sirsi-logo-white.png"
cp -R "$PROJECT_ROOT/contracts/stacklab" "$APP/Contents/Resources/StackLab"
/usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString ${VERSION}" "$APP/Contents/Info.plist"
/usr/libexec/PlistBuddy -c "Set :CFBundleVersion ${VERSION}" "$APP/Contents/Info.plist"

# AppleDouble sidecars are transport metadata, never product resources.
/usr/bin/find "$APP" -type f -name '._*' -delete
if /usr/bin/find "$APP" -type f -name '._*' -print -quit | /usr/bin/grep -q .; then
    echo "✘ AppleDouble metadata remains in the app payload" >&2
    exit 1
fi

SIGN_ID="${SIRSI_SIGN_IDENTITY:-Sirsi Local Code Signing}"
ALLOW_ADHOC="${SIRSI_ALLOW_ADHOC:-0}"
if security find-identity -p codesigning 2>/dev/null | grep -q "$SIGN_ID"; then
    for inner in "$APP/Contents/MacOS/sirsi" "$APP/Contents/MacOS/sirsi-menubar"; do
        codesign --force --sign "$SIGN_ID" --identifier ai.sirsi.pantheon "$inner"
    done
    codesign --force --deep --sign "$SIGN_ID" --identifier ai.sirsi.pantheon "$APP"
    echo "▸ signed with local identity: $SIGN_ID"
elif [ "$ALLOW_ADHOC" = "1" ]; then
    echo "⚠ local signing identity missing — using ad-hoc signature for this non-distributable developer build" >&2
    codesign --force --deep --sign - --identifier ai.sirsi.pantheon "$APP"
else
    echo "✘ local signing identity '$SIGN_ID' not found. Create it with macapp/make-signing-cert.sh," >&2
    echo "  or set SIRSI_ALLOW_ADHOC=1 for a non-distributable developer build." >&2
    exit 1
fi

echo "▸ verifying complete app payload with the bundled canonical engine…"
"$APP/Contents/MacOS/sirsi" package-inventory \
    --app "$APP" \
    --version "$VERSION" \
    --build "$VERSION" \
    --info-plist "$APP/Contents/Info.plist" \
    --pkg-info "$PKG_INFO" \
    --launch-agent "$LAUNCH_AGENT" \
    --brand-logo "$BRAND_LOGO" \
    --require-code-signature

echo "✓ complete Pantheon developer app built: $APP"
echo "  includes: native workspace, bundled CLI, Sirsi logo, Stack Lab recipes, PkgInfo, and LaunchAgent bytes"
echo "  this is a developer build only; use scripts/build-dmg.sh --release for signed/notarized distribution"
