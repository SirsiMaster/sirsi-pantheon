#!/usr/bin/env bash
# build-dmg.sh — Build a signed, notarized macOS DMG for Sirsi Pantheon.
# Usage: scripts/build-dmg.sh [--version VERSION] [--arch ARCH]
# Requires macOS (hdiutil/codesign/notarytool are macOS-specific).
#
# Signing & notarization (the clean-install / no-FDA-churn contract):
#   - With a Developer ID Application identity in the keychain, the bundle is
#     signed with the HARDENED RUNTIME + a secure timestamp, then the DMG is
#     notarized and stapled. A stable Developer ID = stable TCC identity, so
#     `brew upgrade` keeps the user's Full Disk Access grant (no re-grant, no
#     new FDA row) — the whole reason this pipeline exists.
#   - With no identity, it falls back to ad-hoc (dev-only; Gatekeeper warns,
#     and FDA WILL churn on upgrade — never ship an ad-hoc build to users).
#
# Required environment / CI secrets for a real release:
#   DEVELOPER_ID_APPLICATION  e.g. "Developer ID Application: Sirsi … (TEAMID)"
#   APPLE_ID                  the Apple ID email used for notarization
#   APPLE_TEAM_ID             the 10-char Apple Developer Team ID
#   APPLE_APP_PASSWORD        an app-specific password for that Apple ID
#                             (appleid.apple.com → Sign-In & Security → App-Specific Passwords)
# Or, preferably, App Store Connect API credentials:
#   ASC_KEY_PATH              absolute path to AuthKey_<KEYID>.p8
#   ASC_KEY_ID                App Store Connect key ID
#   ASC_ISSUER_ID             App Store Connect issuer UUID
# The cert itself is imported into the build keychain by the CI workflow before
# this script runs (MACOS_CERTIFICATE / MACOS_CERTIFICATE_PWD).

set -euo pipefail

# --- Defaults ---
VERSION="${VERSION:-0.17.0}"
ARCH="${ARCH:-arm64}"
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BUILD_DIR="${PROJECT_ROOT}/bin"
APP_NAME="Pantheon.app"
DMG_VOLUME="Sirsi Pantheon"

# --- Parse flags ---
while [[ $# -gt 0 ]]; do
    case "$1" in
        --version) VERSION="$2"; shift 2 ;;
        --arch)    ARCH="$2"; shift 2 ;;
        *) echo "Unknown flag: $1"; echo "Usage: $0 [--version VERSION] [--arch ARCH]"; exit 1 ;;
    esac
done

# Derive linker metadata only after every environment/default/flag override has
# been resolved, so executable and bundle identities cannot diverge.
GO_LDFLAGS="-s -w -X github.com/SirsiMaster/sirsi-pantheon/internal/version.Version=v${VERSION}"

BUNDLE_BUILD_NUMBER="${BUILD_NUMBER:-$(date -u +%Y%m%d%H%M)}"
if [[ ! "${BUNDLE_BUILD_NUMBER}" =~ ^[0-9]+([.][0-9]+){0,2}$ ]]; then
    echo "ERROR: BUILD_NUMBER must contain one to three dot-separated numeric components." >&2
    exit 1
fi

DMG_NAME="SirsiPantheon-${VERSION}-${ARCH}.dmg"
DMG_PATH="${BUILD_DIR}/${DMG_NAME}"
BUILD_WORK_DIR=""
BUNDLE_DIR=""
STAGING_DIR=""
DMG_CANDIDATE=""

echo "Building Sirsi Pantheon DMG  (version ${VERSION}, arch ${ARCH})"

if [[ "$(uname -s)" != "Darwin" ]]; then
    echo "ERROR: DMG creation requires macOS."; exit 1
fi
mkdir -p "${BUILD_DIR}"
BUILD_WORK_DIR="$(mktemp -d "${BUILD_DIR}/.pantheon-package-${VERSION}-${BUNDLE_BUILD_NUMBER}.XXXXXX")"
trap 'rm -rf "${BUILD_WORK_DIR}"' EXIT
BUNDLE_DIR="${BUILD_WORK_DIR}/${APP_NAME}"
STAGING_DIR="${BUILD_WORK_DIR}/dmg-staging"
DMG_CANDIDATE="${BUILD_WORK_DIR}/${DMG_NAME}"

