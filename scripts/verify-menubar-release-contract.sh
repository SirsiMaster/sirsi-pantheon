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

require 'go build .*\./cmd/sirsi-menubar/' "$DMG_SCRIPT" "dmg_go_control_engine"
require 'CGO_ENABLED=1 .*\./cmd/sirsi/' "$DMG_SCRIPT" "dmg_native_vitals_cli"
require 'PlistBuddy.*CFBundleShortVersionString' "$DMG_SCRIPT" "embedded_marketing_version"
require 'PlistBuddy.*CFBundleVersion' "$DMG_SCRIPT" "embedded_build_version"
require 'verify-pantheon-package-identity\.sh' "$DMG_SCRIPT" "assembled_artifact_identity_gate"
require 'REQUIRE_RELEASE_SIGNING' "$DMG_SCRIPT" "release_signing_fail_closed"
require 'REQUIRE_RELEASE_NOTARIZATION' "$DMG_SCRIPT" "release_notarization_fail_closed"
require '\./cmd/sirsi-menubar/' "$RELEASE_WORKFLOW" "standalone_go_control_engine"
require 'BUILD_NUMBER:.*github\.run_number.*github\.run_attempt' "$RELEASE_WORKFLOW" "deterministic_ci_build_identity"
require 'REQUIRE_RELEASE_SIGNING:.*1' "$RELEASE_WORKFLOW" "ci_release_signing_required"
require 'REQUIRE_RELEASE_NOTARIZATION:.*1' "$RELEASE_WORKFLOW" "ci_release_notarization_required"
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
