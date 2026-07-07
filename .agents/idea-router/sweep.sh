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

# 0. Orphaned-build-tree reaper (2026-07-04 incident: killed `go test` runs leave
#    go-build*/sirsi-integration-* trees in $TMPDIR forever — 7,689 of them ate
#    1.3 TB in 36h during the worker runaway). Reap the two known-orphan patterns
#    older than 24h; live builds are younger and untouched. Self-heals silently
#    (surfaces canon: only alarm on what an operator must act on).
TMPT=$(getconf DARWIN_USER_TEMP_DIR 2>/dev/null)
if [ -n "$TMPT" ] && [ -d "$TMPT" ]; then
  reaped=$(find "$TMPT" -maxdepth 1 \( -name "go-build*" -o -name "sirsi-integration-*" \) -mmin +1440 2>/dev/null | wc -l | tr -d " ")
  if [ "$reaped" -gt 0 ]; then
    find "$TMPT" -maxdepth 1 \( -name "go-build*" -o -name "sirsi-integration-*" \) -mmin +1440 -exec rm -rf {} + 2>/dev/null
    echo "[$(ts)] reaped $reaped orphaned build tree(s) >24h from $TMPT" >> "$LOG"
  fi
fi

# 1. router daemon of record loaded — since PR #155 the single backstop is the
#    Horus supervisor (ai.sirsi.horus.agent-router); the legacy trio
#    (com.sirsi.idea-router, -sweep, ai.sirsi.registry-police) was deliberately
#    migrated away and MUST NOT be alarmed on (that alarm fired hourly forever
#    on correct state — the 2026-07-04 noise class).
if ! launchctl list | awk '{print $3}' | grep -qx ai.sirsi.horus.agent-router; then
  fail "launchd job ai.sirsi.horus.agent-router (router supervisor) NOT loaded"
fi

# 2. dispatch.sh recent activity — dispatch.sh auto-spawns codex-* agents ONLY
#    (interactive claude agents are pull-model and never blind-spawned). Under the
#    pull-only observer model a stale dispatch.log is EXPECTED when no codex work is
#    queued, so alarm ONLY when codex items are actually waiting to be dispatched —
#    otherwise this fires forever on a moot condition (surfaces: current+actionable only).
codex_pending=0
for _f in "$ROUTER_ROOT"/items/*.md; do
  [ -f "$_f" ] || continue
  grep -qE '^to:[[:space:]]*"?codex-' "$_f" || continue
  grep -qE '^status:[[:space:]]*"?open"?[[:space:]]*$' "$_f" || continue
  codex_pending=$((codex_pending+1))
done
if [ "$codex_pending" -gt 0 ]; then
  last_dispatch=$(grep -E '^\[[0-9-]+T' "$ROUTER_ROOT/logs/dispatch.log" 2>/dev/null | tail -1 | head -c 25)
  if [ -z "$last_dispatch" ]; then
    fail "dispatch.log empty but $codex_pending codex item(s) pending"
  else
    last_epoch=$(date -j -f "[%Y-%m-%dT%H:%M:%S" "$last_dispatch" "+%s" 2>/dev/null || echo 0)
    now_epoch=$(date "+%s")
    if [ $((now_epoch - last_epoch)) -gt 86400 ]; then
      fail "dispatch.sh stale 24h+ with $codex_pending codex item(s) waiting (last: $last_dispatch)"
    fi
  fi
fi

# 3. Per-thread watchers: every pidfile must point to a live PID
for pf in /tmp/sirsi-router-watch-*.pid; do
  [ -f "$pf" ] || continue
  pid=$(cat "$pf" 2>/dev/null)
  if [ -z "$pid" ] || ! kill -0 "$pid" 2>/dev/null; then
    # Self-healed cruft, NOT an alarm (surfaces canon: if nothing remains for
    # an operator to act on, it must not alarm). Session watchers die with
    # their CLI sessions by design; the sweep reaps the leftover pidfile and
    # logs it. 2026-07-07: 26 of these raised a FAIL + an inbox alarm item
    # for a condition this very loop had already fixed. The ALARMING check
    # stays below: an ACTIVE thread with NO watcher pidfile persists after
    # this sweep and genuinely needs action.
    rm -f "$pf"
    echo "[$(ts)] reaped dead watcher pidfile $pf (pid $pid gone — self-heal, no alarm)" >> "$LOG"
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

# 5. Probe round-trip: send → confirm seen by dispatcher → close.
# 5a. SELF-CLEANING FIRST (§2b bounded-emitters law, ADR-035): any open probe
#     older than this sweep is a leftover from a run whose close step broke
#     (2026-07-05/07: the wake-loop crash-loop window left 45 open probes and
#     the fabric board read "46 items waiting"). Close stale probes BEFORE
#     emitting a new one so the probe backlog can never exceed 1 regardless
#     of past failures — a timestamp-keyed emitter without a reaper is the
#     11,564-item flood in miniature.
for stale in "$ROUTER_ROOT"/items/*-claude-pantheon-claude-pantheon-sweep-probe-*.md; do
  [ -f "$stale" ] || continue
  grep -q "^status: open" "$stale" || continue
  stale_id="$(basename "$stale" .md)"
  "$SIRSI" router close "$stale_id" --result "stale sweep probe — superseded by the current sweep (self-clean)" >/dev/null 2>&1 \
    && echo "[$(ts)] closed stale probe $stale_id" >> "$LOG"
done

probe_title="sweep-probe-$(date +%s)"
probe_id=$("$SIRSI" router send --from claude-pantheon --to claude-pantheon \
  --title "$probe_title" --instructions "automated sweep probe — close on receive" 2>&1 \
  | awk -F': ' '/Sent|Deduped/{print $NF}' | awk '{print $1}')
if [ -z "$probe_id" ]; then
  fail "probe send failed"
else
  sleep 12
  if ! grep -q "$probe_title\|claude-pantheon.*item.*to dispatch" "$ROUTER_ROOT/logs/dispatch.log" 2>/dev/null; then
    : # not an error per se — dispatcher only logs when items found, may have processed silently
  fi
  if ! "$SIRSI" router close "$probe_id" --result "sweep ok" >/dev/null 2>&1; then
    # A failed close must be VISIBLE — a silent one is how probes piled up.
    fail "probe close failed for $probe_id"
  fi
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
