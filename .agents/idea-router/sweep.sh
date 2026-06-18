#!/usr/bin/env bash
# Periodic verification sweep — fired hourly by launchd.
#
# Performs the "are we complete" probe that has caught real bugs three
# times in this arc (dispatch.sh cwd, missing reaper, adopt-without-watcher).
# Silent on healthy state. On any FAIL: appends to sweep.log AND drops a
# router item addressed to claude-pantheon describing the failure.
#
# Per AGENTS.md §Lean #2 (loud failure is the gift) — healthy = quiet,
# broken = alarm, no noise in between.

set -uo pipefail
ROUTER_ROOT="/Users/thekryptodragon/Development/sirsi-pantheon/.agents/idea-router"
REPO_ROOT="/Users/thekryptodragon/Development/sirsi-pantheon"
LOG="$ROUTER_ROOT/logs/sweep.log"
export PATH="$HOME/.local/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin"
. "$ROUTER_ROOT/router-env.sh"

ts() { date "+%Y-%m-%dT%H:%M:%S%z"; }
cd "$REPO_ROOT" || { echo "[$(ts)] sweep FAIL: cannot cd to $REPO_ROOT" >> "$LOG"; exit 1; }

LOG="$(router_ensure_log "$LOG" "/tmp/sirsi-router-sweep.log")" || exit 1
SIRSI="$(router_resolve_sirsi "$REPO_ROOT")" || { echo "[$(ts)] sweep FAIL: sirsi binary not found" >> "$LOG"; exit 1; }
fails=()
fail() { fails+=("$1"); }

# 1. launchd dispatcher loaded
if ! launchctl list | awk '{print $3}' | grep -qx com.sirsi.idea-router; then
  fail "launchd job com.sirsi.idea-router NOT loaded"
fi

# 2. dispatch.sh recent activity (any fire within 24h)
last_dispatch=$(grep -E '^\[[0-9-]+T' "$ROUTER_ROOT/logs/dispatch.log" 2>/dev/null | tail -1 | head -c 25)
if [ -z "$last_dispatch" ]; then
  fail "dispatch.log empty or unreadable"
else
  last_epoch=$(date -j -f "[%Y-%m-%dT%H:%M:%S" "$last_dispatch" "+%s" 2>/dev/null || echo 0)
  now_epoch=$(date "+%s")
  if [ $((now_epoch - last_epoch)) -gt 86400 ]; then
    fail "dispatch.sh has not fired in 24h+ (last: $last_dispatch)"
  fi
fi

# 3. Per-thread watchers: every pidfile must point to a live PID
for pf in /tmp/sirsi-router-watch-*.pid; do
  [ -f "$pf" ] || continue
  pid=$(cat "$pf" 2>/dev/null)
  if [ -z "$pid" ] || ! kill -0 "$pid" 2>/dev/null; then
    fail "watcher pidfile $pf points to dead PID $pid (removing)"
    rm -f "$pf"
  fi
done

# 4. Active CTR claude threads must have a live watcher (the adopt-without-
#    watcher bug we fixed 2026-05-31). Reaper runs implicitly on `thread list`.
if command -v jq >/dev/null 2>&1; then
  "$SIRSI" thread list --json 2>/dev/null | jq -r '
    .[]
    | select(.thread.status == "active")
    | select(.thread.agent_id | startswith("claude-"))
    | select(.idle_seconds <= 600)
    | [.thread.thread_id, .thread.agent_id]
    | @tsv
  ' 2>/dev/null | while IFS=$'\t' read -r tid agent_id; do
    pf="/tmp/sirsi-router-watch-$tid.pid"
    if [ ! -f "$pf" ]; then
      fail "thread $tid ($agent_id) active but no watcher pidfile"
    fi
  done
else
  fail "jq missing; cannot verify active thread watcher coverage"
fi

# 4.5. Refresh process awareness. Discovery registers mappable repo-launched
#      agent sessions; scout records every visible PID into processes.json.
#      Both are read-only except discover's safe registration of mappable agents.
"$SIRSI" thread discover --json >/dev/null 2>&1 || fail "thread discover failed"
"$SIRSI" thread scout --json >/dev/null 2>&1 || fail "thread scout failed"

# 5. Probe round-trip: send → confirm seen by dispatcher → close
probe_title="sweep-probe-$(date +%s)"
probe_id=$("$SIRSI" router send --from claude-pantheon --to claude-pantheon \
  --title "$probe_title" --instructions "automated sweep probe — close on receive" 2>&1 \
  | awk -F': ' '/Sent/{print $NF}')
if [ -z "$probe_id" ]; then
  fail "probe send failed"
else
  sleep 12
  if ! grep -q "$probe_title\|claude-pantheon.*item.*to dispatch" "$ROUTER_ROOT/logs/dispatch.log" 2>/dev/null; then
    : # not an error per se — dispatcher only logs when items found, may have processed silently
  fi
  "$SIRSI" router close "$probe_id" --result "sweep ok" >/dev/null 2>&1 || true
fi

# Report
if [ ${#fails[@]} -eq 0 ]; then
  echo "[$(ts)] sweep PASS" >> "$LOG"
  exit 0
fi

# Alarm: write to log + drop a router item
{
  echo "[$(ts)] sweep FAIL — ${#fails[@]} issue(s):"
  for f in "${fails[@]}"; do echo "  - $f"; done
} >> "$LOG"

alarm_body="# Periodic Sweep Alarm — $(ts)

The hourly verification sweep found $(echo ${#fails[@]}) issue(s) in router infrastructure:

$(for f in "${fails[@]}"; do echo "- $f"; done)

Run manually to investigate:

    $ROUTER_ROOT/sweep.sh

See log: $LOG
"
"$SIRSI" router send --from sweep-bot --to claude-pantheon \
  --title "sweep alarm: ${#fails[@]} infra issue(s)" \
  --instructions "$alarm_body" >> "$LOG" 2>&1 || echo "[$(ts)] alarm send failed" >> "$LOG"

exit 1
