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

require_text() {
    local pattern="$1" text="$2" label="$3"
    if ! grep -Eq "$pattern" <<< "$text"; then
        echo "menubar_release_contract accepted=false missing=${label} file=${RELEASE_WORKFLOW}" >&2
        exit 1
    fi
}

require_step_line() {
    local line="$1" label="$2"
    if ! grep -Fqx -- "$line" <<< "$FIRST_STEP_ACTIVE"; then
        echo "menubar_release_contract accepted=false missing=${label} file=${RELEASE_WORKFLOW}" >&2
        exit 1
    fi
}

require_run_line() {
    local line="$1" label="$2"
    if ! grep -Fqx -- "$line" <<< "$FIRST_STEP_RUN_ACTIVE"; then
        echo "menubar_release_contract accepted=false missing=${label} file=${RELEASE_WORKFLOW}" >&2
        exit 1
    fi
}

# Read only the real jobs.menubar YAML block. Comments and similarly named
# steps in the source-verification or publisher jobs cannot satisfy this gate.
MENUBAR_JOB="$(awk '
    /^  menubar:$/ { in_job = 1; next }
    in_job && /^  [[:alnum:]_-]+:$/ { exit }
    in_job { print }
' "$RELEASE_WORKFLOW")"
if [[ -z "$MENUBAR_JOB" ]]; then
    echo "menubar_release_contract accepted=false missing=jobs.menubar file=${RELEASE_WORKFLOW}" >&2
    exit 1
fi
require_text '^    runs-on: \[self-hosted, macOS, ARM64, m1\]$' "$MENUBAR_JOB" "qualified_m1_runner_label"

FIRST_STEP="$(awk '
    /^    steps:$/ { in_steps = 1; next }
    in_steps && /^      - / { if (seen_step) exit; seen_step = 1 }
    in_steps && seen_step { print }
' <<< "$MENUBAR_JOB")"
FIRST_STEP_ACTIVE="$(sed '/^[[:space:]]*#/d' <<< "$FIRST_STEP")"
FIRST_STEP_RUN="$(awk '
    /^        run: \|$/ { in_run = 1; next }
    in_run && /^      - / { exit }
    in_run { print }
' <<< "$FIRST_STEP")"
FIRST_STEP_RUN_ACTIVE="$(sed '/^[[:space:]]*#/d' <<< "$FIRST_STEP_RUN")"
require_text '^      - name: Bind native release to qualified M1 runner$' "$FIRST_STEP_ACTIVE" "first_step_m1_binding_guard"
require_step_line '        run: |' "inline_m1_binding_guard"
STEP_FIELDS="$(awk '/^        [[:alnum:]_-]+:/ { print }' <<< "$FIRST_STEP_ACTIVE")"
EXPECTED_STEP_FIELDS="$(printf '        env:\n        run: |')"
if [[ "$STEP_FIELDS" != "$EXPECTED_STEP_FIELDS" ]]; then
    echo "menubar_release_contract accepted=false unexpected_first_guard_step_field file=${RELEASE_WORKFLOW}" >&2
    exit 1
fi
STEP_ENV_BLOCK="$(awk '
    /^        env:$/ { in_env = 1; next }
    in_env && /^        [[:alnum:]_-]+:/ { exit }
    in_env { print }
' <<< "$FIRST_STEP_ACTIVE")"
EXPECTED_STEP_ENV_BLOCK="$(printf '          PANTHEON_RUNNER_NAME: ${{ runner.name }}\n          PANTHEON_RUNNER_OS: ${{ runner.os }}\n          PANTHEON_RUNNER_ARCH: ${{ runner.arch }}')"
if [[ "$STEP_ENV_BLOCK" != "$EXPECTED_STEP_ENV_BLOCK" ]]; then
    echo "menubar_release_contract accepted=false unexpected_first_guard_environment file=${RELEASE_WORKFLOW}" >&2
    exit 1
fi
RUN_COMMAND_COUNT="$(awk 'NF && $1 !~ /^#/ { count++ } END { print count + 0 }' <<< "$FIRST_STEP_RUN_ACTIVE")"
if [[ "$RUN_COMMAND_COUNT" != 8 ]]; then
    echo "menubar_release_contract accepted=false unexpected_first_guard_commands count=${RUN_COMMAND_COUNT}" >&2
    exit 1
