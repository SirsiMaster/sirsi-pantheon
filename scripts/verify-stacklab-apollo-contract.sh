#!/bin/bash
# Static guard for the Stack Lab -> Apollo planning boundary. The source may
# validate a plan, but it must not silently start a model or turn unknown
# throughput/residency into an invented zero-value dashboard.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd -P)
catalog="$root/internal/apollo/catalog.go"
command="$root/cmd/sirsi/apollo.go"
surface="$root/macapp/Sources/SirsiMenubar/ApolloRunPlannerView.swift"
stacklab="$root/macapp/Sources/SirsiMenubar/StackLabView.swift"

for path in "$catalog" "$command" "$surface" "$stacklab"; do
  test -f "$path" || { echo "missing Apollo contract input: $path" >&2; exit 1; }
done

grep -Fq 'SNE admission is required before inference starts' "$catalog"
grep -Fq 'Validate a local Apollo run plan without starting inference' "$command"
grep -Fq 'ApolloRunPlannerView' "$surface"
grep -Fq 'ApolloTelemetryView' "$surface"
grep -Fq 'Tokens / second' "$surface"
grep -Fq 'Awaiting session' "$surface"
grep -Fq 'Plan an Apollo run' "$stacklab"

if grep -Eq 'exec\.Command|Process\(' "$catalog" "$command" "$surface"; then
  echo "Apollo planning surface must not introduce execution authority" >&2
  exit 1
fi

echo "Stack Lab Apollo planning contract: PASS"
