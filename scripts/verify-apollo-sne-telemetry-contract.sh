#!/bin/bash
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd -P)
contract="$root/contracts/stacklab/apollo-sne-telemetry-v1.json"
consumer="$root/internal/apollo/telemetry.go"
dashboard="$root/internal/dashboard/apollo.go"
dashboard_ui="$root/internal/dashboard/pages.go"
native_ui="$root/macapp/Sources/SirsiMenubar/ApolloRunPlannerView.swift"

jq -e '
  .schema == "sirsi.stacklab.apollo-sne-telemetry.v1" and
  .ownership.publisher == "SNE" and
  .ownership.consumer == "Pantheon Apollo" and
  .capability_catalog.schema_version == "apollo-catalog/v2" and
  (.capability_catalog.machine | index("chip_estates")) and
  (.capability_catalog.engine | index("machine_id")) and
  (.capability_catalog.selection_rules | any(contains("typed capacity receipt"))) and
  .session_telemetry.schema_version == "apollo-session-telemetry/v1" and
  (.session_telemetry.optional_metrics | index("tokens_per_second")) and
  (.session_telemetry.optional_metrics | index("gpu_residency_percent")) and
  (.consumer_states.awaiting_session | length > 0)
' "$contract" >/dev/null

grep -Fq 'apollo-session-telemetry/v1' "$consumer"
grep -Fq 'awaiting_session' "$consumer"
grep -Fq 'DisallowUnknownFields' "$consumer"
grep -Fq 'network_saturation_percent' "$consumer"
grep -Fq 'revalidateTelemetryPath' "$consumer"
grep -Fq 'O_NOFOLLOW' "$root/internal/apollo/telemetry_nofollow_unix.go"
grep -Fq '/api/apollo/telemetry' "$root/internal/dashboard/server.go"
grep -Fq 'ApolloTelemetryFn' "$dashboard"
grep -Fq 'writeJSON(w, read)' "$dashboard"
grep -Fq 'function viewApollo()' "$dashboard_ui"
grep -Fq 'refresh telemetry' "$dashboard_ui"
grep -Fq 'No SNE session sample is available' "$dashboard_ui"
grep -Fq 'apollo_session_telemetry' "$root/internal/mcp/tools.go"
grep -Fq 'readApolloSessionTelemetry' "$root/internal/mcp/tools.go"
grep -Fq 'telemetryRefreshIntervalNanoseconds' "$native_ui"
grep -Fq 'Live refresh every 5 seconds while this page is open.' "$native_ui"
echo "Apollo SNE telemetry contract: PASS"
