#!/bin/bash
# Surfaces Pantheon system-health as a one-line SessionStart signal.
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
worst=0; warn=0; bad=[]
for x in f:
    s=x.get("severity",0); worst=max(worst,s)
    if s==1: warn+=1
    if s>=2: bad.append(x.get("check","?"))
icon="🟢" if worst==0 else ("🟡" if worst==1 else "🔴")
score=max(0,100-8*warn-25*len(bad))
tail=(" — "+", ".join(bad)) if bad else ""
print(f"health:{icon} {score}/100{tail}")
' || echo "health:⚠ python3 unavailable for health parse"
