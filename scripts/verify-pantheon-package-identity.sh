#!/usr/bin/env bash
# Verify the assembled Pantheon app identity before DMG creation/notarization.
set -euo pipefail

if [[ $# -ne 4 ]]; then
    echo "usage: $0 APP_PATH EXPECTED_VERSION EXPECTED_BUILD ad-hoc|developer-id" >&2
    exit 2
fi

APP="$1"
EXPECTED_VERSION="$2"
EXPECTED_BUILD="$3"
SIGNING_MODE="$4"
PLIST="${APP}/Contents/Info.plist"
CLI="${APP}/Contents/MacOS/sirsi"
MENUBAR="${APP}/Contents/MacOS/sirsi-menubar"

fail() {
    echo "pantheon_package_identity accepted=false reason=$1" >&2
    exit 1
}

[[ -d "${APP}" ]] || fail "missing_app"
[[ -f "${PLIST}" ]] || fail "missing_info_plist"
[[ -x "${CLI}" ]] || fail "missing_embedded_cli"
[[ -x "${MENUBAR}" ]] || fail "missing_control_engine"
[[ "${EXPECTED_BUILD}" =~ ^[0-9]+([.][0-9]+){0,2}$ ]] || fail "invalid_expected_build"

BUNDLE_ID="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "${PLIST}")"
BUNDLE_VERSION="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "${PLIST}")"
BUNDLE_BUILD="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleVersion' "${PLIST}")"

[[ "${BUNDLE_ID}" == "ai.sirsi.pantheon" ]] || fail "bundle_id_mismatch"
[[ "${BUNDLE_VERSION}" == "${EXPECTED_VERSION}" ]] || fail "bundle_version_mismatch"
[[ "${BUNDLE_BUILD}" == "${EXPECTED_BUILD}" ]] || fail "bundle_build_mismatch"

CLI_VERSION="$(${CLI} version 2>&1)" || fail "cli_version_failed"
grep -Fq "Sirsi Pantheon v${EXPECTED_VERSION}" <<<"${CLI_VERSION}" || fail "cli_version_mismatch"
codesign --verify --deep --strict "${APP}" >/dev/null 2>&1 || fail "invalid_signature"

case "${SIGNING_MODE}" in
    ad-hoc)
        SIGNATURE="$(codesign -dv --verbose=4 "${APP}" 2>&1)"
        grep -Fq 'Signature=adhoc' <<<"${SIGNATURE}" || fail "expected_ad_hoc_signature"
        ;;
    developer-id)
        SIGNATURE="$(codesign -dv --verbose=4 "${APP}" 2>&1)"
        grep -Fq 'Authority=Developer ID Application: Sirsi Technologies Inc. (9D382WV988)' <<<"${SIGNATURE}" || fail "developer_id_authority_mismatch"
        grep -Fq 'TeamIdentifier=9D382WV988' <<<"${SIGNATURE}" || fail "team_identifier_mismatch"
        ;;
    *) fail "invalid_signing_mode" ;;
esac

echo "pantheon_package_identity accepted=true bundle_id=${BUNDLE_ID} version=${BUNDLE_VERSION} build=${BUNDLE_BUILD} signing=${SIGNING_MODE}"
