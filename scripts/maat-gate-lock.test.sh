#!/usr/bin/env bash
# The Ma'at gate queues behind another gate instead of colliding with it. Three behaviors:
#   1. a second acquire WAITS while the first holds the lock, then proceeds once it is released
#   2. a lock whose holder pid is dead is reclaimed (a killed gate must not wedge every push)
#   3. a wait past the limit FAILS loudly instead of proceeding unlocked
set -uo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT
export MAAT_GATE_LOCK_DIR="$T/maat-gate.lock"
LIB="$ROOT/.githooks/gate-lock.sh"
fail() { echo "FAIL: $*"; exit 1; }

# 1. waits, then proceeds
( . "$LIB"; gate_lock_acquire; sleep 4; gate_lock_release ) &
H=$!
sleep 1
START=$(date +%s)
( . "$LIB"; gate_lock_acquire >/dev/null 2>&1 || exit 7; gate_lock_release )
RC=$?; ELAPSED=$(( $(date +%s) - START ))
wait $H
[ $RC -eq 0 ] || fail "second gate did not proceed after the first released (rc=$RC)"
[ $ELAPSED -ge 2 ] || fail "second gate did not wait for the first (elapsed ${ELAPSED}s)"
[ ! -e "$MAAT_GATE_LOCK_DIR" ] || fail "lock left behind after both released"

# 2. dead holder is reclaimed
mkdir -p "$MAAT_GATE_LOCK_DIR"; echo 2147483000 > "$MAAT_GATE_LOCK_DIR/pid"
( . "$LIB"; gate_lock_acquire >/dev/null 2>&1 || exit 7; [ "$(cat "$MAAT_GATE_LOCK_DIR/pid")" = "$$" ] || exit 8; gate_lock_release ) || fail "a dead holder's lock was not reclaimed"

# 3. timeout fails loudly (negative control: a live holder that never releases)
( . "$LIB"; gate_lock_acquire; sleep 8; gate_lock_release ) &
H=$!
sleep 1
( MAAT_GATE_LOCK_WAIT_SECS=2 . "$LIB"; MAAT_GATE_LOCK_WAIT_SECS=2 gate_lock_acquire >/dev/null 2>&1 ) && fail "a timed-out wait must fail, not proceed unlocked"
kill $H 2>/dev/null; wait $H 2>/dev/null
rm -rf "$MAAT_GATE_LOCK_DIR"

# MAAT_GATE_LOCK=off bypasses
( MAAT_GATE_LOCK=off . "$LIB"; MAAT_GATE_LOCK=off gate_lock_acquire ) || fail "off must bypass"
echo "ok: the Ma'at gate queues behind another gate"
