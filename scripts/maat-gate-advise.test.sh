#!/usr/bin/env bash
# The Ma'at gate consults the known-failure catalog on every failing step. Pinned here:
#   1. a passing step is silent
#   2. a failing step with a KNOWN signature prints the cause and the fix, and still fails
#   3. a failing step with an unknown signature says how to record it, and still fails
#   4. MAAT_ADVISE=off prints no advice, and the failure still propagates
#   5. an advisor that cannot run (no Go toolchain) says so and never changes the verdict
set -uo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT" || exit 1
fail() { echo "FAIL: $*"; exit 1; }
export GATE_OUT; GATE_OUT="$(mktemp -d)"; trap 'rm -rf "$GATE_OUT"' EXIT
# shellcheck source=../.githooks/gate-advise.sh
. "$ROOT/.githooks/gate-advise.sh"

out="$(gate_run quiet true 2>&1)"; rc=$?
[ $rc -eq 0 ] && [ -z "$out" ] || fail "a passing step must be silent and succeed (rc=$rc out=$out)"

out="$(gate_run tests bash -c 'echo "--- FAIL: TestSenderFloodRejected (1.99s)"; echo "    dispatch_contract_test.go:248: flood appended 21 items - the 11,564 flood lives"; exit 1' 2>&1)"; rc=$?
[ $rc -ne 0 ] || fail "a failing step must still fail"
echo "$out" | grep -q "KNOWN failure in tests: quota-test-hour-boundary" || fail "known failure not recognized: $out"
echo "$out" | grep -q "cause:" && echo "$out" | grep -q "fix:" || fail "known failure lacks cause or fix: $out"
echo "$out" | grep -q "flood appended 21 items" || fail "the failure lines must still be shown: $out"

out="$(gate_run build bash -c 'echo "./x.go:3:1: undefined: Frobnicate"; exit 1' 2>&1)"; rc=$?
[ $rc -ne 0 ] || fail "an unknown failure must still fail"
echo "$out" | grep -q "no known failure matches" && echo "$out" | grep -q "known-failures register" || fail "unknown failure must say how to record it: $out"
echo "$out" | grep -q "KNOWN failure" && fail "invented a match for unknown text: $out"

out="$(MAAT_ADVISE=off gate_run tests bash -c 'echo "flood appended 21 items"; exit 1' 2>&1)"; rc=$?
[ $rc -ne 0 ] || fail "off must not change the verdict"
echo "$out" | grep -q "Ma'at" && fail "MAAT_ADVISE=off must print no advice: $out"

broken="$(mktemp -d)"; printf '#!/bin/sh\nexit 1\n' > "$broken/go"; chmod +x "$broken/go"
out="$(PATH="$broken:$PATH" gate_run tests bash -c 'echo "flood appended 21 items"; exit 1' 2>&1)"; rc=$?
[ $rc -ne 0 ] || fail "an unavailable advisor must not change the verdict"
echo "$out" | grep -q "advisor could not run" || fail "an unavailable advisor must say so: $out"

echo "ok: the Ma'at gate consults the known-failure catalog on every failing step"
