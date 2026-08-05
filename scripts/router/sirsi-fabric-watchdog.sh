#!/bin/bash
# sirsi-fabric-watchdog.sh — deterministic, no-LLM fabric supervisor (P1 of
# local-llm-sovereignty, owner directive 2026-07-23). Runs every 5 min via
# launchd. Heals what needs zero judgment: wedged/dead broker, dead core
# daemons. Writes a machine-readable last-run report for the future
# `sirsi report` / menubar surface. Cloud conduit keeps judgment work.
set -u
UID_N=$(id -u)
TS=$(date -u +%Y-%m-%dT%H:%M:%SZ)
ACTIONS=()

# 1. Broker: launchd KeepAlive covers crash-death; this covers WEDGED
#    (process alive but cannot complete) → kickstart -k (kill+restart).
#    Port comes from the canonical port file — a hardcoded 8765 kept killing
#    the healthy post-cutover SNE server every 5 min (found 2026-08-05,
#    42+ needless restarts). Probe = 1-token REAL completion, not /health:
#    a wedged engine can answer trivial GETs while unable to infer. Timeout
#    is generous on purpose — killing a busy-but-healthy server is worse
#    than detecting a wedge one cycle later.
BROKER_PORT=$(tr -d '[:space:]' < "$HOME/.sirsi/gemma-server.port" 2>/dev/null)
BROKER_PORT=${BROKER_PORT:-8477}
if ! curl -s -m 45 "http://127.0.0.1:${BROKER_PORT}/v1/chat/completions" \
      -H 'Content-Type: application/json' \
      -d '{"model":"x","messages":[{"role":"user","content":"ok"}],"max_tokens":1}' \
      | grep -q '"choices"'; then
  launchctl kickstart -k "gui/$UID_N/ai.sirsi.gemma-broker" 2>/dev/null \
    && ACTIONS+=("broker-kickstart") || ACTIONS+=("broker-kickstart-FAILED")
fi

# 2. Core daemons: label present but no live PID → kickstart.
for LABEL in ai.sirsi.horus.agent-router ai.sirsi.triage ai.sirsi.gemma-worker ai.sirsi.pantheon; do
  PID=$(launchctl list | awk -v l="$LABEL" '$3==l{print $1}')
  if [ -z "$PID" ] || [ "$PID" = "-" ] || ! ps -p "$PID" >/dev/null 2>&1; then
    launchctl kickstart -k "gui/$UID_N/$LABEL" 2>/dev/null \
      && ACTIONS+=("$LABEL-kickstart") || ACTIONS+=("$LABEL-kickstart-FAILED")
  fi
done

# 3. Report: one JSON blob for the report surface + append-only log line.
OUTCOME=green; [ ${#ACTIONS[@]} -gt 0 ] && OUTCOME=healed
ACT_JSON=$(printf '"%s",' "${ACTIONS[@]:-}" 2>/dev/null | sed 's/,$//; s/""//')
printf '{"ts":"%s","outcome":"%s","actions":[%s],"source":"fabric-watchdog"}\n' \
  "$TS" "$OUTCOME" "$ACT_JSON" > "$HOME/.sirsi/fabric-watchdog-last.json"
[ "$OUTCOME" = healed ] && echo "$TS healed: ${ACTIONS[*]}" >> "$HOME/.sirsi/fabric-watchdog.log"
exit 0
