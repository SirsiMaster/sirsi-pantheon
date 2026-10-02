#!/usr/bin/env bash
# Both directions (A34/A35): a current router rejection of a PR blocks the bind; a
# later accept, a different PR, or no verdict does not. Pure jq, no network.
set -uo pipefail
JQ="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/router-rejection.jq"
fails=0
check() { # <desc> <pr> <want-id-or-empty> <jsonl>
  local got; got="$(printf '%s\n' "$4" | jq -sr --arg pr "$2" -f "$JQ")"
  if [ "$got" = "$3" ]; then echo "ok   — $1"; else echo "FAIL — $1 (want '$3', got '$got')"; fails=$((fails+1)); fi
}
it() { printf '{"id":"%s","type":"review","title":"%s","opened":"%s"}' "$1" "$2" "$3"; }
check "rejection of the PR blocks" 927 r1 "$(it r1 'PR927 CHANGES REQUIRED: router rejection' 2026-10-01T03:00:00Z)"
check "later accept clears it" 927 "" "$(it r1 'PR927 CHANGES REQUIRED' 2026-10-01T03:00:00Z)
$(it a1 'PR #927 exact head ACCEPT' 2026-10-01T05:00:00Z)"
check "accept then a later rejection blocks" 927 r2 "$(it a1 'PR927 PASS' 2026-10-01T03:00:00Z)
$(it r2 'PR927 changes requested' 2026-10-01T05:00:00Z)"
check "another PR's rejection does not block" 927 "" "$(it r1 'PR92 CHANGES REQUIRED' 2026-10-01T03:00:00Z)
$(it r2 'PR9270 CHANGES REQUIRED' 2026-10-01T03:00:00Z)"
check "no verdict in the title does not block" 927 "" "$(it r1 'PR927 question about scope' 2026-10-01T03:00:00Z)"
exit "$fails"
