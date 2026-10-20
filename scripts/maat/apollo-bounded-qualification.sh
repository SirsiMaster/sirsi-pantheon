#!/bin/bash
# One admitted Apollo observe turn. The normal path always delegates to the
# installed Ma'at supervisor; there is no raw-server fallback.
set -euo pipefail

readonly SELF="$0"
readonly MAAT_RUN="$HOME/.local/bin/sirsi-maat-run"
readonly MAAT_CLI="$HOME/.local/bin/sirsi"
readonly ENDPOINT="http://127.0.0.1:8521/v1"
readonly PORT="8521"

die() { printf '%s\n' "apollo qualification: $*" >&2; exit 78; }
sha256() { /usr/bin/shasum -a 256 "$1" | /usr/bin/awk '{print $1}'; }
regular_file() { [[ -f "$1" && ! -L "$1" ]]; }
regular_executable() { regular_file "$1" && [[ -x "$1" ]]; }
valid_id() { [[ "$1" =~ ^[A-Za-z0-9_.-]+$ && ${#1} -ge 1 && ${#1} -le "$2" ]]; }
valid_sha256() { [[ "$1" =~ ^[[:xdigit:]]+$ && ${#1} -eq 64 ]]; }
usage() {
  cat <<'USAGE'
Usage: apollo-bounded-qualification.sh --agent ID --work ID --item ID \
  --sirsi PATH --sirsi-sha256 HASH --profile PATH --profile-sha256 HASH \
  --server PATH --server-sha256 HASH --checkpoint DIR [--timeout SECONDS] [--validate-only]

The normal path requires installed sirsi-maat-run. --validate-only checks
identity and profile capability without starting Ma'at, Apollo, a listener, or a model request.
USAGE
}

agent=""; work=""; item=""; sirsi=""; sirsi_hash=""; profile=""; profile_hash=""
server=""; server_hash=""; checkpoint=""; timeout=900; validate_only=false
supervised=false; attempt=""; nonce=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --agent) agent="${2:-}"; shift 2 ;;
    --work) work="${2:-}"; shift 2 ;;
    --item) item="${2:-}"; shift 2 ;;
    --sirsi) sirsi="${2:-}"; shift 2 ;;
    --sirsi-sha256) sirsi_hash="${2:-}"; shift 2 ;;
    --profile) profile="${2:-}"; shift 2 ;;
    --profile-sha256) profile_hash="${2:-}"; shift 2 ;;
    --server) server="${2:-}"; shift 2 ;;
    --server-sha256) server_hash="${2:-}"; shift 2 ;;
    --checkpoint) checkpoint="${2:-}"; shift 2 ;;
    --timeout) timeout="${2:-}"; shift 2 ;;
    --validate-only) validate_only=true; shift ;;
    --supervised-child) supervised=true; shift ;;
    --attempt) attempt="${2:-}"; shift 2 ;;
    --nonce) nonce="${2:-}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown argument $1" ;;
  esac
done

valid_id "$agent" 128 || die "agent must be a simple declared lane id"
valid_id "$work" 128 || die "work must be a simple identity"
valid_id "$item" 256 || die "item must be an exact router item id"
[[ "$timeout" =~ ^[0-9]+$ ]] && (( timeout >= 180 && timeout <= 7200 )) || die "timeout must be 180..7200 seconds"
valid_sha256 "$sirsi_hash" || die "sirsi SHA-256 must be exact"
valid_sha256 "$profile_hash" || die "profile SHA-256 must be exact"
valid_sha256 "$server_hash" || die "server SHA-256 must be exact"
regular_executable "$sirsi" || die "Pantheon candidate is not an executable regular file"
regular_file "$profile" || die "Apollo profile is not a regular file"
regular_executable "$server" || die "Apollo server is not an executable regular file"
[[ -d "$checkpoint" && ! -L "$checkpoint" ]] || die "checkpoint must be a real reviewed directory"
[[ "$(sha256 "$sirsi")" == "$sirsi_hash" ]] || die "Pantheon candidate identity changed"
[[ "$(sha256 "$profile")" == "$profile_hash" ]] || die "Apollo profile identity changed"
[[ "$(sha256 "$server")" == "$server_hash" ]] || die "Apollo server identity changed"
[[ "${APOLLO_MAAT:-}" != "off" ]] || die "APOLLO_MAAT=off is never permitted"

# Selection is a local capability check; do not print or read token contents.
/usr/bin/jq -e --arg endpoint "$ENDPOINT" \
  '.id == "gemma-m5-apollo21" and .endpoint == $endpoint and .tool_calls == true and .streaming == true and
   (.context_tokens | type == "number" and . > 0) and (.output_tokens | type == "number" and . > 0) and
   (.request_timeout_seconds | type == "number" and . >= 1 and . <= 600)' \
  "$profile" >/dev/null || die "Apollo profile lacks the exact bounded-tool capability contract"

if [[ "$validate_only" == true ]]; then
  [[ "$supervised" == false ]] || die "validate-only is an outer preflight only"
  printf 'validated server=%s profile=%s item=%s\n' "$server_hash" "$profile_hash" "$item"
  exit 0
fi

if [[ "$supervised" == false ]]; then
  [[ -x "$MAAT_RUN" && ! -L "$MAAT_RUN" ]] || die "installed sirsi-maat-run is required; no raw fallback exists"
  umask 077
  attempt="$(/usr/bin/mktemp -d "${TMPDIR:-/private/tmp}/pantheon-apollo-qualification.XXXXXXXX")" || die "cannot create unique attempt directory"
  nonce="$(/usr/bin/uuidgen | /usr/bin/tr '[:upper:]' '[:lower:]')"
  printf '%s\n' "$nonce" >"$attempt/supervision.nonce"
  /bin/chmod 600 "$attempt/supervision.nonce"
  exec "$MAAT_RUN" --agent "$agent" --register-executor --work "$work" --timeout "$timeout" -- \
    "$SELF" --supervised-child --attempt "$attempt" --nonce "$nonce" \
    --agent "$agent" --work "$work" --item "$item" --sirsi "$sirsi" --sirsi-sha256 "$sirsi_hash" \
    --profile "$profile" --profile-sha256 "$profile_hash" --server "$server" --server-sha256 "$server_hash" \
    --checkpoint "$checkpoint" --timeout "$timeout"