# --- Build the canonical menu bar + local control engine ---
# Every distribution channel MUST package the same Go entrypoint. It owns the
# protected loopback API, SNE lifecycle, recovery controller, durable local
# capability, and Nexus handoff. The SwiftUI prototype is a presentation surface
# only; selecting it merely because macapp/Package.swift exists produced a DMG
# that looked native but silently omitted the local control engine while the
# standalone archive retained it. A native surface may replace this executable
# only after it launches the same packaged engine and passes the release contract.
echo "Compiling canonical menu bar and local control engine (cmd/sirsi-menubar/)..."
CGO_ENABLED=1 GOARCH="${ARCH}" go build -ldflags="${GO_LDFLAGS}" -o "${BUILD_DIR}/sirsi-menubar" ./cmd/sirsi-menubar/

echo "Compiling sirsi CLI..."
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

# Embed the requested release identity in the shipped bundle, not only in the
# DMG filename. BUILD_NUMBER is resolved before staging so every attempt has an
# isolated workspace and can never mutate the prior accepted candidate.
/usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString ${VERSION}" "${BUNDLE_DIR}/Contents/Info.plist"
/usr/libexec/PlistBuddy -c "Set :CFBundleVersion ${BUNDLE_BUILD_NUMBER}" "${BUNDLE_DIR}/Contents/Info.plist"
echo "Embedded bundle identity: ${VERSION} (${BUNDLE_BUILD_NUMBER})"

# --- Code signing ---
"${PROJECT_ROOT}/../tools/check_sirsi_macos_permission_contract.sh" "${PROJECT_ROOT}/.."
if [ -n "${DEVELOPER_ID_APPLICATION:-}" ]; then
    echo "Signing with Developer ID: ${DEVELOPER_ID_APPLICATION}"
    LOGIN_KEYCHAIN="${HOME}/Library/Keychains/login.keychain-db"
    if ! security find-identity -v -p codesigning "${LOGIN_KEYCHAIN}" 2>/dev/null | grep -Fq "${DEVELOPER_ID_APPLICATION}"; then
        echo "ERROR: Developer ID identity is not available in the visible login keychain." >&2
        echo "Open Keychain Access, import the certificate and private key into Login, then retry." >&2
        echo "The sudo password cannot authorize a Developer ID private key, and Sirsi will not search hidden release keychains." >&2
        exit 1
    fi
    # Sign inner executables first (inside-out), then the bundle — more robust for
    # notarization than a single --deep pass. Hardened runtime + secure timestamp.
    for inner in "${BUNDLE_DIR}/Contents/MacOS/sirsi" "${BUNDLE_DIR}/Contents/MacOS/sirsi-menubar"; do
        codesign --force --options runtime --timestamp --sign "${DEVELOPER_ID_APPLICATION}" "${inner}"
    done
    codesign --force --options runtime --timestamp --sign "${DEVELOPER_ID_APPLICATION}" "${BUNDLE_DIR}"
    codesign --verify --deep --strict --verbose=2 "${BUNDLE_DIR}"
    SIGNED_FOR_RELEASE=1
else
    echo "Signing ad-hoc (no Developer ID — DEV ONLY; do not distribute)."
    codesign --force --deep --sign - "${BUNDLE_DIR}"
    SIGNED_FOR_RELEASE=0
fi

if [[ "${REQUIRE_RELEASE_SIGNING:-0}" == "1" && "${SIGNED_FOR_RELEASE}" != "1" ]]; then
    echo "ERROR: release signing is required, but no Developer ID Application identity was supplied." >&2
    exit 1
fi

SIGNING_MODE="ad-hoc"
if [[ "${SIGNED_FOR_RELEASE}" == "1" ]]; then
    SIGNING_MODE="developer-id"
fi
"${PROJECT_ROOT}/scripts/verify-pantheon-package-identity.sh" \
    "${BUNDLE_DIR}" "${VERSION}" "${BUNDLE_BUILD_NUMBER}" "${SIGNING_MODE}"

