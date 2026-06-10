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

# Fresh inode (never cp-over) then ad-hoc sign — avoids the AMFI stale-cdhash
# SIGKILL-137 class (reference_macos_amfi_cp_sigkill).
cp "$BIN" "$APP/Contents/MacOS/SirsiMenubar"
codesign --force --deep --sign - --identifier "$BUNDLE_ID" "$APP" >/dev/null 2>&1 || true
codesign -dv "$APP" 2>&1 | grep -i 'Identifier=' || true
echo "▸ built $APP"
