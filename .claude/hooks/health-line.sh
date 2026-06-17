#!/bin/bash
# Surfaces Pantheon system-health as a one-line SessionStart signal.
# Reads the CANONICAL green/amber/red `status` from `sirsi diagnose` (the rubric
# lives in Go: internal/guard/doctor.go HealthStatus) instead of re-deriving from
# raw severities — historical 7-day trends are AMBER, not RED.
# Fails LOUD: if sirsi is missing or diagnose breaks, say so instead of going silent.
if ! command -v sirsi >/dev/null 2>&1; then
  echo "health:⚠ sirsi (Pantheon) not on PATH — system health unmonitored"
  exit 0
fi
sirsi diagnose --json 2>/dev/null | python3 -c '
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
' || echo "health:⚠ python3 unavailable for health parse"