# --- Stage + create the DMG ---
echo "Creating DMG..."
rm -rf "${STAGING_DIR}"; mkdir -p "${STAGING_DIR}"
cp -R "${BUNDLE_DIR}" "${STAGING_DIR}/"
ln -s /Applications "${STAGING_DIR}/Applications"
cat > "${STAGING_DIR}/README.txt" <<'READMEEOF'
Sirsi Pantheon — Unified DevOps Intelligence Platform

INSTALL
  1. Drag Pantheon.app into Applications.
  2. Launch it. Pantheon requests no permissions at startup.
     Enable a capability only when its explicit workflow explains why it is needed.
  3. Enable the optional login caretaker:
     /Applications/Pantheon.app/Contents/MacOS/sirsi surface install gui

The bundle includes the menu bar app and the `sirsi` CLI
(/Applications/Pantheon.app/Contents/MacOS/sirsi). To use the CLI in a terminal:
  alias sirsi="/Applications/Pantheon.app/Contents/MacOS/sirsi"
or: brew install sirsimaster/tools/sirsi-pantheon

More: https://sirsi.ai/pantheon
READMEEOF

hdiutil create -volname "${DMG_VOLUME}" -srcfolder "${STAGING_DIR}" -ov -format UDZO "${DMG_CANDIDATE}"

# --- Sign + notarize + staple the DMG (release builds only, AFTER it exists) ---
if [ "${SIGNED_FOR_RELEASE}" = "1" ]; then
    codesign --force --timestamp --sign "${DEVELOPER_ID_APPLICATION}" "${DMG_CANDIDATE}"
    if [ -n "${ASC_KEY_PATH:-}" ] && [ -n "${ASC_KEY_ID:-}" ] && [ -n "${ASC_ISSUER_ID:-}" ]; then
        if [ ! -f "${ASC_KEY_PATH}" ]; then
            echo "ERROR: ASC_KEY_PATH does not exist: ${ASC_KEY_PATH}" >&2
            exit 1
        fi
        echo "Notarizing ${DMG_NAME} with App Store Connect API credentials..."
        xcrun notarytool submit "${DMG_CANDIDATE}" \
            --key "${ASC_KEY_PATH}" \
            --key-id "${ASC_KEY_ID}" \
            --issuer "${ASC_ISSUER_ID}" \
            --timeout 20m \
            --wait
        echo "Stapling notarization ticket..."
        xcrun stapler staple "${DMG_CANDIDATE}"
        xcrun stapler validate "${DMG_CANDIDATE}"
    elif [ -n "${APPLE_ID:-}" ] && [ -n "${APPLE_TEAM_ID:-}" ] && [ -n "${APPLE_APP_PASSWORD:-}" ]; then
        echo "Notarizing ${DMG_NAME} (this can take a few minutes)..."
        # --timeout bounds the --wait poll so a stuck Apple-notary submission (or
        # a bad credential that never resolves) fails the step instead of hanging;
        # the release.yml job-level timeout-minutes is the outer backstop.
        xcrun notarytool submit "${DMG_CANDIDATE}" \
            --apple-id "${APPLE_ID}" \
            --team-id "${APPLE_TEAM_ID}" \
            --password "${APPLE_APP_PASSWORD}" \
            --timeout 20m \
            --wait
        echo "Stapling notarization ticket..."
        xcrun stapler staple "${DMG_CANDIDATE}"
        xcrun stapler validate "${DMG_CANDIDATE}"
    else
        echo "WARNING: signed but NOT notarized (provide ASC_KEY_PATH/ASC_KEY_ID/ASC_ISSUER_ID or APPLE_ID/APPLE_TEAM_ID/APPLE_APP_PASSWORD)."
    fi
fi

# Promotion is the final operation. A failed compile, signature, image build,
# notarization, or staple leaves the prior accepted app and DMG byte-for-byte
# untouched. Retain a build-numbered app copy for exact post-build inspection.
APP_CANDIDATE="${BUILD_DIR}/Pantheon-${VERSION}-${BUNDLE_BUILD_NUMBER}-${ARCH}.app"
rm -rf "${APP_CANDIDATE}"
cp -R "${BUNDLE_DIR}" "${APP_CANDIDATE}"
mv -f "${DMG_CANDIDATE}" "${DMG_PATH}"

echo ""
echo "DMG created: ${DMG_PATH}"
echo "App candidate: ${APP_CANDIDATE}"
ls -lh "${DMG_PATH}"
