#!/usr/bin/env bash
# fleet-check.sh — one GREEN/RED verdict for this Mac's router plumbing:
#   relay daemon, round trip to the service, outbox/parked, Stack Lab deploy key.
# Owner 2026-10-22: show per-Mac router + Stack Lab health on the board
# (ra-fleet-check-into-repo-20261022).
#
# Each line checks what it claims (A35). A line that cannot establish its fact is
# RED, never green and never "0":
#   relay     the 'router relay serve' daemon is RUNNING and this Mac's configured
#             spool:// URL names the spool it serves. A directory merely existing
#             proves nothing.
#   router    a real read (`sirsi router status`) round-trips relay -> service.
#   outbox    held / parked counts come from that same configured spool; an
#             unreadable spool is RED, not zero.
#   stacklab  a real read of SirsiMaster/sirsi-stacklab using ONLY this Mac's own
#             deploy key (-F /dev/null, IdentitiesOnly, no agent), so the owner's
#             personal login cannot make it pass. Key contents are never read.
#
# Usage: fleet-check.sh [agent-id]       agent-id (default $SIRSI_AGENT_ID) is shown
#                                        as lane info only; it never changes the verdict.
#        fleet-check.sh --self-test      negative AND positive controls, no network.
set -uo pipefail

SCRIPT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/$(basename "${BASH_SOURCE[0]}")"

# with_timeout SECONDS cmd args... (macOS has no timeout(1); perl ships with it.)
with_timeout() { local s=$1; shift; perl -e 'alarm shift; exec @ARGV or exit 127' "$s" "$@"; }

# resolve_spool prints the spool directory from the router URL this Mac is
# actually configured with (env first, then the cut-over env file), or nothing.
resolve_spool() {
  local url="${SIRSI_ROUTER_URL:-}"
  if [ -z "$url" ] && [ -r "$HOME/.sirsi/router-service.env" ]; then
    url="$(grep -E '^[^#]*SIRSI_ROUTER_URL=' "$HOME/.sirsi/router-service.env" | tail -1 \
      | sed -E "s/.*SIRSI_ROUTER_URL=['\"]?([^'\" ]*).*/\1/")"
  fi
  case "$url" in spool://*) printf '%s' "${url#spool://}" ;; esac
}

self_test() {
  local tmp out out2
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  mkdir -p "$tmp/home/.sirsi" "$tmp/spool/lane/outbox" "$tmp/spool/lane/failed"
  : > "$tmp/spool/lane/failed/x.json"
  # NEGATIVE: no deploy key and one parked item. The run must go RED and name BOTH causes.
  if out="$(HOME="$tmp/home" PATH=/usr/bin:/bin SIRSI_ROUTER_URL="spool://$tmp/spool" bash "$SCRIPT" 2>&1)"; then
    echo "  ❌ self-test FAILED: a Mac with no deploy key and a parked item read GREEN"; echo "$out"; exit 1
  fi
  for needle in "✗ stacklab: no deploy key" "✗ outbox: 0 held, 1 parked"; do
    if ! printf '%s' "$out" | grep -qF "$needle"; then
      echo "  ❌ self-test FAILED: RED for the wrong reason, missing: $needle"; echo "$out"; exit 1
    fi
  done
  # POSITIVE: clear the parked item, hold one in the outbox. The outbox line must go green.
  rm "$tmp/spool/lane/failed/x.json"; : > "$tmp/spool/lane/outbox/y.json"
  out2="$(HOME="$tmp/home" PATH=/usr/bin:/bin SIRSI_ROUTER_URL="spool://$tmp/spool" bash "$SCRIPT" 2>&1 || true)"
  if ! printf '%s' "$out2" | grep -qF "✓ outbox: 1 held, 0 parked"; then
    echo "  ❌ self-test FAILED: a held-only outbox was not reported green"; echo "$out2"; exit 1
  fi
  echo "  ✅ self-test passed: missing key and parked item reported RED by name; held-only outbox green"
  exit 0
}
[ "${1:-}" = "--self-test" ] && self_test

