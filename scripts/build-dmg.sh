#!/bin/bash
# build-dmg.sh — Build a signed, notarized macOS DMG for Sirsi Pantheon.
# Usage: scripts/build-dmg.sh (--development | --release) [--version VERSION] [--arch ARCH]
# Requires macOS (hdiutil/codesign/notarytool are macOS-specific).
#
# Signing & notarization (the clean-install / no-FDA-churn contract):
#   - --release requires Developer ID Application signing, a notarization
#     credential set, stapling, and validation. It is the only mode allowed to
#     create a commercial-release filename.
#   - --development deliberately creates a -dev artifact and signs ad-hoc. It
#     is useful for local package work, but must never be presented as a release.
#
# Required environment / CI secrets for a real release:
#   DEVELOPER_ID_APPLICATION  e.g. "Developer ID Application: Sirsi … (TEAMID)"
#   APPLE_ID                  the Apple ID email used for notarization
#   APPLE_TEAM_ID             the 10-char Apple Developer Team ID
#   APPLE_APP_PASSWORD        an app-specific password for that Apple ID
#                             (appleid.apple.com → Sign-In & Security → App-Specific Passwords)
# The cert itself is imported into the build keychain by the CI workflow before
# this script runs (MACOS_CERTIFICATE / MACOS_CERTIFICATE_PWD).

set -euo pipefail

# Build artifacts must not depend on a caller's shell search path. In
# particular, a project-local PATH can shadow or hide the compiler, package,
# and macOS signing tools. These are the standard macOS/Homebrew locations;
# no current-directory or caller-provided directory is inherited.
export PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"

# --- Defaults ---
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="$(tr -d '\n' < "${PROJECT_ROOT}/VERSION")"
ARCH="arm64"
MODE=""
BUILD_DIR="${PROJECT_ROOT}/bin"
APP_NAME="Pantheon.app"
BUNDLE_DIR="${PROJECT_ROOT}/${APP_NAME}"
DMG_VOLUME="Sirsi Pantheon"
GO_LDFLAGS="-s -w -X github.com/SirsiMaster/sirsi-pantheon/internal/version.Version=v${VERSION}"

# --- Parse flags ---
while [[ $# -gt 0 ]]; do
    case "$1" in
        --development)
            [[ -z "$MODE" ]] || { echo "ERROR: choose exactly one of --development or --release" >&2; exit 2; }
            MODE="development"; shift ;;
        --release)
            [[ -z "$MODE" ]] || { echo "ERROR: choose exactly one of --development or --release" >&2; exit 2; }
            MODE="release"; shift ;;
        --version) VERSION="$2"; GO_LDFLAGS="-s -w -X github.com/SirsiMaster/sirsi-pantheon/internal/version.Version=v${VERSION}"; shift 2 ;;
        --arch)    ARCH="$2"; shift 2 ;;
        *) echo "Unknown flag: $1"; echo "Usage: $0 (--development | --release) [--version VERSION] [--arch ARCH]"; exit 2 ;;
    esac
done

[[ -n "$MODE" ]] || { echo "ERROR: choose --development or --release explicitly" >&2; exit 2; }
if [[ "$MODE" == "release" ]]; then
    for required in DEVELOPER_ID_APPLICATION APPLE_ID APPLE_TEAM_ID APPLE_APP_PASSWORD; do
        [[ -n "${!required:-}" ]] || { echo "ERROR: --release requires ${required}" >&2; exit 2; }
    done
    DMG_NAME="SirsiPantheon-${VERSION}-${ARCH}.dmg"
    ARTIFACT_LABEL="Commercial release"
else
    DMG_NAME="SirsiPantheon-${VERSION}-dev-${ARCH}.dmg"
    ARTIFACT_LABEL="Development"
fi
DMG_PATH="${BUILD_DIR}/${DMG_NAME}"
STAGING_DIR="${BUILD_DIR}/dmg-staging"

echo "Building Sirsi Pantheon ${MODE} DMG  (version ${VERSION}, arch ${ARCH})"

if [[ "$(uname -s)" != "Darwin" ]]; then
    echo "ERROR: DMG creation requires macOS."; exit 1
fi
mkdir -p "${BUILD_DIR}"

# --- Build the menu bar app ---
# Prefer the native SwiftUI surface (macapp/, ADR-030) when present; fall back to
# the legacy fyne systray binary otherwise. Either way the executable lands at
# Contents/MacOS/sirsi-menubar so the Info.plist (CFBundleExecutable) is stable.
if [[ -f "${PROJECT_ROOT}/macapp/Package.swift" ]]; then
    echo "Compiling native menu bar app (macapp/, SwiftUI)..."
    ( cd "${PROJECT_ROOT}/macapp" && swift build -c release )
    cp "${PROJECT_ROOT}/macapp/.build/release/SirsiMenubar" "${BUILD_DIR}/sirsi-menubar"
else
    echo "Compiling legacy menu bar app (cmd/sirsi-menubar/)..."
    CGO_ENABLED=1 GOARCH="${ARCH}" go build -ldflags="${GO_LDFLAGS}" -o "${BUILD_DIR}/sirsi-menubar" ./cmd/sirsi-menubar/
fi

echo "Compiling sirsi CLI..."
# The macOS CLI links the native vitals surface; disabling CGO here produces a
# binary that cannot compile the same product that `make build` validates.
CGO_ENABLED=1 GOARCH="${ARCH}" go build -ldflags="${GO_LDFLAGS}" -o "${BUILD_DIR}/sirsi" ./cmd/sirsi/

