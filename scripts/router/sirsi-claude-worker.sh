#!/bin/bash
# sirsi-claude-worker.sh — headless agentic Claude router worker.
#
# Polls a claude-* agent's router inbox and EXECUTES build/implementation tasks
# with a real agentic `claude -p` session (file edits, go build/test, git, PR) —
# headless, on the owner's Claude subscription (CLAUDE_CODE_OAUTH_TOKEN), NOT a
# live interactive session and NOT the orchestrator's session token budget.
#
# This is the agentic counterpart to sirsi-gemma-worker.sh: gemma drafts text
# (single-shot, zero-token); THIS worker ships code (multi-step, tool-using).
# Together they let the fleet self-build without the owner opening CLI sessions.
#
# WHY THIS EXISTS (owner directive 2026-06-22): gemma cannot agentic-build
# (single-shot model) and interactive claude-* threads can't be blind-spawned;
# the headless-auth fix (long-lived CLAUDE_CODE_OAUTH_TOKEN) finally makes a
# headless agentic claude worker possible. See feedback_auth_expiry +
# feedback_no_idle_threads_prd_stophook.
#
# SAFETY: the spawned `claude -p` inherits the user-scope PreToolUse guard hook
# (~/.claude/hooks/guard-pretool.py: blocks .app writes, commits-to-main,
# secret-leaks, rm -rf roots). Each task runs in its OWN git worktree off main
# (no clobbering peers / the orchestrator). Build-only mandate: open a PR, route
# a completion to claude-home — NEVER merge, never push main, never touch prod.

set -u
AGENT_ID=${SIRSI_CLAUDE_WORKER_AGENT:-claude-pantheon}
REPO=${SIRSI_CLAUDE_WORKER_REPO:-$HOME/Development/sirsi-pantheon}
SIRSI=$HOME/.local/bin/sirsi
export SIRSI_SUPERVISOR=0   # headless: never run interactive session hooks (rc=1 loop fix 2026-07-03)
CLAUDE=$(command -v claude || echo "$HOME/.local/bin/claude")
MODEL=${SIRSI_CLAUDE_WORKER_MODEL:-claude-sonnet-4-6}   # sonnet: fast + quota-friendly for scoped builds
POLL=${SIRSI_CLAUDE_WORKER_POLL:-60}
TASK_TIMEOUT=${SIRSI_CLAUDE_WORKER_TIMEOUT:-2400}        # 40 min hard cap per build
MIN_FREE_GB=${SIRSI_CLAUDE_WORKER_MIN_FREE:-8}           # Horus guardrail: never Jetsam a sibling
LOG=$HOME/.sirsi/claude-worker-${AGENT_ID}.log
ATTEMPT_DIR=$HOME/.sirsi/worker-attempts/${AGENT_ID}
MAX_ATTEMPTS=${SIRSI_WORKER_MAX_ATTEMPTS:-2}
MIN_AGE=${SIRSI_WORKER_MIN_AGE:-1800}   # anti-stall backstop: leave fresh items to attended sessions
mkdir -p "$ATTEMPT_DIR"
mkdir -p "$(dirname "$LOG")"

log(){ echo "[$(date -u +%FT%TZ)] $*" >> "$LOG"; }

# Auth: the long-lived headless token MUST be present (launchd setenv or env).
[ -n "${CLAUDE_CODE_OAUTH_TOKEN:-}" ] || { log "ERROR: CLAUDE_CODE_OAUTH_TOKEN unset — headless claude can't auth; exiting"; exit 1; }
[ -x "$CLAUDE" ] || { log "ERROR: claude CLI not found at $CLAUDE"; exit 1; }

free_gb() {
  vm_stat 2>/dev/null | python3 -c "
import sys,re
pg=16384;d={}  # Apple Silicon page size (was wrongly 4096 → 4x-low free-RAM, false deferrals)
for l in sys.stdin:
    m=re.match(r'\"?([\w\s]+)\"?:\s+(\d+)',l)
    if m:d[m.group(1).strip()]=int(m.group(2))*pg
print(round((d.get('Pages free',0)+d.get('Pages inactive',0)+d.get('Pages speculative',0))/1e9,1))"
}

