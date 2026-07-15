#!/usr/bin/env bash
# Pins the bind-selection logic in .github/workflows/binding-hold.yml (ADR-041).
#
# The filter is EXTRACTED from the workflow rather than copied here — a copy would
# drift and this test would pass while the real gate rotted. If the workflow's jq
# program changes shape, extraction fails loudly rather than testing a stale string.
#
# Run: scripts/bind/binding-hold-selection.test.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
WF="$ROOT/.github/workflows/binding-hold.yml"
[ -r "$WF" ] || { echo "✗ missing $WF"; exit 1; }

# The jq program lives between `--jq '` and the line that terminates it with `')`.
# That terminator sits on the last line OF the program, so strip it and keep the line.
FILTER=$(awk '
  /--jq '"'"'$/ { f=1; next }
  f && /'"'"'\)[[:space:]]*$/ { sub(/'"'"'\)[[:space:]]*$/, ""); print; exit }
  f { print }
' "$WF")
[ -n "$FILTER" ] || { echo "✗ could not extract the jq filter from $WF — did its shape change?"; exit 1; }

HEAD="aaa111"
run() { HEAD_SHA="$HEAD" AUTHOR="SirsiMaster" jq -r "$FILTER"; }

fail=0
check() { # name expected actual
  if [ "$2" = "$3" ]; then
    echo "  ✔ $1"
  else
    echo "  ✗ $1 — expected '$2', got '$3'"; fail=1
  fi
}

echo "binding-hold review selection:"

# The whole point: one shared identity must not be able to bind its own PR.
got=$(echo '[{"state":"APPROVED","commit_id":"aaa111","user":{"login":"SirsiMaster"}}]' | run)
check "author's own approval does NOT bind" "" "$got"

# Stale bind: approved, independent, but on a commit that is no longer head.
got=$(echo '[{"state":"APPROVED","commit_id":"old999","user":{"login":"sirsi-bind[bot]"}}]' | run)
check "approval on a stale SHA does NOT bind" "" "$got"

# A comment-only or requested-changes review is not a bind.
got=$(echo '[{"state":"COMMENTED","commit_id":"aaa111","user":{"login":"sirsi-bind[bot]"}}]' | run)
check "non-APPROVED review does NOT bind" "" "$got"

# The happy path.
got=$(echo '[{"state":"APPROVED","commit_id":"aaa111","user":{"login":"sirsi-bind[bot]"}}]' | run)
check "independent approval on head SHA binds" "sirsi-bind[bot]" "$got"

# Author noise must not mask a real bind.
got=$(echo '[{"state":"APPROVED","commit_id":"aaa111","user":{"login":"SirsiMaster"}},
             {"state":"APPROVED","commit_id":"aaa111","user":{"login":"sirsi-bind[bot]"}}]' | run)
check "author approval alongside a real bind still binds" "sirsi-bind[bot]" "$got"

# No reviews at all.
check "no reviews does NOT bind" "" "$(echo '[]' | run)"

[ "$fail" -eq 0 ] && echo "✔ all bind-selection cases pass" || { echo "✗ bind-selection FAILED"; exit 1; }