# --- Assemble the .app bundle ---
echo "Assembling ${APP_NAME}..."
rm -rf "${BUNDLE_DIR}"
mkdir -p "${BUNDLE_DIR}/Contents/MacOS" "${BUNDLE_DIR}/Contents/Resources"
cp "${BUILD_DIR}/sirsi-menubar" "${BUNDLE_DIR}/Contents/MacOS/sirsi-menubar"
cp "${BUILD_DIR}/sirsi"         "${BUNDLE_DIR}/Contents/MacOS/sirsi"
cp "${PROJECT_ROOT}/cmd/sirsi-menubar/bundle/Info.plist" "${BUNDLE_DIR}/Contents/Info.plist"
cp "${PROJECT_ROOT}/cmd/sirsi-menubar/bundle/PkgInfo"    "${BUNDLE_DIR}/Contents/PkgInfo"
cp "${PROJECT_ROOT}/cmd/sirsi-menubar/bundle/ai.sirsi.pantheon.plist" "${BUNDLE_DIR}/Contents/Resources/ai.sirsi.pantheon.plist"
# Stack Lab is a shipped, inspectable recipe surface rather than build-only
# documentation.  Keep its contracts alongside the app they describe.
cp -R "${PROJECT_ROOT}/contracts/stacklab" "${BUNDLE_DIR}/Contents/Resources/StackLab"
# Never ship the historical template version from the source plist.  The
# package and both executables are built from the requested release version.
/usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString ${VERSION}" "${BUNDLE_DIR}/Contents/Info.plist"
/usr/libexec/PlistBuddy -c "Set :CFBundleVersion ${VERSION}" "${BUNDLE_DIR}/Contents/Info.plist"
# AppleDouble sidecars are transport metadata, never product resources.  They
# can appear when a source tree crosses volumes; remove them before signing so
# a DMG and PKG cannot silently ship hidden duplicate payload entries.
/usr/bin/find "${BUNDLE_DIR}" -type f -name '._*' -delete
if /usr/bin/find "${BUNDLE_DIR}" -type f -name '._*' -print -quit | /usr/bin/grep -q .; then
    echo "ERROR: AppleDouble metadata remains in application bundle." >&2
    exit 1
fi

# --- Code signing ---
if [[ "$MODE" == "release" ]]; then
    echo "Signing with Developer ID: ${DEVELOPER_ID_APPLICATION}"
    # Sign inner executables first (inside-out), then the bundle — more robust for
    # notarization than a single --deep pass. Hardened runtime + secure timestamp.
    for inner in "${BUNDLE_DIR}/Contents/MacOS/sirsi" "${BUNDLE_DIR}/Contents/MacOS/sirsi-menubar"; do
        codesign --force --options runtime --timestamp --sign "${DEVELOPER_ID_APPLICATION}" "${inner}"
    done
    codesign --force --options runtime --timestamp --sign "${DEVELOPER_ID_APPLICATION}" "${BUNDLE_DIR}"
    codesign --verify --deep --strict --verbose=2 "${BUNDLE_DIR}"
else
    echo "Signing ad-hoc development bundle (not distributable)."
    codesign --force --deep --sign - "${BUNDLE_DIR}"
fi

# --- Stage + create the DMG ---
echo "Creating DMG..."
rm -rf "${STAGING_DIR}"; mkdir -p "${STAGING_DIR}"
cp -R "${BUNDLE_DIR}" "${STAGING_DIR}/"
ln -s /Applications "${STAGING_DIR}/Applications"
if [[ "$MODE" == "release" ]]; then
    BUILD_CLASS="Commercial release: Developer ID signed, notarized, and stapled."
else
    BUILD_CLASS="Development build: ad-hoc signed and not for distribution."
fi
cat > "${STAGING_DIR}/README.txt" <<READMEEOF
Sirsi Pantheon — Unified DevOps Intelligence Platform

${BUILD_CLASS}

INSTALL
  1. Drag Pantheon.app into Applications.
  2. Launch it; grant Full Disk Access when prompted (one time).

The bundle includes the menu bar app and the sirsi CLI
(/Applications/Pantheon.app/Contents/MacOS/sirsi). To use the CLI in a terminal:
  alias sirsi="/Applications/Pantheon.app/Contents/MacOS/sirsi"
or: brew install sirsimaster/tools/sirsi-pantheon

More: https://sirsi.ai/pantheon
READMEEOF

rm -f "${DMG_PATH}"
hdiutil create -volname "${DMG_VOLUME}" -srcfolder "${STAGING_DIR}" -ov -format UDZO "${DMG_PATH}"
rm -rf "${STAGING_DIR}"

# --- Sign + notarize + staple the DMG (release builds only, AFTER it exists) ---
if [[ "$MODE" == "release" ]]; then
    codesign --force --timestamp --sign "${DEVELOPER_ID_APPLICATION}" "${DMG_PATH}"
    echo "Notarizing ${DMG_NAME} (this can take a few minutes)..."
    # --timeout bounds the --wait poll so a stuck Apple-notary submission (or
    # a bad credential that never resolves) fails the step instead of hanging;
    # the release.yml job-level timeout-minutes is the outer backstop.
    xcrun notarytool submit "${DMG_PATH}" \
        --apple-id "${APPLE_ID}" \
        --team-id "${APPLE_TEAM_ID}" \
        --password "${APPLE_APP_PASSWORD}" \
        --timeout 20m \
        --wait
    echo "Stapling notarization ticket..."
    xcrun stapler staple "${DMG_PATH}"
    xcrun stapler validate "${DMG_PATH}"
fi

echo ""
echo "${ARTIFACT_LABEL} DMG created: ${DMG_PATH}"
ls -lh "${DMG_PATH}"
