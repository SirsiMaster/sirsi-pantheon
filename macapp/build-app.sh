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
ALLOW_ADHOC="${SIRSI_ALLOW_ADHOC:-0}"   # set 1 to permit ad-hoc (FDA will NOT persist)
if security find-identity -p codesigning 2>/dev/null | grep -q "$SIGN_ID"; then
	if ! codesign --force --deep --sign "$SIGN_ID" --identifier "$BUNDLE_ID" "$APP" 2>&1; then
		echo "✘ codesign with '$SIGN_ID' FAILED (keychain locked, or codesign not authorized for the key)." >&2
		echo "  Fix: unlock the login keychain and click 'Always Allow' once, or run:" >&2
		echo "    security set-key-partition-list -S apple-tool:,apple: -s -k <login-pw> ~/Library/Keychains/login.keychain-db" >&2
		exit 1
	fi
	echo "▸ signed with: $SIGN_ID"
elif [ "$ALLOW_ADHOC" = "1" ]; then
	echo "⚠ '$SIGN_ID' not found — ad-hoc signing (SIRSI_ALLOW_ADHOC=1). FDA will NOT persist." >&2
	codesign --force --deep --sign - --identifier "$BUNDLE_ID" "$APP" >/dev/null 2>&1 || true
else
	echo "✘ signing identity '$SIGN_ID' not found. Create it with macapp/make-signing-cert.sh," >&2
	echo "  or set SIRSI_ALLOW_ADHOC=1 to ship ad-hoc (FDA will not persist). Refusing to ship ad-hoc silently." >&2
	exit 1
fi

# Fail loud if the result is not the stable cert-based requirement: an ad-hoc or
# cdhash-only DR means TCC will drop the FDA grant on relaunch. This guard is why
# a backgrounded build can never silently regress to ad-hoc again.
DR="$(codesign -d --requirements - "$APP" 2>&1 | grep designated || true)"
if [ "$ALLOW_ADHOC" != "1" ] && ! printf '%s' "$DR" | grep -q 'certificate leaf'; then
	echo "✘ post-sign check FAILED — designated requirement is not certificate-based:" >&2
	echo "    $DR" >&2
	echo "  TCC would drop FDA on relaunch. Aborting." >&2
	exit 1
fi
codesign -dv "$APP" 2>&1 | grep -i 'Identifier=' || true
echo "▸ built $APP  (DR: ${DR#*designated => })"
