#!/usr/bin/env bash
# Router dispatch handler — fired by launchd WatchPaths on any change under
# .agents/idea-router/{state.json,items,proposals}.
#
# Handles queues for local agents this workstation can actually wake.
# Pull-model items live in .agents/idea-router/items/*.md; legacy pending
# queues live in state.json[pending][<agent>].
#
# Dispatch stays intentionally small: resolve each local agent, then spawn the
# matching CLI once with a router-focused prompt. No daemon, no polling loop.

set -uo pipefail
ROUTER_ROOT="/Users/thekryptodragon/Development/sirsi-pantheon/.agents/idea-router"
REPO_ROOT="/Users/thekryptodragon/Development/sirsi-pantheon"
LOG="$ROUTER_ROOT/logs/dispatch.log"
export PATH="$HOME/.local/bin:/Applications/Codex.app/Contents/Resources:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin"
CLAUDE_BIN="$HOME/.local/bin/claude"
CODEX_BIN="/Applications/Codex.app/Contents/Resources/codex"
LOCK_ROOT="$ROUTER_ROOT/locks"
. "$ROUTER_ROOT/router-env.sh"

ts() { date "+%Y-%m-%dT%H:%M:%S%z"; }

LOG="$(router_ensure_log "$LOG" "/tmp/sirsi-router-dispatch.log")" || exit 1
if ! mkdir -p "$LOCK_ROOT" 2>/dev/null; then
  echo "[$(ts)] dispatch.sh warning — lock root $LOCK_ROOT unavailable; using /tmp" >> "$LOG"
  LOCK_ROOT="/tmp/sirsi-router-locks"
  mkdir -p "$LOCK_ROOT" || { echo "[$(ts)] dispatch.sh exit — cannot create $LOCK_ROOT" >> "$LOG"; exit 1; }
fi

# cd into the repo upfront so `sirsi` commands can locate the router via
# FindRepoRoot() — launchd's default cwd is `/`, which silently breaks pull.
cd "$REPO_ROOT" || { echo "[$(ts)] dispatch.sh exit — cannot cd to $REPO_ROOT" >> "$LOG"; exit 1; }

SIRSI="$(router_resolve_sirsi "$REPO_ROOT")" || { echo "[$(ts)] dispatch.sh exit — sirsi binary not found" >> "$LOG"; exit 1; }

# Agents this host claims. Keep this explicit so launchd does not pretend to
# deliver to agents that have no local headless wake path.
AGENTS=(claude-pantheon codex-pantheon)

agent_lock_dir() { printf '%s/dispatch-%s.lock' "$LOCK_ROOT" "$1"; }

agent_is_running() {
  agent="$1"
  lock_dir="$(agent_lock_dir "$agent")"
  pid_file="$lock_dir/pid"
  [ -d "$lock_dir" ] || return 1
  if [ -f "$pid_file" ]; then
    pid="$(cat "$pid_file" 2>/dev/null || true)"
    if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
      return 0
    fi
  fi
  rm -rf "$lock_dir"
  return 1
}

start_agent_worker() {
  agent="$1"
  prompt="$2"
  lock_dir="$(agent_lock_dir "$agent")"

  if agent_is_running "$agent"; then
    echo "[$(ts)] $agent skipped: worker already running (lock=$lock_dir)" >> "$LOG"
    return 2
  fi
  if ! mkdir "$lock_dir" 2>/dev/null; then
    if agent_is_running "$agent"; then
      echo "[$(ts)] $agent skipped: worker already running (lock=$lock_dir)" >> "$LOG"
      return 2
    fi
    echo "[$(ts)] $agent skipped: lock contention at $lock_dir" >> "$LOG"
    return 2
  fi

  case "$agent" in
    claude-*)
      if [ ! -x "$CLAUDE_BIN" ]; then
        echo "[$(ts)] $agent blocked: claude CLI not executable at $CLAUDE_BIN" >> "$LOG"
        rm -rf "$lock_dir"
        return 1
      fi
      ( printf '%s\n' "$prompt" | "$CLAUDE_BIN" --print --permission-mode auto >> "$LOG" 2>&1; rm -rf "$lock_dir" ) &
      ;;
    codex-*)
      if [ ! -x "$CODEX_BIN" ]; then
        echo "[$(ts)] $agent blocked: Codex CLI not executable at $CODEX_BIN" >> "$LOG"
        rm -rf "$lock_dir"
        return 1
      fi
      ( printf '%s\n' "$prompt" | "$CODEX_BIN" exec -C "$REPO_ROOT" --sandbox workspace-write - >> "$LOG" 2>&1; rm -rf "$lock_dir" ) &
      ;;
    *)
      echo "[$(ts)] $agent blocked: no dispatch branch for agent family" >> "$LOG"
      rm -rf "$lock_dir"
      return 1
      ;;
  esac
  echo "$!" > "$lock_dir/pid"
  return 0
}

dispatched=0
pull_errors=0
for agent in "${AGENTS[@]}"; do
  # --- Pull-model: items/*.md with to: <agent> and status: open ---
  # Capture exit code AND stderr so a broken pull (missing sirsi, wrong cwd,
  # corrupt items dir) reports loudly instead of silently returning empty.
  pull_out=$("$SIRSI" router pull "$agent" 2>&1)
  pull_rc=$?
  if [ $pull_rc -ne 0 ]; then
    echo "[$(ts)] $agent ERROR: sirsi router pull exit=$pull_rc -- $(echo "$pull_out" | head -2 | tr '\n' ' ')" >> "$LOG"
    pull_errors=$((pull_errors + 1))
    continue
  fi
  ids=$(echo "$pull_out" | awk '/• /{print $2}')

  # --- Legacy push: state.json pending[<agent>] ---
  legacy_ids=""
  if command -v jq >/dev/null 2>&1; then
    legacy_ids=$(jq -r --arg agent "$agent" '.pending[$agent][]? // empty' "$ROUTER_ROOT/state.json" 2>/dev/null)
  fi

  all_ids=$(printf '%s\n%s\n' "$ids" "$legacy_ids" | grep -v '^$' | sort -u)
  [ -z "$all_ids" ] && continue

  count=$(echo "$all_ids" | wc -l | tr -d ' ')
  echo "[$(ts)] $agent: $count item(s) to dispatch" >> "$LOG"

  prompt="ctr

You are $agent on this workstation.
Read $ROUTER_ROOT/state.json and the open router item(s) addressed to $agent:
$all_ids

Work only those addressed items. Write the required router review/decision/completion artifacts, update state.json, and stop. If a blocker prevents work, write a blocker artifact instead of looping."

  cd "$REPO_ROOT" || continue
  start_agent_worker "$agent" "$prompt"
  start_rc=$?
  [ $start_rc -eq 1 ] && continue
  [ $start_rc -eq 2 ] && continue
  if ! "$SIRSI" router ack "$agent" $all_ids >> "$LOG" 2>&1; then
    echo "[$(ts)] $agent warning: failed to ack legacy pending ids" >> "$LOG"
  fi
  dispatched=$((dispatched + 1))
done

if [ $pull_errors -gt 0 ]; then
  echo "[$(ts)] dispatch.sh exit — agents fired: $dispatched  pull_errors: $pull_errors (see above)" >> "$LOG"
else
  echo "[$(ts)] dispatch.sh exit — agents fired: $dispatched" >> "$LOG"
fi
