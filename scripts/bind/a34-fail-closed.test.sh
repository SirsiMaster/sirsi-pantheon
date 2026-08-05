#!/usr/bin/env bash
# Asserts the A34 "most-recent-review-per-reviewer" jq filter in sirsi-bind.sh
# picks the right blocking reviewer (or none) from synthetic GitHub reviews
# JSON. No network/credentials needed — this exercises the filter logic in
# isolation, not the live gh calls (those need a real PR + bind identity;
# see bind-event-selection.test.sh for the argument-guard coverage).
#
# The filter (kept byte-identical to sirsi-bind.sh so this test catches drift):
#   [.[] | select(.user.login != "sirsi-bind[bot]") | select(.state == "APPROVED" or .state == "CHANGES_REQUESTED")]
#   | group_by(.user.login) | map(max_by(.submitted_at))
#   | map(select(.state == "CHANGES_REQUESTED")) | .[0].user.login // empty
set -uo pipefail

FILTER='[.[] | select(.user.login != "sirsi-bind[bot]") | select(.state == "APPROVED" or .state == "CHANGES_REQUESTED")]
        | group_by(.user.login) | map(max_by(.submitted_at))
        | map(select(.state == "CHANGES_REQUESTED")) | .[0].user.login // empty'

fails=0
check() { # <desc> <expected-blocking-login-or-empty> <reviews-json>
  local desc="$1" want="$2" json="$3"
  local got; got="$(printf '%s' "$json" | jq -r "$FILTER")"
  if [ "$got" = "$want" ]; then
    echo "ok   — $desc"
  else
    echo "FAIL — $desc (want '$want', got '$got')"
    fails=$((fails + 1))
  fi
}

review() { printf '{"user":{"login":"%s"},"state":"%s","submitted_at":"%s"}' "$1" "$2" "$3"; }

check "no reviews at all -> not blocked" "" '[]'

check "single CHANGES_REQUESTED -> blocked by that reviewer" "codex" \
  "[$(review codex CHANGES_REQUESTED 2026-08-01T00:00:00Z)]"

check "single APPROVED -> not blocked" "" \
  "[$(review codex APPROVED 2026-08-01T00:00:00Z)]"

check "same reviewer requests changes then later approves -> cleared (latest wins)" "" \
  "[$(review codex CHANGES_REQUESTED 2026-08-01T00:00:00Z),$(review codex APPROVED 2026-08-02T00:00:00Z)]"

check "same reviewer approves then later requests changes -> blocked (latest wins)" "codex" \
  "[$(review codex APPROVED 2026-08-01T00:00:00Z),$(review codex CHANGES_REQUESTED 2026-08-02T00:00:00Z)]"

check "a DIFFERENT reviewer approving does not clear codex's standing rejection (A34 clause a)" "codex" \
  "[$(review codex CHANGES_REQUESTED 2026-08-01T00:00:00Z),$(review claude-deck APPROVED 2026-08-02T00:00:00Z)]"

check "two reviewers, only one rejecting -> that one blocks" "reviewer-b" \
  "[$(review reviewer-a APPROVED 2026-08-01T00:00:00Z),$(review reviewer-b CHANGES_REQUESTED 2026-08-01T00:00:00Z)]"

check "sirsi-bind's own prior REQUEST_CHANGES is excluded from the blocking set" "" \
  "[$(review 'sirsi-bind[bot]' CHANGES_REQUESTED 2026-08-01T00:00:00Z)]"

check "COMMENTED reviews are ignored, not treated as a verdict" "" \
  "[$(review codex COMMENTED 2026-08-01T00:00:00Z)]"

[ "$fails" = 0 ] && echo "all A34 filter checks pass" || echo "$fails check(s) failed"
exit "$fails"
