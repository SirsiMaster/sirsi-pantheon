#!/bin/bash
# Source/static contract for Ma'at's first fully attested native repair. This
# checks the source wiring only; it deliberately does not inspect launchd,
# execute a repair, or read an operator's decision journal.
set -euo pipefail

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
REPAIR="$ROOT/cmd/sirsi/maatrepair.go"
RECEIPT="$ROOT/internal/maat/diagnostic_repair.go"
VIEW="$ROOT/macapp/Sources/SirsiMenubar/Views.swift"

for file in "$REPAIR" "$RECEIPT" "$VIEW"; do
  test -f "$file" || { echo "missing Ma'at guided-repair source: $file" >&2; exit 1; }
done

grep -Fq 'Use:   "launchd-disabled"' "$REPAIR"
grep -Fq 'maatRepairReadDiagnosis' "$REPAIR"
grep -Fq 'maatRepairRestoreLaunchAgents' "$REPAIR"
grep -Fq 'no actionable managed disabled-override finding exists; no launchd state changed' "$REPAIR"
grep -Fq 'post-repair diagnostic still reports an actionable managed disabled override' "$REPAIR"
# A repair starts as failed and may become resolved only after the same
# diagnostic has been re-observed cleanly. Do not regress this into a brittle
# initializer-literal check: the post-observation assignment is the actual
# safety contract.
grep -Fq 'outcome.Determination = "resolved"' "$REPAIR"
grep -Fq 'RecordDiagnosticRepair' "$REPAIR"
grep -Fq 'diagnostic-repair:sha256=' "$RECEIPT"
grep -Fq 'determination must be resolved or failed' "$RECEIPT"
grep -Fq '["maat", "repair", "launchd-disabled", "--confirm"]' "$VIEW"
grep -Fq 'retain either a verified recovery receipt or an explicit incomplete outcome' "$VIEW"

if grep -Fq 'This needs attention but has no one-click fix yet.' "$VIEW"; then
  echo "guided-repair UI still advertises a dead-end repair state" >&2
  exit 1
fi

echo "Ma'at guided repair contract: pass"
