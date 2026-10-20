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
valid_host() { [[ "$1" =~ ^[A-Za-z0-9_.-]+$ && ${#1} -ge 1 && ${#1} -le 255 ]]; }

# Use the installed supervisor's own Darwin kernel probe and resource resolver.
# This avoids inventing a parallel hostname→resource map in the wrapper. Cloud
# executor identities are deliberately outside this local, loopback-only run.
maat_parent_identity() {
  [[ -x "$MAAT_RUN" && ! -L "$MAAT_RUN" ]] || die "installed sirsi-maat-run is required for parent identity"
  [[ -z "${SIRSI_ROUTER_CLOUD_HOST:-}" && -z "${SIRSI_ROUTER_USE_MACHINE_ID_HOST:-}" ]] || \
    die "cloud executor identity is not permitted for local Apollo qualification"
  /usr/bin/python3 - "$MAAT_RUN" "$1" <<'PY'
import importlib.util
from importlib.machinery import SourceFileLoader
import json
import socket
import sys

path, raw_pid = sys.argv[1:]
# The installed sirsi-maat-run is deliberately an extensionless executable.
# SourceFileLoader is explicit about that source format; spec_from_file_location
# otherwise returns None and would fail before an admitted child can launch.
loader = SourceFileLoader("installed_maat_run", path)
spec = importlib.util.spec_from_loader("installed_maat_run", loader)
if spec is None or spec.loader is None:
    raise SystemExit("cannot load installed Ma'at runner")
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
pid = int(raw_pid)
_, start = module.process_identity(pid)
resource = module.local_resource()
host = socket.gethostname()
if not isinstance(resource, str) or not resource or not isinstance(host, str) or not host:
    raise SystemExit("installed Ma'at identity is incomplete")
print(json.dumps({"resource": resource, "host": host, "pid": pid, "start_time": start}, sort_keys=True))
PY
}

# The admission and executor are separate authorities. Both must bind the same
# direct supervisor generation before this child can create a server or listener.
validate_grant_json() {
  local admission_json="$1" executor_json="$2" resource="$3" host="$4" parent="$5" parent_start="$6"
  printf '%s' "$admission_json" | /usr/bin/jq -e \
    --arg agent "$agent" --arg work "$work" --arg resource "$resource" --argjson parent "$parent" \
    '(type == "array") and
     ([ .[] | select(
       .status == "active" and .holder == $agent and .work == $work and .resource == $resource and
       .share == "full" and .exempt_pid == $parent and
       ((.lease_ttl_sec | type) == "number" and .lease_ttl_sec > 0) and
       (.heartbeat_at | type == "string") and
       ((.heartbeat_at | fromdateiso8601) as $heartbeat |
        (.lease_ttl_sec as $ttl | (now - $heartbeat) >= 0 and (now - $heartbeat) < $ttl))
     ) ] | length == 1)' \
    >/dev/null || return 1
  printf '%s' "$executor_json" | /usr/bin/jq -e \
    --arg agent "$agent" --arg thread "$SIRSI_THREAD_ID" --arg host "$host" --arg start "$parent_start" --argjson parent "$parent" \
    '.agent_id == $agent and .thread_id == $thread and .status == "active" and .pid == $parent and
     .host == $host and .start_time == $start' \
    >/dev/null
}

validate_grant_fixture() {
  local fixture="$1" expected admission_json executor_json resource host parent parent_start
  regular_file "$fixture" || die "grant fixture is not a regular file"
  expected="$(/usr/bin/jq -ce '.expected | objects' "$fixture")" || die "grant fixture expected identity is malformed"
  admission_json="$(/usr/bin/jq -ce '.admission' "$fixture")" || die "grant fixture admission is malformed"
  executor_json="$(/usr/bin/jq -ce '.executor | objects' "$fixture")" || die "grant fixture executor is malformed"
  resource="$(printf '%s' "$expected" | /usr/bin/jq -er '.resource | strings')" || die "grant fixture resource is malformed"
  host="$(printf '%s' "$expected" | /usr/bin/jq -er '.host | strings')" || die "grant fixture host is malformed"
  parent="$(printf '%s' "$expected" | /usr/bin/jq -er '.pid | numbers')" || die "grant fixture parent pid is malformed"
  parent_start="$(printf '%s' "$expected" | /usr/bin/jq -er '.start_time | strings')" || die "grant fixture parent start is malformed"
  valid_host "$host" && [[ "$parent" =~ ^[0-9]+$ && "$parent" -gt 1 && -n "$parent_start" ]] || die "grant fixture expected identity is invalid"
  validate_grant_json "$admission_json" "$executor_json" "$resource" "$host" "$parent" "$parent_start" || \
    die "grant fixture does not bind resource, host, parent pid, and start generation"
}
usage() {
  cat <<'USAGE'
Usage: apollo-bounded-qualification.sh --agent ID --work ID --item ID \
  --sirsi PATH --sirsi-sha256 HASH --profile PATH --profile-sha256 HASH \
  --server PATH --server-sha256 HASH --checkpoint DIR [--timeout SECONDS] [--validate-only]

The normal path requires installed sirsi-maat-run. --validate-only checks
identity and profile capability without starting Ma'at, Apollo, a listener, or a model request.
--validate-grant-fixture validates a non-executing admission/executor fixture.
USAGE
}

agent=""; work=""; item=""; sirsi=""; sirsi_hash=""; profile=""; profile_hash=""
server=""; server_hash=""; checkpoint=""; timeout=900; validate_only=false
supervised=false; attempt=""; nonce=""
grant_fixture=""; validate_installed_identity=false
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
    --validate-grant-fixture) grant_fixture="${2:-}"; shift 2 ;;
    --validate-installed-maat-identity) validate_installed_identity=true; shift ;;
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

if [[ -n "$grant_fixture" ]]; then
  [[ "$supervised" == false && "$validate_only" == false && "$validate_installed_identity" == false ]] || die "grant fixture is an outer non-executing check only"
  validate_grant_fixture "$grant_fixture"
  printf 'validated grant fixture=%s\n' "$grant_fixture"
  exit 0
fi

if [[ "$validate_installed_identity" == true ]]; then
  [[ "$supervised" == false && "$validate_only" == false ]] || die "installed Ma'at identity preflight is outer and non-executing only"
  installed_identity="$(maat_parent_identity "$PPID")" || die "installed Ma'at identity preflight failed"
  printf '%s' "$installed_identity" | /usr/bin/jq -e --argjson parent "$PPID" \
    '(.resource | type) == "string" and (.resource | length) > 0' >/dev/null &&
    printf '%s' "$installed_identity" | /usr/bin/jq -e --argjson parent "$PPID" \
    '((.host | type) == "string" and (.host | length) > 0) and .pid == $parent and
     ((.start_time | type) == "string" and (.start_time | length) > 0)' >/dev/null || \
    die "installed Ma'at identity preflight is malformed"
  printf 'validated installed-maat identity=%s\n' "$installed_identity"
  exit 0
fi

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
parent_identity="$(maat_parent_identity "$parent_pid")" || die "cannot derive installed Ma'at parent identity"
parent_resource="$(printf '%s' "$parent_identity" | /usr/bin/jq -er '.resource | strings')" || die "installed Ma'at resource is malformed"
parent_host="$(printf '%s' "$parent_identity" | /usr/bin/jq -er '.host | strings')" || die "installed Ma'at host is malformed"
parent_start="$(printf '%s' "$parent_identity" | /usr/bin/jq -er '.start_time | strings')" || die "installed Ma'at parent start is malformed"
valid_host "$parent_host" && [[ -n "$parent_resource" && -n "$parent_start" ]] || die "installed Ma'at identity is incomplete"
admission_json="$("$MAAT_CLI" maat status --json)" || die "cannot read current Ma'at admission"
executor_json="$("$MAAT_CLI" thread heartbeat --thread "$SIRSI_THREAD_ID" --json)" || die "cannot read native executor"
validate_grant_json "$admission_json" "$executor_json" "$parent_resource" "$parent_host" "$parent_pid" "$parent_start" || \
  die "no fresh full-share Ma'at admission and executor bind this local parent generation"

terminal="$attempt/terminal.tsv"
log="$attempt/apollo-batch.log"
printf 'phase\tstarted\nserver_sha256\t%s\nprofile_sha256\t%s\nsirsi_sha256\t%s\n' \
  "$server_hash" "$profile_hash" "$sirsi_hash" >"$terminal"
listener_pids() { /usr/sbin/lsof -nP -tiTCP:"$PORT" -sTCP:LISTEN 2>/dev/null || true; }
existing="$(listener_pids)"
[[ -z "$existing" ]] || die "loopback listener $PORT is already occupied; refusing without signal"

# The reviewed native server owns the listener directly and requires its
# checkpoint flag. Shell/batch launchers are not interchangeable here because
# they can fork a different child and weaken listener/image identity binding.
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
