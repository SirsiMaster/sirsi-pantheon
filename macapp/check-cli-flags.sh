#!/usr/bin/env bash
# check-cli-flags.sh — behavioral guard: SirsiMenubar must answer a command-line
# question on the command line, and must never answer it by opening a window.
#
# WHY THIS EXISTS: argument handling was `if let i = argv.firstIndex(of:
# "--snapshot") … else { app.run() }`. Every argument the snapshot parser did not
# consume fell through to the LAUNCH path, so `SirsiMenubar --help` put a second
# Command Deck panel on the owner's screen. `--help` was the reported symptom;
# `--snapshot` with no directory and `--width` without `--snapshot` had the same
# fate. This guard asserts the fall-through is closed, not that one flag works.
#
# WHY IT RUNS THE BINARY INSTEAD OF GREPPING THE SOURCE: the invariant is "the
# process EXITS". app.run() blocks forever, so a launch and a correct exit are
# trivially distinguishable by whether the process terminates on its own — but
# only if you actually run it. A structural check could confirm an `exit(0)` is
# written somewhere and still miss that it is unreachable.
#
# Both directions are exercised: cases that must exit 0, cases that must exit 2,
# and (the regression that started this) the assertion that NEITHER hangs.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
BIN="$HERE/.build/release/SirsiMenubar"

if [ ! -x "$BIN" ]; then
  echo "check-cli-flags: building release binary…"
  (cd "$HERE" && swift build -c release >/dev/null)
fi
[ -x "$BIN" ] || { echo "check-cli-flags: no binary at $BIN" >&2; exit 2; }

# THE RENAME IS LOAD-BEARING — do not "simplify" it away by running $BIN directly.
# The failure this guard detects is "the process launched instead of exiting", and
# a launched instance runs retireOlderInstances(), which terminates peer copies of
# this surface — including the operator's live menubar, matched by executable NAME.
# Run under the real name and a REGRESSION IN THE CODE UNDER TEST would kill the
# owner's running panel as a side effect of testing for it. Copied to a name no
# peer matcher recognises, a regressed case can only hang and be timed out.
PROBE="$(mktemp -d)/sirsi-cli-probe"
trap 'rm -rf "$(dirname "$PROBE")"' EXIT
cp "$BIN" "$PROBE"

fails=0

# run_case <expected-exit> <must-contain> <args…>
run_case() {
  local want="$1" needle="$2"; shift 2
  local out rc=0
  # A 5s cap is the whole point: a regressed build LAUNCHES and never returns.
  # Timing out is a FAILURE, and is reported as one rather than as a flake.
  out="$(timeout 5 "$PROBE" "$@" 2>&1)" || rc=$?
  if [ "$rc" = 124 ]; then
    echo "  FAIL  [$*] did not exit within 5s — it launched the UI"
    fails=$((fails + 1)); return
  fi
  if [ "$rc" != "$want" ]; then
    echo "  FAIL  [$*] exit=$rc want=$want"
    fails=$((fails + 1)); return
  fi
  if ! grep -q "$needle" <<<"$out"; then
    echo "  FAIL  [$*] output missing '$needle'"
    fails=$((fails + 1)); return
  fi
  echo "  ok    [$*] exit=$rc"
}

echo "check-cli-flags: usage paths must exit, never launch"
run_case 0 "Usage: SirsiMenubar" --help
run_case 0 "Usage: SirsiMenubar" -h

echo "check-cli-flags: malformed invocations must fail closed"
run_case 2 "unknown flag"                  --nonsense
run_case 2 "requires a directory"          --snapshot
run_case 2 "requires --snapshot"           --width 500
run_case 2 "requires --snapshot"           --appearance light
run_case 2 "requires a number"             --snapshot /tmp --width wide
run_case 2 "requires light or dark"        --snapshot /tmp --appearance purple

if [ "$fails" -gt 0 ]; then
  echo "check-cli-flags: $fails case(s) FAILED" >&2
  exit 1
fi
echo "check-cli-flags: all cases passed"
