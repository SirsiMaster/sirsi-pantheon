#!/usr/bin/env bash
# Static, signing-independent contract for the Pantheon unsigned release path.
# This deliberately inspects source only: it must never invoke codesign,
# hdiutil, launchd, a service, or an installed product.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd -P)"
fail() {
    echo "pantheon_unsigned_release_source accepted=false reason=$1" >&2
    exit 1
}
require_file() {
    [[ -f "$1" ]] || fail "missing_file:${1#"$ROOT/"}"
}
require_text() {
    local pattern="$1" file="$2" label="$3"
    grep -Eq "$pattern" "$file" || fail "missing_${label}:${file#"$ROOT/"}"
}
reject_text() {
    local pattern="$1" file="$2" label="$3"
    ! grep -Eq "$pattern" "$file" || fail "forbidden_${label}:${file#"$ROOT/"}"
}

DMG="$ROOT/scripts/build-dmg.sh"
MENUBAR_CONTRACT="$ROOT/scripts/verify-menubar-release-contract.sh"
PACKAGE_IDENTITY="$ROOT/scripts/verify-pantheon-package-identity.sh"
WORKFLOW="$ROOT/.github/workflows/release.yml"
for required in "$DMG" "$MENUBAR_CONTRACT" "$PACKAGE_IDENTITY" "$WORKFLOW" \
    "$ROOT/cmd/sirsi-menubar/bundle/Info.plist" \
    "$ROOT/cmd/sirsi-menubar/bundle/PkgInfo" \
    "$ROOT/cmd/sirsi-menubar/bundle/ai.sirsi.pantheon.plist"; do
    require_file "$required"
done

# Assembly is isolated and creates both resource and executable parents before
# any copy/build. This is the source-level guard for the historical ENOENT gate.
require_text 'BUILD_WORK_DIR=.*mktemp -d' "$DMG" isolated_workspace
require_text 'mkdir -p .*Contents/MacOS.*Contents/Resources' "$DMG" resource_parent_creation
require_text 'go build .*\./cmd/sirsi-menubar/' "$DMG" canonical_menubar_build
require_text 'go build .*\./cmd/sirsi/' "$DMG" canonical_cli_build
require_text 'verify-pantheon-package-identity\.sh' "$DMG" package_identity_gate
require_text 'REQUIRE_RELEASE_SIGNING' "$DMG" signing_fail_closed
require_text 'hdiutil create .*srcfolder' "$DMG" isolated_dmg_source
require_text 'verify-menubar-release-contract\.sh' "$DMG" menubar_contract_gate

# The release workflow must use the same canonical Go entrypoints and must not
# silently substitute the Swift prototype or a mutable shared executable.
require_text '\./cmd/sirsi-menubar/' "$WORKFLOW" workflow_menubar_build
reject_text 'swift build|macapp/\.build/release/SirsiMenubar' "$DMG" swift_payload_substitution
reject_text 'python(3)?|pip3|\.pyc|libpython' "$DMG" python_product_dependency

# Keep the source fixture itself honest: any README/resource write has an
# explicit parent creation, matching the production isolated-workspace rule.
fixture_root="$(mktemp -d "${TMPDIR:-/private/tmp}/pantheon-unsigned-source.XXXXXX")"
trap 'rm -rf "$fixture_root"' EXIT
mkdir -p "$fixture_root/scripts/resources"
printf '%s\n' 'fixture' > "$fixture_root/scripts/resources/Pantheon-DMG-README.txt"
[[ -s "$fixture_root/scripts/resources/Pantheon-DMG-README.txt" ]] || fail "resource_fixture_write"

echo "pantheon_unsigned_release_source accepted=true isolated_workspace=required canonical_engines=2 signing=external_gate python_payload=forbidden"