process_item() {
  local id=$1
  local f="$REPO/.agents/idea-router/items/$id.md"
  [ -f "$f" ] || return 0

  local from body
  from=$(awk '/^---$/{n++; if(n==2)exit; next} n==1 && /^from:/{gsub(/^from:[[:space:]]*"?|"?$/,""); print; exit}' "$f")
  body=$(awk 'f{print} /## Instructions/{f=1}' "$f" | sed '/## Result/,$d')

  # RAM guardrail (Horus): defer if the box can't take another build worker.
  local fg; fg=$(free_gb)
  if python3 -c "import sys;sys.exit(0 if $fg < $MIN_FREE_GB else 1)"; then
    log "DEFER $id — only ${fg}GB free (< ${MIN_FREE_GB}GB); leaving queued to avoid Jetsam"
    return 0
  fi

  log "BUILD START $id (from $from, free=${fg}GB, model=$MODEL)"

  local prompt
  prompt="You are ${AGENT_ID}, a headless agentic build worker in the Sirsi fleet. Execute the build task below end-to-end, then STOP. Repo: ${REPO}.

MANDATE + GUARDRAILS (strict):
- Work in a FRESH git worktree off origin/main (e.g. git worktree add /tmp/${AGENT_ID}-\$(date +%s) -b <feat-branch> origin/main). NEVER edit the main checkout, NEVER commit to main, NEVER merge, NEVER push to main, NEVER touch any cloud/prod resource.
- Implement the task. Then verify ALL green: gofmt, go build ./..., go vet ./..., go test ./... (relevant pkgs), golangci-lint if configured. Fix until green. If a check genuinely cannot pass, say so explicitly — never fake it.
- Commit (clear message + Co-Authored-By: Claude), push the feature branch, open a PR (base main) with a CHANGELOG [Unreleased] entry + Refs.
- Route a completion item back to claude-home to bind: sirsi router send --from ${AGENT_ID} --to claude-home --type review --title '<PR#> ready: <task>' --instructions '<what you built, test results, PR url, any caveat>'. Then close THIS item: sirsi router close ${id} --result '<summary + PR#>'.
- If you hit a hard blocker, route the SPECIFIC blocker to claude-home and close this item noting it. Honesty over completion — a clear blocker beats broken code.

THE TASK (from ${from}):
${body}

Begin. Do the whole task, open the PR, route the completion, then stop."

  local out rc
  out=$(cd "$REPO" && timeout "$TASK_TIMEOUT" "$CLAUDE" --print \
        --model "$MODEL" \
        --permission-mode bypassPermissions \
        --dangerously-skip-permissions \
        "$prompt" 2>>"$LOG")
  rc=$?
  if [ $rc -eq 124 ]; then
    log "BUILD TIMEOUT $id (>${TASK_TIMEOUT}s)"
    (cd "$REPO" && "$SIRSI" router send --from "$AGENT_ID" --to claude-home --type review \
      --title "BUILD TIMEOUT: $id exceeded ${TASK_TIMEOUT}s" \
      --instructions "Headless build worker timed out on $id. claude-home: re-scope smaller or build directly. Partial work may exist in a /tmp worktree." >/dev/null 2>&1)
  elif [ $rc -ne 0 ]; then
    log "BUILD ERROR $id (rc=$rc)"
  else
    log "BUILD DONE $id"
  fi
  # The claude -p session itself routes the completion + closes the item on success.
  # If it didn't close (error/timeout), leave it open for claude-home to triage.
}

log "claude-worker started (agent=$AGENT_ID, poll=${POLL}s, model=$MODEL, timeout=${TASK_TIMEOUT}s)"
echo $$ > "$HOME/.sirsi/claude-worker-${AGENT_ID}.pid"
trap 'rm -f "$HOME/.sirsi/claude-worker-${AGENT_ID}.pid"' EXIT

while true; do
  ids=$(cd "$REPO" && AGENT_ID="$AGENT_ID" python3 -c "
import os, re, sys
agent=os.environ['AGENT_ID']
d='.agents/idea-router/items'
for fn in sorted(os.listdir(d)):
    if not fn.endswith('.md'): continue
    txt=open(os.path.join(d,fn)).read()
    m=re.match(r'---\n(.*?)\n---', txt, re.DOTALL)
    if not m: continue
    fm=m.group(1)
    if not re.search(r'^status:\s*\"?open\"?', fm, re.MULTILINE): continue
    if re.search(r'^to:\s*\"?'+re.escape(agent)+r'\"?\s*$', fm, re.MULTILINE): print(fn[:-3])
")
  if [ -n "$ids" ]; then
    while IFS= read -r id; do
      [ -z "$id" ] && continue
      # Staleness threshold: the worker is the anti-stall BACKSTOP, not a racer.
      # Fresh items belong to whatever attended session is live (claim/lease gap).
      f="$REPO/.agents/idea-router/items/$id.md"
      age=$(( $(date +%s) - $(stat -f %m "$f" 2>/dev/null || echo 0) ))
      if [ "$age" -lt "$MIN_AGE" ]; then continue; fi
      # Skip fabric self-probes (sweep-probe / arm-proof): these are liveness
      # pings, not build tasks — the triage tier closes them. A build worker
      # agentic-building a heartbeat is the 2026-07-03 waste bug.
      case "$id" in *sweep-probe*|*arm-proof*|*-${AGENT_ID}-${AGENT_ID}-*)
        (cd "$REPO" && "$SIRSI" router close "$id" --result "self-probe — closed by build worker (no build task); fabric alive" >/dev/null 2>&1)
        log "SKIP+CLOSE self-probe $id"; continue;; esac
      # Bounded retries — the structural loop-proof (2026-07-03). Count attempts
      # per item id; after MAX_ATTEMPTS, abandon + surface to the owner and never
      # pull it again. No item can loop, whatever the failure cause.
      af="$ATTEMPT_DIR/$(echo "$id" | tr '/' '_')"
      n=$(cat "$af" 2>/dev/null || echo 0)
      if [ "$n" -ge "$MAX_ATTEMPTS" ]; then
        # At the cap: escalate ONCE (idempotent marker), then stay SILENT on
        # every future poll. The 2026-07-04 flood (11,564 spam items) was this
        # branch firing per-poll without the marker.
        if [ ! -f "$af.gaveup" ]; then
          log "ABANDON $id after $n failed attempts — surfacing to owner ONCE"
        (cd "$REPO" && "$SIRSI" router send --from "$AGENT_ID" --to claude-home --type review           --title "WORKER GAVE UP: $id failed ${MAX_ATTEMPTS}x"           --instructions "The headless build worker failed this item ${MAX_ATTEMPTS} times and has STOPPED retrying it (loop-proof). Needs a human/owner: build it directly or re-scope. Item stays OPEN, no longer auto-pulled." >/dev/null 2>&1)
          touch "$af.gaveup"
        fi
        continue
      fi
      # Count only REAL processing attempts (increment just before spending one).
      echo "$((n+1))" > "$af"
      process_item "$id"
      # Success clears the counter (item is closed by the session on success).
      if grep -q "^status: closed" "$REPO/.agents/idea-router/items/$id.md" 2>/dev/null; then rm -f "$af"; fi
    done <<< "$ids"
  fi
  sleep "$POLL"
done
