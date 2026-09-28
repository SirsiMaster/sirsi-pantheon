#!/bin/bash
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd -P)
contract="$root/contracts/stacklab/apollo-sne-telemetry-v1.json"
consumer="$root/internal/apollo/telemetry.go"
dashboard="$root/internal/dashboard/apollo.go"
dashboard_ui="$root/internal/dashboard/pages.go"

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
grep -Fq '/api/apollo/telemetry' "$root/internal/dashboard/server.go"
grep -Fq 'ApolloTelemetryFn' "$dashboard"
grep -Fq 'writeJSON(w, read)' "$dashboard"
grep -Fq 'function viewApollo()' "$dashboard_ui"
grep -Fq 'refresh telemetry' "$dashboard_ui"
grep -Fq 'No SNE session sample is available' "$dashboard_ui"
echo "Apollo SNE telemetry contract: PASS"
