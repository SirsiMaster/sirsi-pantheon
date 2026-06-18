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

# 4. Flag registered-but-not-looping: threads the CLI already marks stale.
#    `sirsi thread list --json` emits a list of {idle_seconds, stale, thread:{...}}.
#    Trust the CLI's own `stale` determination — do NOT reinvent heartbeat math on a
#    guessed field name (an absent field counts every live thread as a violation).
STALE=$("$SIRSI" thread list --json 2>/dev/null | python3 -c '
import json,sys
try: d=json.load(sys.stdin)
except Exception: print(0); sys.exit()
rows=d.get("threads",d) if isinstance(d,dict) else d
n=0
for t in (rows if isinstance(rows,list) else []):
    if isinstance(t,dict) and t.get("stale") is True: n+=1
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
      echo "- **$STALE registered-but-not-looping thread(s)** — registered in CTR but no recent heartbeat (A27 violation)."
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
