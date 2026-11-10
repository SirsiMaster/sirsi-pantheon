#!/bin/zsh
# Canonical ai.sirsi.host-readiness-watch observer.
# Observation only: never relaunches Tailscale, unlocks Aqua, or restarts a service.
set -u

resources="${SIRSI_PANTHEON_RESOURCES:-${0:A:h}/..}"
monitor="$resources/unattended-availability-monitor.py"
host="${SIRSI_READINESS_HOST:-127.0.0.1}"
ssh_user="${SIRSI_READINESS_SSH_USER:-}"
state_root="${SIRSI_READINESS_RECEIPT_ROOT:-$HOME/Library/Application Support/Sirsi/HostReadiness}"
run_id="$(/bin/date -u +%Y%m%dT%H%M%SZ)-$$"
receipt_root="$state_root/receipts/$run_id"
current="$state_root/current.json"

# current.json is the latest observation (what readers use); receipts/ keeps only state changes.
args=("$monitor" --host "$host" --attempts 3 --interval 1 --timeout 3 --output "$receipt_root" --current "$current" --history changes)
if [[ -n "$ssh_user" ]]; then
  args+=(--ssh-user "$ssh_user")
fi

exec /usr/bin/python3 "${args[@]}"
