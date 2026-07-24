#!/bin/bash
# 𓂀 Registry Police — A27 enforcement + process accountability
# Two-tier model:
#   Tier 1 (agents): MUST register + run a heartbeat loop. Police auto-discovers,
#                    flags unmappable sessions, and flags registered-but-not-looping.
#   Tier 2 (all else): recorded read-only via `sirsi thread scout`.
# Read-only + advisory: never kills, renices, or steers. Writes findings to the
# router as an item addressed to claude-pantheon (the A27 heartbeat picks them up).
set -uo pipefail
REPO="/Users/thekryptodragon/Development/sirsi-pantheon"
ROUTER="$REPO/.agents/idea-router"
LOG="$ROUTER/police/police.log"
STAMP="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
cd "$REPO" || exit 1
. "$ROUTER/router-env.sh"

LOG="$(router_ensure_log "$LOG" "/tmp/sirsi-registry-police.log")" || exit 1
SIRSI="$(router_resolve_sirsi "$REPO")" || { echo "[$STAMP] ⚠ sirsi binary not found — police cannot run" >>"$LOG"; exit 0; }

# 1. Reconcile registry with reality (auto-registers mappable agent sessions)
DISCOVER=$("$SIRSI" thread discover --json 2>/dev/null)
# 2. Record the full process table read-only
"$SIRSI" thread scout >/dev/null 2>&1

# 3. Count unmappable agent sessions (running agents with no repo identity)
UNMAPPABLE=$(printf '%s' "$DISCOVER" | python3 -c 'import json,sys
try: d=json.load(sys.stdin)
except Exception: print(0); sys.exit()
print(d.get("unmappable",0) if isinstance(d,dict) else 0)' 2>/dev/null || echo 0)

# 4. Flag registered-but-not-looping PER AGENT (#230 model): an agent violates only
#    if it has open inbox items AND zero armed watchers across ALL its live threads.
#    Redundant same-agent sessions (e.g. claude-home's CCD sessions) are benign, so
#    a per-THREAD stale count over-fires — count per AGENT to match the Go doctor's
#    router.AgentLoopDead. An armed watcher = a live, non-stale, active thread that
#    watches its own agent id. Interactive `user` is never an A27 loop agent — exclude.
#    Sources: `router status` "Open by recipient" (open items/agent) + `thread list
#    --json` (armed watchers/agent). Trust the CLI's own `stale` — do NOT reinvent
#    heartbeat math on a guessed field.
OPEN_BY_RECIPIENT="$("$SIRSI" router status 2>/dev/null)"
LAUNCHD_WAKE_AGENTS="$(launchctl list 2>/dev/null | awk '{print $3}' | sed -n 's/^ai\.sirsi\.router\.wake\.//p' | tr '\n' ' ')"
export LAUNCHD_WAKE_AGENTS
STALE=$("$SIRSI" thread list --json 2>/dev/null | OPEN_BY_RECIPIENT="$OPEN_BY_RECIPIENT" python3 -c '
import json,os,re,sys
open_agents={}
sect=False
for line in os.environ.get("OPEN_BY_RECIPIENT","").splitlines():
    if "Open by recipient" in line: sect=True; continue
    if sect:
        m=re.match(r"\s+([A-Za-z0-9_-]+):\s+(\d+)\s*$", line)
        if m: open_agents[m.group(1)]=int(m.group(2))
        elif line.strip()=="": break
try: d=json.load(sys.stdin)
except Exception: print(0); sys.exit()
rows=d.get("threads",d) if isinstance(d,dict) else d
armed=set()
for t in (rows if isinstance(rows,list) else []):
    if not isinstance(t,dict) or t.get("stale") is True: continue
    th=t.get("thread",{})
    if th.get("status")!="active": continue
    a=th.get("agent_id")
    # An agent is armed if ANY live thread watches it — not only if it watches
    # ITSELF. The fleet supervisor (horus-supervisor) watches every agent on
    # their behalf; crediting self-watch only flagged the whole fleet (2026-07-24:
    # 10 "violations" while 10 launchd wake jobs were armed).
    for w in (th.get("watches") or []): armed.add(w)
    if a: armed.add(a) if a in (th.get("watches") or []) else None
# launchd wake jobs ARE armed watchers (ai.sirsi.router.wake.<agent>) — they are
# the post-cutover durable loop, and they live in launchd, not the thread
# registry. Same lesson as sweep.sh #284: key liveness on the real mechanism.
for label in os.environ.get("LAUNCHD_WAKE_AGENTS","").split():
    if label: armed.add(label)
n=sum(1 for a,c in open_agents.items() if a!="user" and c>0 and a not in armed)
print(n)' 2>/dev/null || echo 0)

VIOLATIONS=$(( ${UNMAPPABLE:-0} + ${STALE:-0} ))
echo "[$STAMP] police: unmappable=$UNMAPPABLE stale-loop=$STALE" >>"$LOG"

# 5. If violations exist, file ONE advisory router item to claude-pantheon (deduped by day)
if [ "$VIOLATIONS" -gt 0 ]; then
  FLAG="$ROUTER/police/.last-alarm-$(date -u +%Y%m%d)"
  if [ ! -f "$FLAG" ]; then
    BODY="$ROUTER/police/.alarm-body.md"
    {
      echo "# Registry Police Alarm — $STAMP"
      echo
      echo "A27 two-tier accountability check found issues:"
      echo
      echo "- **$UNMAPPABLE unmappable agent session(s)** — running agents launched outside any known repo (cwd=\$HOME). They have no agent identity and no inbox. Operator must register them with an explicit repo, or relaunch from the repo dir."
      echo "- **$STALE registered-but-not-looping agent(s)** — have open inbox items but zero armed watchers across all their live threads (A27 violation, per-agent per #230)."
      echo
      echo "Run \`sirsi thread discover\` and \`sirsi thread list\` to inspect. Police is read-only/advisory; no process was killed or steered."
    } > "$BODY"
    "$SIRSI" router send --from registry-police --to claude-pantheon \
      --title "Registry police: $VIOLATIONS A27 accountability issue(s)" \
      --instructions @"$BODY" --quiet >/dev/null 2>&1 && touch "$FLAG"
    # prune old day-flags
    find "$ROUTER/police" -name '.last-alarm-*' -mtime +2 -delete 2>/dev/null
  fi
fi
echo "[$STAMP] police PASS" >>"$LOG"
