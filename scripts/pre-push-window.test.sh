#!/usr/bin/env bash
# Both directions (A35): the pre-push gate refuses while rails.lock exists and
# passes (tag-only fast pass) once it is gone; the override lets it through.
set -uo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOCK="$(mktemp -u)"; fails=0
run() { MAAT_RAILS_LOCK="$LOCK" "$@" bash "$ROOT/.githooks/pre-push" origin url </dev/null >/dev/null 2>&1; }
echo hermes >"$LOCK"
run env; [ $? -eq 1 ] && echo "ok   — window open: push refused" || { echo "FAIL — push allowed during window"; fails=$((fails+1)); }
run env MAAT_WINDOW_OVERRIDE=1; [ $? -eq 0 ] && echo "ok   — override passes" || { echo "FAIL — override refused"; fails=$((fails+1)); }
rm -f "$LOCK"
run env; [ $? -eq 0 ] && echo "ok   — window clear: push passes" || { echo "FAIL — push refused with no window"; fails=$((fails+1)); }
exit "$fails"
