#!/usr/bin/env bash
# Builds SirsiMenubar and packages it as a signed .app bundle with a stable
# CFBundleIdentifier (ai.sirsi.pantheon) so macOS TCC keys Full Disk Access on it
# across reinstalls. AMFI-safe install (fresh inode + ad-hoc sign). LSUIElement
# keeps it a Dock-less menubar agent. ADR-030.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
APP_NAME="Sirsi Menubar"
BUNDLE_ID="ai.sirsi.pantheon"
DEST="${1:-$HOME/Applications}"
APP="$DEST/$APP_NAME.app"

echo "▸ swift build…"
( cd "$HERE" && swift build -c release )
BIN="$HERE/.build/release/SirsiMenubar"

echo "▸ packaging $APP"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"

cat > "$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleIdentifier</key>
	<string>$BUNDLE_ID</string>
	<key>CFBundleName</key>
	<string>$APP_NAME</string>
	<key>CFBundleExecutable</key>
	<string>SirsiMenubar</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleShortVersionString</key>
	<string>1.0</string>
	<key>CFBundleVersion</key>
	<string>1</string>
	<key>LSUIElement</key>
	<true/>
	<key>LSMinimumSystemVersion</key>
	<string>13.0</string>
</dict>
</plist>
PLIST

# Fresh inode (never cp-over) then sign — avoids the AMFI stale-cdhash
# SIGKILL-137 class (reference_macos_amfi_cp_sigkill).
cp "$BIN" "$APP/Contents/MacOS/SirsiMenubar"

# Sign with a STABLE identity, not ad-hoc. Ad-hoc (cdhash-only) signatures are
# NOT honored by TCC for Full Disk Access across relaunch — the grant toggle
# reverts. A self-signed code-signing cert gives a stable designated requirement
# (identifier + certificate leaf) that TCC persists across relaunch AND across
# rebuilds, so the user grants FDA exactly once. Falls back to ad-hoc only if the
# cert is missing (FDA will not persist in that case). Create the cert with:
#   macapp/make-signing-cert.sh   (self-signed, login keychain, no Apple needed)
SIGN_ID="${SIRSI_SIGN_IDENTITY:-Sirsi Local Code Signing}"
if security find-identity -p codesigning 2>/dev/null | grep -q "$SIGN_ID"; then
	codesign --force --deep --sign "$SIGN_ID" --identifier "$BUNDLE_ID" "$APP" >/dev/null 2>&1 \
		&& echo "▸ signed with: $SIGN_ID"
else
	echo "⚠ signing identity '$SIGN_ID' not found — falling back to ad-hoc (FDA will NOT persist)"
	codesign --force --deep --sign - --identifier "$BUNDLE_ID" "$APP" >/dev/null 2>&1 || true
fi
codesign -dv "$APP" 2>&1 | grep -i 'Identifier=' || true
echo "▸ built $APP"
