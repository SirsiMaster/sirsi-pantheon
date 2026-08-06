#!/bin/bash
# should_defer_test.sh — the worker's deferral truth table.
#
# should_defer gates whether an agentic build session gets spent, so it gets a
# check. The four cases below are the whole contract; the third one is the
# regression this exists for.
#
# Run: bash scripts/worker/should_defer_test.sh
set -u

SIRSI_WORKER_LIB_ONLY=1 . "$(dirname "$0")/sirsi-claude-worker.sh"

fail=0
check() { # check <name> <expected: defer|take>
  local name=$1 want=$2 got
  if should_defer "$AGE"; then got=defer; else got=take; fi
  if [ "$got" != "$want" ]; then
    echo "FAIL: $name — want $want, got $got"
    fail=1
  else
    echo "ok:   $name ($got)"
  fi
}

MIN_AGE=1800

# A live attended session owns fresh items; the backstop stays out of the way.
attended_session_live(){ return 0; }
AGE=60;   check "fresh item, attended session live" defer

# Past the window the backstop takes over regardless — an attended session that
# has held an item for 30 minutes is not making progress on it.
AGE=3600; check "old item, attended session live"   take

# THE REGRESSION. With nobody attending, a fresh item must be taken NOW, not
# left to age out. This was an unconditional 30-minute idle window: a healthy,
# polling worker declined the item every pass while every surface read green.
attended_session_live(){ return 1; }
AGE=60;   check "fresh item, NO attended session"   take

AGE=3600; check "old item, NO attended session"     take

# Guard the short-circuit order: the CLI must not be consulted for items already
# past MIN_AGE, or every poll shells out once per stale item forever.
probed=0
attended_session_live(){ probed=1; return 0; }
AGE=3600
should_defer "$AGE" >/dev/null 2>&1 || true
if [ "$probed" -ne 0 ]; then
  echo "FAIL: old item probed attended-liveness — short-circuit order is wrong"
  fail=1
else
  echo "ok:   old item does not probe attended-liveness"
fi

[ "$fail" -eq 0 ] && echo "PASS" || echo "FAILED"
exit "$fail"
