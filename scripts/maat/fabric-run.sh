#!/bin/bash
# Narrow pinned transport. Proxy mode remains the default; direct is explicit,
# and never permitted to bypass a runtime-provided proxy.
set -euo pipefail
usage() { printf 'usage: sirsi-fabric-run [--plan] [--transport socks|direct] m1|m5|studio --work ID [--cpu-cores N] [--timeout SECONDS] -- COMMAND [ARG...]\n' >&2; exit 64; }
plan=0
transport_mode=socks
while [[ "${1:-}" == --plan || "${1:-}" == --transport ]]; do
  case "$1" in
    --plan) plan=1; shift;;
    --transport) [[ $# -ge 2 ]] || usage; transport_mode=$2; shift 2;;
  esac
done
[[ "$transport_mode" == socks || "$transport_mode" == direct ]] || usage
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
if [[ "$transport_mode" == socks ]]; then
  case "$proxy_url" in socks5://*|socks5h://*) ;; *) printf 'REFUSED: supported runtime SOCKS proxy required\n' >&2; exit 78;; esac
  proxy=${proxy_url#*://}
  [[ "$proxy" =~ ^127\.0\.0\.1:[0-9]+$ ]] || { printf 'REFUSED: proxy must be loopback\n' >&2; exit 78; }
else
  [[ -z "$proxy_url${HTTP_PROXY:-}${http_proxy:-}${HTTPS_PROXY:-}${https_proxy:-}" ]] || { printf 'REFUSED: configured runtime proxy must not be bypassed\n' >&2; exit 78; }
  # Prevent ambient SSH config from silently adding a proxy/jump to an explicit
  # direct transaction. Host authentication and known_hosts stay unchanged.
fi
script_dir=$(CDPATH= cd -- "$(/usr/bin/dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
selector="$script_dir/fabric-endpoints.py"
[[ -f "$selector" && ! -L "$selector" ]] || { printf 'REFUSED: shared endpoint selector absent\n' >&2; exit 78; }
trust=(-oUserKnownHostsFile="$HOME/.ssh/sirsi_known_hosts" -oHostKeyAlias="sirsi-$host" -oHostKeyAlgorithms=ssh-ed25519)
case "$host" in
  m1)
    destination=sirsimasterdev@sirsi-m1
    remote_runner=/Users/sirsimasterdev/.local/bin/sirsi-maat-run;;
  studio)
    destination=sirsimasterdev@sirsi-studio
    remote_runner=/Users/sirsimasterdev/.local/bin/sirsi-maat-run;;
  m5)
    destination=thekryptodragon@sirsi-m5
    remote_runner=/Users/thekryptodragon/.local/bin/sirsi-maat-run
    # M5 is enrolled under its stable logical identity, not its current
    # tailnet address. Keep the address for transport only; strict host-key
    # verification must use the existing named pin and its dedicated store.
    ;;
  *) usage;;
esac
# OpenSSH passes one command through the destination shell. Quote EVERY argv
# element as POSIX literal data, including the actual payload's arguments.
quote() { local value=${1//\'/\'\\\'\'}; printf "'%s'" "$value"; }
remote_command=
for arg in "$remote_runner" --agent "$agent" --register-executor --work "$work" --cpu-cores "$cores" --timeout "$timeout" -- "$@"; do
  remote_command+="$(quote "$arg") "
done
proxy_command="$(quote /usr/bin/python3) $(quote "$selector") connect $(quote "$host") --transport $(quote "$transport_mode")"
transport=(/usr/bin/ssh -oBatchMode=yes -oPasswordAuthentication=no -oKbdInteractiveAuthentication=no -oPreferredAuthentications=publickey
  -oStrictHostKeyChecking=yes -oConnectTimeout=45 -oServerAliveInterval=15 -oServerAliveCountMax=4
  -oProxyJump=none -oControlMaster=no -oControlPath=none -o "ProxyCommand=$proxy_command"
  "${trust[@]}" "$destination" "$remote_command")
origin=("$HOME/.local/bin/sirsi-maat-run" --agent "$agent" --register-executor --work "fabric-ssh-$work" --cpu-cores 1 --timeout "$((timeout+120))" --)
if [[ "$plan" == 1 ]]; then
  printf 'PLAN_ONLY_NO_EXECUTION\n'
  printf 'ENDPOINT_POLICY=thunderbolt,thunderbolt-relay,classified-lan,tailscale; selection only inside admitted transport\n'
  printf 'REMOTE_COMMAND=%s\n' "$remote_command"
  printf '%q ' "${origin[@]}" "${transport[@]}"; printf '\n'
  exit 0
fi
test -x "${origin[0]}" || { printf 'REFUSED: installed local supervisor absent\n' >&2; exit 78; }
exec "${origin[@]}" "${transport[@]}"