fi
require_run_line '          set -euo pipefail' "fail_closed_binding_shell"
require_run_line '          [[ "${PANTHEON_RUNNER_NAME:-}" == "m1-pantheon" ]] || { echo "requires runner m1-pantheon" >&2; exit 1; }' "pinned_runner_name_check"
require_run_line '          [[ "${PANTHEON_RUNNER_OS:-}" == "macOS" ]] || { echo "requires GitHub macOS runner context" >&2; exit 1; }' "pinned_runner_os_check"
require_run_line '          [[ "${PANTHEON_RUNNER_ARCH:-}" == "ARM64" ]] || { echo "requires GitHub ARM64 runner context" >&2; exit 1; }' "pinned_runner_arch_check"
require_run_line '          [[ "$(/usr/bin/uname -s)" == "Darwin" ]] || { echo "requires native Darwin" >&2; exit 1; }' "native_darwin_check"
require_run_line '          [[ "$(/usr/bin/uname -m)" == "arm64" ]] || { echo "requires native arm64" >&2; exit 1; }' "native_arm64_check"
require_run_line '          ACTUAL_HW_MODEL=$(/usr/sbin/sysctl -n hw.model) || { echo "cannot read native hardware model" >&2; exit 1; }' "native_model_read"
require_run_line '          [[ "$ACTUAL_HW_MODEL" == "MacBookPro18,1" ]] || { echo "requires qualified M1 model MacBookPro18,1" >&2; exit 1; }' "qualified_m1_model_check"

require '^PANTHEON_BUILD_JOBS="\$\{PANTHEON_BUILD_JOBS:-1\}"' "$DMG_SCRIPT" "native_swift_job_default"
require '^\s*\( cd "\$\{PROJECT_ROOT\}/macapp" && swift build -c release --jobs "\$\{PANTHEON_BUILD_JOBS\}" \)' "$DMG_SCRIPT" "native_swift_release_build"
require 'cp "\$\{PROJECT_ROOT\}/macapp/\.build/release/SirsiMenubar"' "$DMG_SCRIPT" "native_swift_payload"
require 'CGO_ENABLED=1 GOARCH="\$\{ARCH\}" go build' "$DMG_SCRIPT" "go_cli_payload"
require 'package-inventory' "$DMG_SCRIPT" "payload_inventory"
require 'hdiutil create' "$DMG_SCRIPT" "dmg_creation"
require 'codesign --force --options runtime' "$DMG_SCRIPT" "release_signing"
require 'verify-menubar-release-contract\.sh' "$RELEASE_WORKFLOW" "workflow_contract_gate"
require 'build-dmg\.sh --release' "$RELEASE_WORKFLOW" "commercial_release_mode"
require 'test -x "\$HOME/\.local/bin/sirsi-maat-run"' "$RELEASE_WORKFLOW" "native_release_installed_preflight"
require '"\$HOME/\.local/bin/sirsi-maat-run" --agent codex-pantheon' "$RELEASE_WORKFLOW" "native_release_admission"
reject 'python3 scripts/maat/admitted-run\.py --agent codex-pantheon' "$RELEASE_WORKFLOW" "future_workspace_supervisor_bootstrap"
require '\-\-register-executor --work' "$RELEASE_WORKFLOW" "native_release_executor"
require '\-\-cpu-cores 1 --timeout 1500' "$RELEASE_WORKFLOW" "native_release_cpu_bound"
reject 'macapp/\.build/release/SirsiMenubar.*fallback|go build.*fallback' "$DMG_SCRIPT" "build_fallback"

# Exercise negative controls as well as line-presence checks; this executes no
# signing or resource workload and catches payloads moved outside supervision.
PYTHONDONTWRITEBYTECODE=1 python3 "$ROOT/scripts/maat/test_release_admission.py"
PYTHONDONTWRITEBYTECODE=1 python3 "$ROOT/scripts/test_release_publication.py"

echo "menubar_release_contract accepted=true native_swift=required go_cli=required inventory=required signing=required"
