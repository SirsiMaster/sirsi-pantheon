#!/bin/bash
# Narrow transport for proxy-enabled native Codex workers. No direct fallback.
set -euo pipefail
usage() { printf 'usage: sirsi-fabric-run [--plan] m1|m5|studio --work ID [--cpu-cores N] [--timeout SECONDS] -- COMMAND [ARG...]\n' >&2; exit 64; }
plan=0
if [[ "${1:-}" == --plan ]]; then plan=1; shift; fi
host=${1:-}; [[ -n "$host" ]] || usage; shift
work=; cores=1; timeout=90
while [[ $# -gt 0 && "$1" != -- ]]; do
  [[ $# -ge 2 ]] || usage
  case "$1" in
    --work) work=$2;;
    --cpu-cores) cores=$2;;
    --timeout) timeout=$2;;
    *) usage;;
  esac
  shift 2
done
[[ "${1:-}" == -- ]] || usage; shift
[[ $# -gt 0 && "$work" =~ ^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,127}$ ]] || usage
[[ "$cores" =~ ^[1-9][0-9]?$ && "$timeout" =~ ^[1-9][0-9]{0,4}$ ]] || usage
(( cores <= 64 && timeout <= 86400 )) || usage
agent=${SIRSI_ROUTER_AGENT:-${SIRSI_AGENT_ID:-}}
[[ "$agent" =~ ^[a-z0-9][a-z0-9-]{0,62}$ ]] || { printf 'REFUSED: own logical lane identity missing\n' >&2; exit 78; }
if [[ -n "${SIRSI_AGENT_ID:-}" && "${SIRSI_AGENT_ID}" != "$agent" ]]; then
  printf 'REFUSED: conflicting lane identities\n' >&2; exit 78
fi
proxy_url=${ALL_PROXY:-${all_proxy:-}}
case "$proxy_url" in socks5://*|socks5h://*) ;; *) printf 'REFUSED: supported runtime SOCKS proxy required\n' >&2; exit 78;; esac
proxy=${proxy_url#*://}
[[ "$proxy" =~ ^127\.0\.0\.1:[0-9]+$ ]] || { printf 'REFUSED: proxy must be loopback\n' >&2; exit 78; }
trust=()
case "$host" in
  m1)
    destination=sirsimasterdev@192.168.1.185
    remote_runner=/Users/sirsimasterdev/.local/bin/sirsi-maat-run
    trust=(-oHostKeyAlias=100.88.242.95);;
  studio)
    destination=sirsimasterdev@10.96.127.2
    remote_runner=/Users/sirsimasterdev/.local/bin/sirsi-maat-run
    trust=(-oUserKnownHostsFile="$HOME/.sirsi/apollo/runtime/sdk-9d24c8c9/trust/known_hosts" -oHostKeyAlias=10.96.127.2);;
  m5)
    destination=thekryptodragon@100.92.193.26
    remote_runner=/Users/thekryptodragon/.local/bin/sirsi-maat-run
    trust=(-oHostKeyAlias=100.92.193.26);;
  *) usage;;
esac
# OpenSSH passes one command through the destination shell. Quote EVERY argv
# element as POSIX literal data, including the actual payload's arguments.
quote() { local value=${1//\'/\'\\\'\'}; printf "'%s'" "$value"; }
remote_command=
for arg in "$remote_runner" --agent "$agent" --register-executor --work "$work" --cpu-cores "$cores" --timeout "$timeout" -- "$@"; do
  remote_command+="$(quote "$arg") "
done
transport=(/usr/bin/ssh -oBatchMode=yes -oPasswordAuthentication=no -oKbdInteractiveAuthentication=no -oPreferredAuthentications=publickey
  -oStrictHostKeyChecking=yes -oConnectTimeout=8 -oServerAliveInterval=15 -oServerAliveCountMax=4
  -o "ProxyCommand=/usr/bin/nc -x $proxy -X 5 %h %p" "${trust[@]}" "$destination" "$remote_command")
origin=("$HOME/.local/bin/sirsi-maat-run" --agent "$agent" --register-executor --work "fabric-ssh-$work" --cpu-cores 1 --timeout "$((timeout+120))" --)
if [[ "$plan" == 1 ]]; then
  printf 'PLAN_ONLY_NO_EXECUTION\n'
  printf 'REMOTE_COMMAND=%s\n' "$remote_command"
  printf '%q ' "${origin[@]}" "${transport[@]}"; printf '\n'
  exit 0
fi
test -x "${origin[0]}" || { printf 'REFUSED: installed local supervisor absent\n' >&2; exit 78; }
exec "${origin[@]}" "${transport[@]}"
