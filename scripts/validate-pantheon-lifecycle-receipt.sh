#!/usr/bin/env bash
# Validate a host-produced lifecycle receipt; never mount, install, or remove
# an app. Real M1/M5 execution remains a separately admitted operation.
set -euo pipefail

[[ $# -eq 4 && "$1" == "--plan" && "$3" == "--receipt" ]] || {
  echo "usage: $0 --plan PLAN.json --receipt RECEIPT.json" >&2; exit 2;
}
PLAN="$2"; RECEIPT="$4"
[[ -f "$PLAN" && -f "$RECEIPT" ]] || { echo "pantheon_lifecycle accepted=false reason=missing_input" >&2; exit 1; }
command -v jq >/dev/null || { echo "pantheon_lifecycle accepted=false reason=missing_jq" >&2; exit 1; }

jq -e --slurpfile plan "$PLAN" '
  $plan[0].schema == "sirsi.pantheon.lifecycle-plan.v1" and
  .schema == "sirsi.pantheon.lifecycle-receipt.v1" and
  .decision == "accepted" and
  .source.commit == $plan[0].source.commit and
  .source.tree == $plan[0].source.tree and
  .package.sha256 == $plan[0].package.sha256 and
  .host.profile == $plan[0].host.profile and
  (.host.profile == "m1" or .host.profile == "m5") and
  (.phases.install == "passed") and (.phases.upgrade == "passed") and
  (.phases.rollback == "passed") and (.phases.uninstall == "passed") and
  (.resource.process_leak_free == true) and
  ((.resource.peak_rss_bytes | tonumber) >= 0) and
  ((.resource.swap_growth_bytes | tonumber) >= 0)
' "$RECEIPT" >/dev/null || { echo "pantheon_lifecycle accepted=false reason=receipt_contract_mismatch" >&2; exit 1; }

echo "pantheon_lifecycle accepted=true profile=$(jq -r '.host.profile' "$RECEIPT")"
