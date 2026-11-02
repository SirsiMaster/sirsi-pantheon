#!/bin/bash
# Source/static contract for Ma'at's first fully attested native repair. This
# checks the source wiring only; it deliberately does not inspect launchd,
# execute a repair, or read an operator's decision journal.
set -euo pipefail

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
REPAIR="$ROOT/cmd/sirsi/maatrepair.go"
RECEIPT="$ROOT/internal/maat/diagnostic_repair.go"
VIEW="$ROOT/macapp/Sources/SirsiMenubar/Views.swift"
CASEBOOK="$ROOT/macapp/Sources/SirsiMenubar/MaatCasebookView.swift"
LAUNCHD_REVIEW="$ROOT/macapp/Sources/SirsiMenubar/MaatLaunchdReview.swift"
CORE_TEST="$ROOT/macapp/Tests/SirsiMenubarTests/CoreContractsTests.swift"

for file in "$REPAIR" "$RECEIPT" "$VIEW" "$CASEBOOK" "$LAUNCHD_REVIEW" "$CORE_TEST"; do
  test -f "$file" || { echo "missing Ma'at guided-repair source: $file" >&2; exit 1; }
done

grep -Fq 'Use:   "launchd-disabled"' "$REPAIR"
grep -Fq 'maatRepairReadDiagnosis' "$REPAIR"
grep -Fq 'maatRepairRestoreReviewedLaunchAgents' "$REPAIR"
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
grep -Fq 'MaatLaunchdPreviewReviewControls(' "$VIEW"
grep -Fq 'MaatLaunchdPreviewReviewControls(' "$CASEBOOK"
grep -Fq 'maatLaunchdRepairArguments(previewBinding: preview.binding)' "$VIEW"
grep -Fq 'MaatCasebookLaunchdConsent(preview: launchdPreview)' "$CASEBOOK"
grep -Fq 'let launchdConsent = confirmedLaunchdConsent' "$CASEBOOK"
grep -Fq 'confirmedLaunchdConsent: launchdConsent' "$CASEBOOK"
grep -Fq 'consent.apply(afterHistoryRead: { true }' "$CASEBOOK"
grep -Fq 'canRefresh: !operationInFlight' "$CASEBOOK"
grep -Fq 'confirmedLaunchdConsent?.preview' "$CASEBOOK"
grep -Fq 'Label(receipt.summary, systemImage: receipt.succeeded ? "checkmark.seal.fill" : "exclamationmark.triangle.fill")' "$CASEBOOK"
grep -Fq 'let frozenArguments = repairArguments' "$LAUNCHD_REVIEW"
grep -Fq 'guard preview.isValid, await afterHistoryRead()' "$LAUNCHD_REVIEW"
grep -Fq 'MaatLaunchdPreviewTargets(preview: preview)' "$LAUNCHD_REVIEW"
grep -Fq 'extension MaatCasebookLaunchdApply: Sendable where Value: Sendable' "$LAUNCHD_REVIEW"
grep -Fq 'func testCasebookConfirmedLaunchdPreviewSurvivesHistoryWaitAndStaleBindingDoesNotActuate()' "$CORE_TEST"
grep -Fq 'func testLaunchdStaleApprovalGuidanceIsRenderedAndConsentUsesFrozenPreview()' "$CORE_TEST"
grep -Fq 'if isLaunchdDisabledRepair, let repairReviewMessage' "$VIEW"
grep -Fq '"--preview-binding", previewBinding, "--confirm", "--json"' "$LAUNCHD_REVIEW"
for file in "$VIEW" "$CASEBOOK" "$LAUNCHD_REVIEW"; do
  if grep -Fq '"maat", "repair", "launchd-disabled", "--confirm"' "$file"; then
    echo "native launchd repair still contains unbound confirmation arguments: $file" >&2
    exit 1
  fi
done
grep -Fq 'DiagnosticRepairResultSheet(item: $presentedRepair)' "$VIEW"
grep -Fq 'args: frozen.args, reverifyCheck: frozen.snapshot.check' "$VIEW"
grep -Fq 'Unverified — the fresh diagnostic read failed or was incomplete. The prior finding is not resolved.' "$VIEW"
grep -Fq 'Fresh check: “\(check)” is clear now. This observation does not prove which action caused the change.' "$VIEW"
grep -Fq 'Fresh check: “\(check)” remains present.' "$VIEW"
grep -Fq 'Failed or incomplete checks remain labeled unverified; the interface does not call them fixed.' "$VIEW"

if grep -Fq 'This needs attention but has no one-click fix yet.' "$VIEW"; then
  echo "guided-repair UI still advertises a dead-end repair state" >&2
  exit 1
fi

echo "Ma'at guided repair contract: pass"