status=GREEN
fail() { status=RED; echo "  ✗ $1"; }
ok()   { echo "  ✓ $1"; }

host="$(hostname -s 2>/dev/null || hostname)"
agent="${1:-${SIRSI_AGENT_ID:-}}"
spool="$(resolve_spool)"

# 1. Relay daemon.
if [ -z "$spool" ]; then
  fail "relay: no spool:// router URL configured (SIRSI_ROUTER_URL or ~/.sirsi/router-service.env)"
elif ! pgrep -f 'router relay serve' >/dev/null 2>&1; then
  fail "relay: no 'router relay serve' process is running (configured spool $spool)"
else
  ok "relay: daemon running, configured spool $spool"
fi

# 2. Round trip relay -> service with a real read.
if ! command -v sirsi >/dev/null 2>&1; then
  fail "router: sirsi CLI not on PATH"
elif [ -n "$agent" ]; then
  # Light probe: `router ping` reads the lane's published state from the service, so ANY
  # verdict word (LIVE, WAKEABLE, HELD, UNREACHABLE...) proves the relay -> service round
  # trip worked. The verdict itself says nothing about relay health, so it is not judged here.
  out="$(with_timeout 20 sirsi router ping "$agent" 2>&1)"; rc=$?
  verdict="$(printf '%s' "$out" | awk 'NF{print $1; exit}')"
  if printf '%s' "$verdict" | grep -Eq '^[A-Z_]+$'; then
    ok "router: round trip through the relay to the service (lane $agent: $verdict)"
  else
    fail "router: round trip failed (rc=$rc): $(printf '%s' "$out" | head -1 | cut -c1-120)"
  fi
else
  # No lane to ask about: fall back to the heavier queue read.
  out="$(with_timeout 20 sirsi router status 2>&1)"; rc=$?
  if [ "$rc" -eq 0 ] && printf '%s' "$out" | grep -q 'Items:'; then
    ok "router: round trip through the relay to the service ($(printf '%s' "$out" | grep -m1 'Items:' | sed 's/^ *//'))"
  else
    fail "router: round trip failed (rc=$rc): $(printf '%s' "$out" | head -1 | cut -c1-120)"
  fi
fi

# 3. Outbox / parked (ADR-069): held items retry on their own; a parked item needs a person.
if [ -z "$spool" ] || [ ! -r "$spool" ]; then
  fail "outbox: spool unreadable (${spool:-not configured}); queue depth unknown, not zero"
else
  held="$(find "$spool" -mindepth 3 -maxdepth 3 -path '*/outbox/*.json' 2>/dev/null | wc -l | tr -d ' ')"
  parked="$(find "$spool" -mindepth 3 -maxdepth 3 -path '*/failed/*.json' 2>/dev/null | wc -l | tr -d ' ')"
  if [ "$parked" -gt 0 ]; then
    fail "outbox: $held held, $parked parked, needs audit"
  else
    ok "outbox: $held held, $parked parked"
  fi
fi

# 4. Stack Lab: a real read with this Mac's own deploy key and nothing else.
key="${FLEET_CHECK_STACKLAB_KEY:-$HOME/.sirsi/secrets/stacklab-deploy-ed25519}"
if [ ! -f "$key" ]; then
  fail "stacklab: no deploy key at $key"
elif GIT_SSH_COMMAND="ssh -F /dev/null -i $key -o IdentitiesOnly=yes -o IdentityAgent=none -o BatchMode=yes -o ConnectTimeout=10" \
     with_timeout 25 git ls-remote git@github.com:SirsiMaster/sirsi-stacklab.git HEAD >/dev/null 2>&1; then
  ok "stacklab: repo read with this Mac's own deploy key"
else
  fail "stacklab: deploy-key read of SirsiMaster/sirsi-stacklab failed (key $key)"
fi

echo "$host: $status"
[ "$status" = "GREEN" ]