fi

# Only a registered Ma'at child is permitted to start the native server.
[[ -d "$attempt" && ! -L "$attempt" ]] || die "supervised attempt directory is unavailable"
[[ -f "$attempt/supervision.nonce" && ! -L "$attempt/supervision.nonce" ]] || die "supervision nonce missing"
[[ "$(/bin/cat "$attempt/supervision.nonce")" == "$nonce" ]] || die "supervision nonce mismatch"
[[ "${SIRSI_AGENT_ID:-}" == "$agent" ]] && valid_id "${SIRSI_THREAD_ID:-}" 256 && [[ "${SIRSI_THREAD_ID:-}" == thr-* ]] || die "Ma'at executor identity is not present"

# A nonce and inherited names are not an admission. The direct parent must be
# both the active full-share Ma'at exemption and the exact native executor
# bound to this child thread. These are fresh readbacks; no caller-supplied
# environment or stale record can satisfy them.
[[ -x "$MAAT_CLI" ]] || die "installed Ma'at CLI is unavailable"
parent_pid="$PPID"
admission_json="$("$MAAT_CLI" maat status --json)" || die "cannot read current Ma'at admission"
printf '%s' "$admission_json" | /usr/bin/jq -e \
  --arg agent "$agent" --arg work "$work" --argjson parent "$parent_pid" \
  'map(. as $row | select(
      $row.status == "active" and $row.holder == $agent and $row.work == $work and
      $row.share == "full" and $row.exempt_pid == $parent and
      (($row.lease_ttl_sec | type) == "number" and $row.lease_ttl_sec > 0) and
      ($row.heartbeat_at | type == "string") and
      (($row.heartbeat_at | fromdateiso8601) as $heartbeat |
       ($row.lease_ttl_sec as $ttl | (now - $heartbeat) >= 0 and (now - $heartbeat) < $ttl))
    )) | length == 1' \
  >/dev/null || die "no fresh full-share Ma'at admission binds this direct parent"
executor_json="$("$MAAT_CLI" thread heartbeat --thread "$SIRSI_THREAD_ID" --json)" || die "cannot read native executor"
printf '%s' "$executor_json" | /usr/bin/jq -e \
  --arg agent "$agent" --arg thread "$SIRSI_THREAD_ID" --argjson parent "$parent_pid" \
  '.agent_id == $agent and .thread_id == $thread and .status == "active" and .pid == $parent' \
  >/dev/null || die "native executor does not bind this direct parent generation"

terminal="$attempt/terminal.tsv"
log="$attempt/apollo-batch.log"
printf 'phase\tstarted\nserver_sha256\t%s\nprofile_sha256\t%s\nsirsi_sha256\t%s\n' \
  "$server_hash" "$profile_hash" "$sirsi_hash" >"$terminal"
listener_pids() { /usr/sbin/lsof -nP -tiTCP:"$PORT" -sTCP:LISTEN 2>/dev/null || true; }
existing="$(listener_pids)"
[[ -z "$existing" ]] || die "loopback listener $PORT is already occupied; refusing without signal"

"$server" --checkpoint "$checkpoint" --idle-exit 120 --addr 127.0.0.1:"$PORT" >"$log" 2>&1 &
server_pid=$!
printf 'server_pid\t%s\n' "$server_pid" >>"$terminal"
ready=false
for ((i = 0; i < 240; i++)); do
  if ! /bin/kill -0 "$server_pid" 2>/dev/null; then
    wait "$server_pid" || true
    die "Apollo child exited before readiness"
  fi
  owners="$(listener_pids)"
  if [[ "$owners" == "$server_pid" ]]; then
    child_exec="$(/usr/sbin/lsof -a -p "$server_pid" -d txt -Fn 2>/dev/null | /usr/bin/sed -n 's/^n//p' | /usr/bin/head -1)"
    [[ "$child_exec" == "$server" ]] || die "listener owner does not retain the exact Apollo server executable"
    ready=true
    break
  fi
  /bin/sleep 0.5
done
[[ "$ready" == true ]] || die "Apollo child never acquired its exclusive loopback listener"
printf 'phase\tlistener-owned\nlistener_pid\t%s\n' "$server_pid" >>"$terminal"

# Exact selection: no oldest-item fallback and one observe tool only.
set +e
"$sirsi" gemma worker --once --apollo-tool-turn --item "$item" \
  --inference-profile "$profile" --allow-tool process.census
worker_rc=$?
set -e
printf 'worker_exit\t%s\n' "$worker_rc" >>"$terminal"

# Apollo exits naturally after idle-exit. Ma'at alone terminates on revocation.
set +e
wait "$server_pid"
server_rc=$?
set -e
printf 'server_exit\t%s\nphase\tcomplete\n' "$server_rc" >>"$terminal"
! /bin/kill -0 "$server_pid" 2>/dev/null || die "owned Apollo child remains after natural shutdown"
remaining="$(listener_pids)"
[[ -z "$remaining" ]] || die "listener remains after owned Apollo child shutdown"
[[ "$worker_rc" -eq 0 && "$server_rc" -eq 0 ]] || die "worker or admitted Apollo child failed; retained terminal is $terminal"
printf 'Apollo qualification terminal: %s\n' "$terminal"
