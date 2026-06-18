#!/usr/bin/env bash
# Shared shell helpers for router launchd/watch scripts.

router_resolve_sirsi() {
  repo="$1"
  for p in "$repo/bin/sirsi" "$repo/sirsi" "$HOME/.local/bin/sirsi" "/opt/homebrew/bin/sirsi" "/usr/local/bin/sirsi"; do
    [ -x "$p" ] && { printf '%s\n' "$p"; return 0; }
  done
  command -v sirsi 2>/dev/null || return 1
}

router_ensure_log() {
  log_path="$1"
  fallback="$2"
  if ! (mkdir -p "$(dirname "$log_path")" && : >> "$log_path") 2>/dev/null; then
    log_path="$fallback"
    mkdir -p "$(dirname "$log_path")" || return 1
    : >> "$log_path" || return 1
  fi
  printf '%s\n' "$log_path"
}
