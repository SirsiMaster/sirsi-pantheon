#!/usr/bin/env bash
# fleet-check.sh — per-Mac GREEN/RED line: relay, router ping, outbox/parked,
# Stack Lab key. Owner 2026-10-22: show per-Mac router + Stack Lab health on
# the board (ra-fleet-check-into-repo-20261022).
#
# Usage: fleet-check.sh [agent-id]
#   agent-id defaults to $SIRSI_AGENT_ID. Needed only for the router-ping check.
#
# `--self-test` plants a RED condition (missing Stack Lab key file under a
# fake $HOME) and expects this script to go red — the negative control.
set -euo pipefail

SCRIPT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/$(basename "${BASH_SOURCE[0]}")"

self_test() {
  TMP="$(mktemp -d)"
  trap 'rm -rf "$TMP"' EXIT
  mkdir -p "$TMP/.sirsi"
  if HOME="$TMP" bash "$SCRIPT" >"$TMP/self-test.out" 2>&1; then
    echo "  ❌ self-test FAILED: missing relay/stacklab key was not caught"
    cat "$TMP/self-test.out"
    exit 1
  fi
  echo "  ✅ self-test passed: missing relay/stacklab key correctly reported RED"
  exit 0
}
[ "${1:-}" = "--self-test" ] && self_test

status=GREEN
fail() { status=RED; echo "  ✗ $1"; }
ok()   { echo "  ✓ $1"; }

host="$(hostname -s 2>/dev/null || hostname)"
agent="${1:-${SIRSI_AGENT_ID:-}}"

# 1. Relay: LaunchAgent loaded, or the spool directory it serves is present.
if launchctl list ai.sirsi.router.relay >/dev/null 2>&1; then
  ok "relay: ai.sirsi.router.relay loaded"
elif [ -d "$HOME/.sirsi/relay" ]; then
  ok "relay: spool present ($HOME/.sirsi/relay)"
else
  fail "relay: not loaded and no spool directory at $HOME/.sirsi/relay"
fi

# 2. Router ping: this host's own lane reachable through the router CLI.
if [ -z "$agent" ]; then
  fail "router ping: no agent id (pass one as \$1 or set SIRSI_AGENT_ID)"
elif ! command -v sirsi >/dev/null 2>&1; then
  fail "router ping: sirsi CLI not on PATH"
elif sirsi router ping "$agent" >/dev/null 2>&1; then
  ok "router ping: $agent reachable"
else
  fail "router ping: $agent did not respond"
fi

# 3. Outbox / parked (ADR-069 durable outbox): held items retry on their own;
# a parked (failed/) item needs a human to look at it.
spool="$HOME/.sirsi/relay"
held=0
parked=0
if [ -d "$spool" ]; then
  held=$(find "$spool" -mindepth 3 -maxdepth 3 -path '*/outbox/*.json' 2>/dev/null | wc -l | tr -d ' ')
  parked=$(find "$spool" -mindepth 3 -maxdepth 3 -path '*/failed/*.json' 2>/dev/null | wc -l | tr -d ' ')
fi
if [ "$parked" -gt 0 ]; then
  fail "outbox: $held held, $parked parked — needs audit"
else
  ok "outbox: $held held, 0 parked"
fi

# 4. Stack Lab key: presence only. Never reads or prints the file's contents.
if [ -f "$HOME/.sirsi/stacklab/account.json" ]; then
  ok "stacklab key: present"
else
  fail "stacklab key: missing ($HOME/.sirsi/stacklab/account.json)"
fi

echo "$host: $status"
[ "$status" = "GREEN" ]
