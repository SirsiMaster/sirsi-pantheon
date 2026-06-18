#!/bin/bash
# Wake helper for the pull-model router. Fired by launchd WatchPaths on
# changes to state.json, items/, or proposals/. This script OBSERVES the
# queue and logs activity — it does NOT spawn agents or mutate items.
# That matches Codex's review condition: the plist is a wake helper, not
# the source of truth. Real work happens when an agent next opens an
# interactive session and reads its inbox.
#
# If you want auto-spawn on item arrival, write a sibling script that
# calls `sirsi router pull <id>` and invokes your agent of choice. Keep
# that separate from this observer so a broken spawner can't make the
# queue look broken.

set -euo pipefail

REPO="/Users/thekryptodragon/Development/sirsi-pantheon"
LOG_DIR="${REPO}/.agents/idea-router/logs"
WAKE_LOG="${LOG_DIR}/wake.log"
. "${REPO}/.agents/idea-router/router-env.sh"

WAKE_LOG="$(router_ensure_log "$WAKE_LOG" "/tmp/sirsi-router-wake.log")" || exit 1

{
  echo "=== wake $(date -u +%Y-%m-%dT%H:%M:%SZ) ==="
  cd "${REPO}"
  if SIRSI="$(router_resolve_sirsi "$REPO")"; then
    "$SIRSI" router status --stale 6 2>&1 || true
  else
    echo "  (sirsi binary not found — build ./cmd/sirsi or install sirsi to enable status)"
  fi
} >> "${WAKE_LOG}"
