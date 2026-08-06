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
# The token lives in the secrets file (mode 600), NEVER in the plist: the plist
# copy leaked into transcripts and was scrubbed 2026-07-23. Sourcing here
# OVERRIDES whatever launchd exported — including the scrub placeholder that
# passed the old non-empty check and crash-looped the worker at auth every 30s
# (claude-home RCA: "un-quarantining alone crash-loops").
# launchd gui jobs get no TMPDIR, and bash heredocs need one — every item
# attempt died with "cannot create temp file for here document" until this.
# A private tmp under ~/.sirsi also survives /tmp cleaners.
export TMPDIR="$HOME/.sirsi/tmp"; mkdir -p "$TMPDIR"

[ -f "$HOME/.sirsi/secrets/claude-worker.env" ] && . "$HOME/.sirsi/secrets/claude-worker.env"
[ -n "${CLAUDE_CODE_OAUTH_TOKEN:-}" ] || { log "ERROR: CLAUDE_CODE_OAUTH_TOKEN unset — headless claude can't auth; exiting"; exit 1; }
case "$CLAUDE_CODE_OAUTH_TOKEN" in *SCRUBBED*) log "ERROR: CLAUDE_CODE_OAUTH_TOKEN is the scrub placeholder, not a token — rotate into ~/.sirsi/secrets/claude-worker.env"; exit 1;; esac
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

# Fetch an item THROUGH THE FACADE (ADR-036/037). The legacy dir
# .agents/idea-router/items/ has been frozen since the store cutover — every
# read of it finds nothing. The loop was fixed to pull via the facade, but
# THREE reads of the legacy path survived (item body, age check, closed check)
# and silently no-op'd every store-era item while fabricating "failed 2x".
# `router show` emits the same ---frontmatter--- + "## Instructions" shape the
# legacy .md had, so every downstream parser below works unchanged.
fetch_item() {
  local id=$1 dest=$2
  (cd "$REPO" && "$SIRSI" router show "$id" 2>/dev/null) > "$dest"
  [ -s "$dest" ]
}

process_item() {
  local id=$1 f=$2

  local from body
  from=$(awk '/^---$/{n++; if(n==2)exit; next} n==1 && /^from:/{gsub(/^from:[[:space:]]*"?|"?$/,""); print; exit}' "$f")
  body=$(awk 'f{print} /## Instructions/{f=1}' "$f" | sed '/## Result/,$d')

  log "BUILD START $id (from $from, model=$MODEL)"

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
  # THROUGH THE FACADE, never the legacy dir (ADR-036/037): after the store
  # cutover a send is a store row and items/*.md is never written — this loop
  # scanned the frozen directory and polled an EMPTY world forever while real
  # items sat in the store. The phantom class, inside the worker itself.
  ids=$(cd "$REPO" && "$SIRSI" router pull "$AGENT_ID" 2>/dev/null | sed -n 's/^  • //p')
  if [ -n "$ids" ]; then
    while IFS= read -r id; do
      [ -z "$id" ] && continue
      f="$TMPDIR/item-$$.md"
      fetch_item "$id" "$f" || { log "SKIP $id — router show returned nothing (no attempt counted)"; continue; }
      # Staleness threshold: the worker is the anti-stall BACKSTOP, not a racer.
      # Fresh items belong to whatever attended session is live (claim/lease gap).
      # Age comes from the store's `opened:`, NOT file mtime: stat on the frozen
      # legacy path failed to epoch 0, so every item read as ~55 years old and
      # this backstop never once fired — fresh items were burned immediately.
      opened=$(sed -n 's/^opened:[[:space:]]*"\{0,1\}\([^"]*\)"\{0,1\}[[:space:]]*$/\1/p' "$f" | head -1)
      ots=$(TZ=UTC date -j -f "%Y-%m-%dT%H:%M:%SZ" "$opened" +%s 2>/dev/null || echo 0)
      if [ "$ots" -eq 0 ]; then
        log "SKIP $id — unparseable opened='$opened'; refusing to guess age (no attempt counted)"; continue
      fi
      age=$(( $(date +%s) - ots ))
      if [ "$age" -lt "$MIN_AGE" ]; then continue; fi
      # RAM guardrail (Horus): defer BEFORE spending an attempt. This ran inside
      # process_item after the counter rose, so a low-RAM box permanently
      # abandoned items it had never actually tried to build.
      fg=$(free_gb)
      if python3 -c "import sys;sys.exit(0 if $fg < $MIN_FREE_GB else 1)"; then
        log "DEFER $id — only ${fg}GB free (< ${MIN_FREE_GB}GB); leaving queued to avoid Jetsam"
        continue
      fi
      # Skip fabric self-probes (sweep-probe / arm-proof): these are liveness
      # pings, not build tasks — the triage tier closes them. A build worker
      # agentic-building a heartbeat is the 2026-07-03 waste bug.
      case "$id" in *sweep-probe*|*arm-proof*|*-${AGENT_ID}-${AGENT_ID}-*)
        (cd "$REPO" && "$SIRSI" router close "$id" --agent "$AGENT_ID" --result "self-probe — closed by build worker (no build task); fabric alive" >/dev/null 2>&1)
        log "SKIP+CLOSE self-probe $id"; continue;; esac
      # Skip non-build item types: decision + review items have no build artifact
      # by construction — a build worker can only spin for 2400s then time out.
      # Root cause of the ADR-006 burn: type:decision was queued, looped twice,
      # burned 80 min. Route back to a human lane instead of consuming quota.
      itype=$(sed -n 's/^type:[[:space:]]*"\{0,1\}\([^"]*\)"\{0,1\}[[:space:]]*$/\1/p' "$f" | head -1)
      case "$itype" in decision|review)
        log "SKIP $id — type=$itype has no build target; leaving open for attended session"
        continue;; esac
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
          # Mark BEFORE sending. `router send` is a slow subprocess and this
          # worker gets SIGTERMed by launchd mid-poll; a kill landing between
          # send and touch loses the marker and re-escalates on every restart.
          # Losing one escalation beats re-running the 2026-07-04 11,564 flood.
          touch "$af.gaveup"
          log "ABANDON $id after $n failed attempts — surfacing to owner ONCE"
        (cd "$REPO" && "$SIRSI" router send --from "$AGENT_ID" --to claude-home --type review           --title "WORKER GAVE UP: $id failed ${MAX_ATTEMPTS}x"           --instructions "The headless build worker failed this item ${MAX_ATTEMPTS} times and has STOPPED retrying it (loop-proof). Needs a human/owner: build it directly or re-scope. Item stays OPEN, no longer auto-pulled." >/dev/null 2>&1)
        fi
        continue
      fi
      # Count only REAL processing attempts (increment just before spending one).
      echo "$((n+1))" > "$af"
      process_item "$id" "$f"
      # Success clears the counter (item is closed by the session on success).
      # Re-read through the facade — the legacy path checked here never existed,
      # so the counter never cleared and even a clean build trended to ABANDON.
      if fetch_item "$id" "$f" && grep -q "^status: closed" "$f"; then rm -f "$af" "$af.gaveup"; fi
      rm -f "$f"
    done <<< "$ids"
  fi
  sleep "$POLL"
done
