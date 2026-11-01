#!/usr/bin/env bash
# Inert workflow-source fixtures; never contact GitHub, Apple, a runner, or signer.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
FIXTURE="$(mktemp -d "${TMPDIR:-/tmp}/pantheon-menubar-runner-contract.XXXXXXXX")"
trap 'rm -rf "$FIXTURE"' EXIT

mkdir -p "$FIXTURE/.github/workflows" "$FIXTURE/scripts/maat" "$FIXTURE/macapp"
cp "$ROOT/.github/workflows/release.yml" "$FIXTURE/.github/workflows/release.yml"
cp "$ROOT/scripts/build-dmg.sh" "$ROOT/scripts/verify-menubar-release-contract.sh" \
   "$ROOT/scripts/test_release_publication.py" "$ROOT/scripts/publish-release-assets.py" "$FIXTURE/scripts/"
cp "$ROOT/scripts/maat/test_release_admission.py" "$FIXTURE/scripts/maat/"

WORKFLOW="$FIXTURE/.github/workflows/release.yml"
GATE="$FIXTURE/scripts/verify-menubar-release-contract.sh"
SOURCE_WORKFLOW="$ROOT/.github/workflows/release.yml"

run_gate() {
    (cd "$FIXTURE" && bash "$GATE")
}

replace_once() {
    python3 - "$WORKFLOW" "$1" "$2" <<'PY'
from pathlib import Path
import sys

path = Path(sys.argv[1])
source = path.read_text()
old, new = (value.replace("\\n", "\n") for value in sys.argv[2:])
if source.count(old) != 1:
    raise SystemExit(f"fixture mutation expected one source match: {old!r}")
path.write_text(source.replace(old, new, 1))
PY
}

expect_refusal() {
    local label="$1"
    if run_gate >"$FIXTURE/result.log" 2>&1; then
        echo "fixture unexpectedly accepted: $label" >&2
        exit 1
    fi
    grep -Fq 'menubar_release_contract accepted=false' "$FIXTURE/result.log" || {
        echo "fixture failed outside the contract gate: $label" >&2
        cat "$FIXTURE/result.log" >&2
        exit 1
    }
}

move_guard_after() {
    python3 - "$WORKFLOW" "$1" <<'PY'
from pathlib import Path
import sys

path = Path(sys.argv[1])
target_name = sys.argv[2]
lines = path.read_text().splitlines(keepends=True)
menubar_start = next(i for i, line in enumerate(lines) if line.rstrip("\n") == "  menubar:")
guard_name = "      - name: Bind native release to qualified M1 runner"
guard_start = next(i for i in range(menubar_start, len(lines)) if lines[i].rstrip("\n") == guard_name)
guard_end = next(i for i in range(guard_start + 1, len(lines)) if lines[i].startswith("      - "))
guard = lines[guard_start:guard_end]
del lines[guard_start:guard_end]
target = "      - name: " + target_name
target_start = next(i for i in range(menubar_start, len(lines)) if lines[i].rstrip("\n") == target)
target_end = next(i for i in range(target_start + 1, len(lines)) if lines[i].startswith("      - "))
lines[target_end:target_end] = guard
path.write_text("".join(lines))
PY
}

run_gate >/dev/null

reset_workflow() { cp "$SOURCE_WORKFLOW" "$WORKFLOW"; }

reset_workflow
replace_once 'runs-on: [self-hosted, macOS, ARM64, m1]' 'runs-on: [self-hosted, macOS, ARM64]'
expect_refusal 'missing M1 runner label'

reset_workflow
replace_once 'runs-on: [self-hosted, macOS, ARM64, m1]' 'runs-on: [self-hosted, macOS, ARM64, m5]'
expect_refusal 'mislabelled runner'

reset_workflow
replace_once '[[ "${PANTHEON_RUNNER_NAME:-}" == "m1-pantheon" ]]' '[[ "${PANTHEON_RUNNER_NAME:-}" == "m5-sirsi" ]]'
expect_refusal 'wrong runner name'

reset_workflow
replace_once '          [[ "${PANTHEON_RUNNER_NAME:-}" == "m1-pantheon" ]] || { echo "requires runner m1-pantheon" >&2; exit 1; }' \
    '          # [[ "${PANTHEON_RUNNER_NAME:-}" == "m1-pantheon" ]] || { echo "requires runner m1-pantheon" >&2; exit 1; }'
expect_refusal 'commented runner check'

reset_workflow
replace_once '          [[ "${PANTHEON_RUNNER_NAME:-}" == "m1-pantheon" ]] || { echo "requires runner m1-pantheon" >&2; exit 1; }' \
    '          # [[ "${PANTHEON_RUNNER_NAME:-}" == "m1-pantheon" ]] || { echo "requires runner m1-pantheon" >&2; exit 1; }'
replace_once '  menubar:\n' '    # decoy runner.name and M1 pin in another job\n  menubar:\n'
expect_refusal 'other-job comment decoy'

reset_workflow
replace_once '      - name: Bind native release to qualified M1 runner' \
    '      - name: Bind native release to qualified M1 runner\n        if: false'
expect_refusal 'step-level if false'

reset_workflow
replace_once '      - name: Bind native release to qualified M1 runner' \
    '      - name: Bind native release to qualified M1 runner\n        continue-on-error: true'
expect_refusal 'step-level continue-on-error'

reset_workflow
replace_once '          PANTHEON_RUNNER_ARCH: ${{ runner.arch }}' \
    '          PANTHEON_RUNNER_ARCH: ${{ runner.arch }}\n          EXTRA_RUNNER_CONTEXT: ${{ runner.name }}'
expect_refusal 'extra guard environment'

reset_workflow
replace_once 'PANTHEON_RUNNER_OS: ${{ runner.os }}' 'PANTHEON_RUNNER_OS: ${{ runner.environment }}'
expect_refusal 'wrong runner OS context'

reset_workflow
replace_once '[[ "${PANTHEON_RUNNER_OS:-}" == "macOS" ]]' '[[ "${PANTHEON_RUNNER_OS:-}" == "Linux" ]]'
expect_refusal 'wrong runner OS value'

reset_workflow
replace_once 'PANTHEON_RUNNER_ARCH: ${{ runner.arch }}' 'PANTHEON_RUNNER_ARCH: ${{ runner.os }}'
expect_refusal 'wrong runner architecture context'

reset_workflow
replace_once '[[ "${PANTHEON_RUNNER_ARCH:-}" == "ARM64" ]]' '[[ "${PANTHEON_RUNNER_ARCH:-}" == "X64" ]]'
expect_refusal 'wrong runner architecture value'

reset_workflow
replace_once '[[ "$(/usr/bin/uname -s)" == "Darwin" ]]' '[[ "$(/usr/bin/uname -s)" == "Linux" ]]'
expect_refusal 'wrong native OS'

reset_workflow
replace_once '[[ "$(/usr/bin/uname -m)" == "arm64" ]]' '[[ "$(/usr/bin/uname -m)" == "x86_64" ]]'
expect_refusal 'wrong native architecture'

reset_workflow
replace_once '[[ "$ACTUAL_HW_MODEL" == "MacBookPro18,1" ]]' '[[ "$ACTUAL_HW_MODEL" == "MacBookPro18,3" ]]'
expect_refusal 'wrong hardware model'

for target in Checkout 'Setup Go' 'Import Developer ID signing identities'; do
    reset_workflow
    move_guard_after "$target"
    expect_refusal "M1 guard after $target"
done

echo 'menubar runner-binding source fixtures PASS'
