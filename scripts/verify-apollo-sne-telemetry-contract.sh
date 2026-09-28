#!/bin/bash
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd -P)
contract="$root/contracts/stacklab/apollo-sne-telemetry-v1.json"
consumer="$root/internal/apollo/telemetry.go"

jq -e '
  .schema == "sirsi.stacklab.apollo-sne-telemetry.v1" and
  .ownership.publisher == "SNE" and
  .ownership.consumer == "Pantheon Apollo" and
  .session_telemetry.schema_version == "apollo-session-telemetry/v1" and
  (.session_telemetry.optional_metrics | index("tokens_per_second")) and
  (.session_telemetry.optional_metrics | index("gpu_residency_percent")) and
  (.consumer_states.awaiting_session | length > 0)
' "$contract" >/dev/null

grep -Fq 'apollo-session-telemetry/v1' "$consumer"
grep -Fq 'awaiting_session' "$consumer"
grep -Fq 'DisallowUnknownFields' "$consumer"
grep -Fq 'network_saturation_percent' "$consumer"
echo "Apollo SNE telemetry contract: PASS"
