#!/usr/bin/env bash
# Fail closed if a release path can silently substitute a UI-only executable for
# Pantheon's canonical local control engine.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DMG_SCRIPT="${ROOT}/scripts/build-dmg.sh"
RELEASE_WORKFLOW="${ROOT}/.github/workflows/release.yml"
SWIFT_APP_DELEGATE="${ROOT}/macapp/Sources/SirsiMenubar/AppDelegate.swift"
SWIFT_ENGINE="${ROOT}/macapp/Sources/SirsiMenubar/SirsiEngine.swift"
SWIFT_VIEWS="${ROOT}/macapp/Sources/SirsiMenubar/Views.swift"

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

require_order() {
    local earlier="$1" later="$2" label="$3"
    local earlier_line later_line
    earlier_line="$(grep -n -m1 -E "$earlier" "$DMG_SCRIPT" | cut -d: -f1)"
    later_line="$(grep -n -m1 -E "$later" "$DMG_SCRIPT" | cut -d: -f1)"
    if (( earlier_line >= later_line )); then
        echo "menubar_release_contract accepted=false order=${label} file=${DMG_SCRIPT}" >&2
        exit 1
    fi
}

require 'go build .*\./cmd/sirsi-menubar/' "$DMG_SCRIPT" "dmg_go_control_engine"
require 'CGO_ENABLED=1 .*\./cmd/sirsi/' "$DMG_SCRIPT" "dmg_native_vitals_cli"
require 'PlistBuddy.*CFBundleShortVersionString' "$DMG_SCRIPT" "embedded_marketing_version"
require 'PlistBuddy.*CFBundleVersion' "$DMG_SCRIPT" "embedded_build_version"
require 'verify-pantheon-package-identity\.sh' "$DMG_SCRIPT" "assembled_artifact_identity_gate"
require 'PACKAGE_INVENTORY_ENGINE=.*sirsi-package-inventory' "$DMG_SCRIPT" "isolated_go_inventory_engine"
require 'go build .*\./cmd/sirsi/' "$DMG_SCRIPT" "go_native_package_inventory_build"
require '"\$\{PACKAGE_INVENTORY_ENGINE\}" package-inventory' "$DMG_SCRIPT" "go_native_package_inventory_execution"
require 'cmd/sirsi-menubar/bundle/ai\.sirsi\.pantheon\.plist' "$DMG_SCRIPT" "canonical_launch_agent_reference"
require '\-\-require-code-signature' "$DMG_SCRIPT" "signed_payload_inventory"
require_order 'verify-pantheon-package-identity\.sh' 'PACKAGE_INVENTORY_ENGINE=.*sirsi-package-inventory' "package_identity_before_inventory"
require_order 'PACKAGE_INVENTORY_ENGINE=.*sirsi-package-inventory' 'go build.*PACKAGE_INVENTORY_ENGINE.*\./cmd/sirsi/' "inventory_helper_build_after_path_binding"
require_order 'go build.*PACKAGE_INVENTORY_ENGINE.*\./cmd/sirsi/' '"\$\{PACKAGE_INVENTORY_ENGINE\}" package-inventory' "inventory_helper_built_before_execution"
require_order '"\$\{PACKAGE_INVENTORY_ENGINE\}" package-inventory' 'hdiutil create .*srcfolder' "payload_inventory_before_dmg_creation"
require 'REQUIRE_RELEASE_SIGNING' "$DMG_SCRIPT" "release_signing_fail_closed"
require '\./cmd/sirsi-menubar/' "$RELEASE_WORKFLOW" "standalone_go_control_engine"
require 'BUILD_NUMBER:.*github\.run_number.*github\.run_attempt' "$RELEASE_WORKFLOW" "deterministic_ci_build_identity"
require 'REQUIRE_RELEASE_SIGNING:.*1' "$RELEASE_WORKFLOW" "ci_release_signing_required"
reject 'swift build|macapp/\.build/release/SirsiMenubar' "$DMG_SCRIPT" "conditional_swift_substitution"

# A resident Pantheon surface may render persisted projections, but it must not
# launch full diagnostics merely because it started, opened, or reached a timer
# tick. Those probes can cross protected macOS locations and provoke TCC UI.
# Explicit Re-check remains permitted and refreshes the durable snapshot.
require 'health-snapshot\.json' "$SWIFT_ENGINE" "persisted_health_projection"
require 'diagnose\(force: true\)' "$SWIFT_VIEWS" "explicit_diagnostic_action"
reject 'await engine\.diagnose\(\)' "$SWIFT_APP_DELEGATE" "ambient_launch_diagnostics"
reject 'await self\?\.engine\.diagnose' "$SWIFT_APP_DELEGATE" "ambient_timer_diagnostics"
reject 'await engine\.diagnose\(\); await engine\.loadRouterBoard' "$SWIFT_VIEWS" "ambient_panel_open_diagnostics"

echo "menubar_release_contract accepted=true canonical_entrypoint=cmd/sirsi-menubar channels=dmg,standalone permission_silence=persisted_projection"
