#!/bin/bash
# Surfaces Pantheon system-health as a one-line SessionStart signal.
# Reads the CANONICAL green/amber/red `status` from `sirsi diagnose` (the rubric
# lives in Go: internal/guard/doctor.go HealthStatus) instead of re-deriving from
# raw severities — historical 7-day trends are AMBER, not RED.
# Fails LOUD: if sirsi is missing or diagnose breaks, say so instead of going silent.
#
# CACHED (2026-07-02): a full diagnose per SessionStart × a dozen live agent
# threads piled up 7+ concurrent 30s+ diagnose runs (log-show scans), pegging the
# box and firing an Anubis toast every 10–15s. One diagnose per 5 minutes
# machine-wide now; every other session start reads the cached line. A lock dir
# prevents the thundering herd; a hard timeout keeps a slow run from hanging a
# session start.
CACHE="$HOME/.cache/sirsi/health-line"
LOCK="$HOME/.cache/sirsi/health-line.lock"
TTL=300
mkdir -p "$HOME/.cache/sirsi"

fresh() {
  [ -f "$CACHE" ] || return 1
  local age=$(( $(date +%s) - $(stat -f%m "$CACHE" 2>/dev/null || echo 0) ))
  [ "$age" -lt "$TTL" ]
}

if fresh; then cat "$CACHE"; exit 0; fi

if ! command -v sirsi >/dev/null 2>&1; then
  echo "health:⚠ sirsi (Pantheon) not on PATH — system health unmonitored"
  exit 0
fi

# Only one session refreshes at a time; everyone else takes the stale cache.
if ! mkdir "$LOCK" 2>/dev/null; then
  [ -f "$CACHE" ] && cat "$CACHE" || echo "health:… refreshing in another session"
  exit 0
fi
trap 'rmdir "$LOCK" 2>/dev/null' EXIT

LINE=$(timeout 60 sirsi diagnose --json 2>/dev/null | python3 -c '
import json,sys
try:
    d=json.load(sys.stdin)
except Exception:
    print("health:⚠ sirsi diagnose returned no data"); sys.exit()
f=d.get("findings",[])
status=d.get("status")
icon={"green":"🟢","amber":"🟡","red":"🔴"}.get(status)
if icon is None:
    # Fallback for an older binary without the canonical status field.
    worst=max([x.get("severity",0) for x in f], default=0)
    icon="🟢" if worst==0 else ("🟡" if worst==1 else "🔴")
score=d.get("score")
if score is None:
    score=max(0,100-8*sum(1 for x in f if x.get("severity",0)==1)-25*sum(1 for x in f if x.get("severity",0)>=2))
# Name the non-OK checks for context.
bad=[x.get("check","?") for x in f if x.get("severity",0)>=2]
tail=(" — "+", ".join(bad)) if bad else ""
print(f"health:{icon} {score}/100{tail}")
' || echo "health:⚠ python3 unavailable for health parse")
[ -n "$LINE" ] || LINE="health:⚠ diagnose timed out (>60s) — box may be under load"
printf '%s\n' "$LINE" | tee "$CACHE"
