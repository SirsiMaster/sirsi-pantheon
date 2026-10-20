#!/usr/bin/env bash
# Verify the release workflow still builds the canonical native menu-bar app
# and the bundled Go CLI before signing and publishing a DMG.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DMG_SCRIPT="$ROOT/scripts/build-dmg.sh"
RELEASE_WORKFLOW="$ROOT/.github/workflows/release.yml"

require() {
    local pattern="$1" file="$2" label="$3"
    if ! grep -Eq "$pattern" "$file"; then
        echo "menubar_release_contract accepted=false missing=${label} file=${file}" >&2
        exit 1
    fi
}

reject() {
    local pattern="$1" file="$2" label="$3"
    if grep -Eq "$pattern" "$file"; then
        echo "menubar_release_contract accepted=false forbidden=${label} file=${file}" >&2
        exit 1
    fi
}

require '^SWIFT_BUILD_ARGS=\(-c release\)' "$DMG_SCRIPT" "native_swift_release_configuration"
require '^\s*\( cd "\$\{PROJECT_ROOT\}/macapp" && swift build "\$\{SWIFT_BUILD_ARGS\[@\]\}" \)' "$DMG_SCRIPT" "native_swift_release_build"
require 'cp "\$\{PROJECT_ROOT\}/macapp/\.build/release/SirsiMenubar"' "$DMG_SCRIPT" "native_swift_payload"
require 'CGO_ENABLED=1 GOARCH="\$\{ARCH\}" go build' "$DMG_SCRIPT" "go_cli_payload"
require 'package-inventory' "$DMG_SCRIPT" "payload_inventory"
require 'hdiutil create' "$DMG_SCRIPT" "dmg_creation"
require 'codesign --force --options runtime' "$DMG_SCRIPT" "release_signing"
require 'verify-menubar-release-contract\.sh' "$RELEASE_WORKFLOW" "workflow_contract_gate"
require 'build-dmg\.sh --release' "$RELEASE_WORKFLOW" "commercial_release_mode"
require 'python3 scripts/maat/admitted-run\.py --agent codex-pantheon' "$RELEASE_WORKFLOW" "native_release_admission"
require '\-\-register-executor --work' "$RELEASE_WORKFLOW" "native_release_executor"
require '\-\-cpu-cores 1 --timeout 1500' "$RELEASE_WORKFLOW" "native_release_cpu_bound"
reject 'macapp/\.build/release/SirsiMenubar.*fallback|go build.*fallback' "$DMG_SCRIPT" "build_fallback"

echo "menubar_release_contract accepted=true native_swift=required go_cli=required inventory=required signing=required"
