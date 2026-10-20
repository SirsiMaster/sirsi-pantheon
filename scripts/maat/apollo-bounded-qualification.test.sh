#!/bin/bash
# Non-executing contract checks for the bounded Apollo qualification wrapper.
set -euo pipefail

root="$(cd "$(dirname "$0")/../.." && pwd -P)"
runner="$root/scripts/maat/apollo-bounded-qualification.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

cat >"$tmp/server" <<'EOF'
#!/bin/sh
exit 0
EOF
chmod 700 "$tmp/server"
cat >"$tmp/sirsi" <<'EOF'
#!/bin/sh
if [ "$1" = gemma ] && [ "$2" = worker ] && [ "$3" = --help ]; then
  printf '%s\n' '--once --apollo-tool-turn --item --inference-profile --allow-tool'
fi
exit 0
EOF
chmod 700 "$tmp/sirsi"
mkdir "$tmp/checkpoint"
cat >"$tmp/profile.json" <<'EOF'
{"id":"gemma-m5-apollo21","endpoint":"http://127.0.0.1:8521/v1","model":"fixture","context_tokens":4096,"output_tokens":1024,"streaming":true,"tool_calls":true,"request_timeout_seconds":600}
EOF

sha() { shasum -a 256 "$1" | awk '{print $1}'; }
args=(--agent codex-pantheon --work apollo-qualification-fixture --item fixture-item
  --sirsi "$tmp/sirsi" --sirsi-sha256 "$(sha "$tmp/sirsi")"
  --profile "$tmp/profile.json" --profile-sha256 "$(sha "$tmp/profile.json")"
  --server "$tmp/server" --server-sha256 "$(sha "$tmp/server")"
  --checkpoint "$tmp/checkpoint" --timeout 900 --validate-only)
fixtures="$root/scripts/maat/testdata/apollo-bounded-qualification"

bash -n "$runner"
"$runner" "${args[@]}" | grep -q '^validated '

# This imports the installed extensionless Ma'at runner under a non-main module
# name and reads only the direct parent identity; it starts no admission,
# listener, server, model, or worker. It is intentionally host-only.
if [[ -x "$HOME/.local/bin/sirsi-maat-run" && ! -L "$HOME/.local/bin/sirsi-maat-run" && "$(/usr/bin/uname -s)" == Darwin ]]; then
  identity_args=(--agent codex-pantheon --work apollo-qualification-fixture --item fixture-item
    --sirsi "$tmp/sirsi" --sirsi-sha256 "$(sha "$tmp/sirsi")"
    --profile "$tmp/profile.json" --profile-sha256 "$(sha "$tmp/profile.json")"
    --server "$tmp/server" --server-sha256 "$(sha "$tmp/server")"
    --checkpoint "$tmp/checkpoint" --timeout 900 --validate-installed-maat-identity)
  "$runner" "${identity_args[@]}" | grep -q '^validated installed-maat identity='
fi

# The structured grant predicate accepts only one fresh local full-share
# admission plus the exact executor host, PID, and kernel-start generation.
grant_args=(--agent codex-pantheon --work apollo-qualification-fixture --item fixture-item
  --sirsi "$tmp/sirsi" --sirsi-sha256 "$(sha "$tmp/sirsi")"
  --profile "$tmp/profile.json" --profile-sha256 "$(sha "$tmp/profile.json")"
  --server "$tmp/server" --server-sha256 "$(sha "$tmp/server")"
  --checkpoint "$tmp/checkpoint" --timeout 900 --validate-grant-fixture)
fresh_fixture() {
  local source="$1" target=""
  target="$tmp/${source##*/}"
  /usr/bin/jq --arg now "$(/bin/date -u +%Y-%m-%dT%H:%M:%SZ)" \
    '.admission[].heartbeat_at = $now' "$source" >"$target"
  printf '%s\n' "$target"
}
SIRSI_THREAD_ID=thr-fixture "$runner" "${grant_args[@]}" "$(fresh_fixture "$fixtures/valid.json")" | grep -q '^validated grant fixture='
for fixture in wrong-resource-same-pid.json wrong-host-same-pid.json wrong-start-time.json; do
  if SIRSI_THREAD_ID=thr-fixture "$runner" "${grant_args[@]}" "$(fresh_fixture "$fixtures/$fixture")" >/dev/null 2>&1; then
    echo "negative control: $fixture was accepted" >&2
    exit 1
  fi
done

# A capability downgrade must refuse before Ma'at or the server can run.
printf '%s\n' '{"id":"gemma-m5-apollo21","endpoint":"http://127.0.0.1:8521/v1","model":"fixture","context_tokens":4096,"output_tokens":1024,"streaming":true,"tool_calls":false,"request_timeout_seconds":600}' >"$tmp/bad-profile.json"
if "$runner" --agent codex-pantheon --work apollo-qualification-fixture --item fixture-item \
  --sirsi "$tmp/sirsi" --sirsi-sha256 "$(sha "$tmp/sirsi")" \
  --profile "$tmp/bad-profile.json" --profile-sha256 "$(sha "$tmp/bad-profile.json")" \
  --server "$tmp/server" --server-sha256 "$(sha "$tmp/server")" \
  --checkpoint "$tmp/checkpoint" --timeout 900 --validate-only >/dev/null 2>&1; then
  echo 'negative control: downgraded tool capability was accepted' >&2
  exit 1
fi

# An installed CLI missing the exact bounded worker flags must refuse before a
# Ma'at reservation, listener, or model launch.
cat >"$tmp/old-sirsi" <<'EOF'
#!/bin/sh
if [ "$1" = gemma ] && [ "$2" = worker ] && [ "$3" = --help ]; then
  printf '%s\n' '--once --item'
fi
exit 0
EOF
chmod 700 "$tmp/old-sirsi"
if "$runner" --agent codex-pantheon --work apollo-qualification-fixture --item fixture-item \
  --sirsi "$tmp/old-sirsi" --sirsi-sha256 "$(sha "$tmp/old-sirsi")" \
  --profile "$tmp/profile.json" --profile-sha256 "$(sha "$tmp/profile.json")" \
  --server "$tmp/server" --server-sha256 "$(sha "$tmp/server")" \
  --checkpoint "$tmp/checkpoint" --timeout 900 --validate-only >/dev/null 2>&1; then
  echo 'negative control: old Pantheon worker CLI was accepted' >&2
  exit 1
fi

# The wrapper must reject the legacy supervision bypass before any payload runs.
if APOLLO_MAAT=off "$runner" "${args[@]}" >/dev/null 2>&1; then
  echo 'negative control: APOLLO_MAAT=off was accepted' >&2
  exit 1
fi

# A changed binary is never admitted under a stale identity hash.
printf '# drift\n' >>"$tmp/server"
if "$runner" "${args[@]}" >/dev/null 2>&1; then
  echo 'negative control: changed server identity was accepted' >&2
  exit 1
fi

# The normal path refuses a missing Ma'at helper before a child can launch.
cp "$tmp/sirsi" "$tmp/server"
if HOME="$tmp/missing-home" "$runner" \
  --agent codex-pantheon --work apollo-qualification-fixture --item fixture-item \
  --sirsi "$tmp/sirsi" --sirsi-sha256 "$(sha "$tmp/sirsi")" \
  --profile "$tmp/profile.json" --profile-sha256 "$(sha "$tmp/profile.json")" \
  --server "$tmp/server" --server-sha256 "$(sha "$tmp/server")" \
  --checkpoint "$tmp/checkpoint" --timeout 900 >/dev/null 2>&1; then
  echo 'negative control: missing Ma at helper was accepted' >&2
  exit 1
fi

# The inner mode cannot be invoked directly outside a registered executor.
attempt="$tmp/attempt"
mkdir "$attempt"
printf '%s\n' nonce >"$attempt/supervision.nonce"
if SIRSI_AGENT_ID=codex-pantheon SIRSI_THREAD_ID=thr-forged "$runner" --supervised-child --attempt "$attempt" --nonce nonce \
  --agent codex-pantheon --work apollo-qualification-fixture --item fixture-item \
  --sirsi "$tmp/sirsi" --sirsi-sha256 "$(sha "$tmp/sirsi")" \
  --profile "$tmp/profile.json" --profile-sha256 "$(sha "$tmp/profile.json")" \
  --server "$tmp/server" --server-sha256 "$(sha "$tmp/server")" \
  --checkpoint "$tmp/checkpoint" --timeout 900 >/dev/null 2>&1; then
  echo 'negative control: direct child invocation was accepted' >&2
  exit 1
fi

grep -q 'sirsi-maat-run is required; no raw fallback exists' "$runner"
grep -q -- '--once --apollo-tool-turn --item' "$runner"
grep -q 'loopback listener' "$runner"
grep -q -- '"$server" --checkpoint "$checkpoint" --idle-exit 120 --addr' "$runner"
! grep -q -- '"$server" "$checkpoint" --idle-exit 120 --addr' "$runner"
grep -q 'no fresh full-share Ma.at admission and executor bind this local parent generation' "$runner"
grep -q 'parent_resource' "$runner"
grep -q 'parent_start' "$runner"
grep -q 'SourceFileLoader' "$runner"
grep -q 'listener remains after owned Apollo child shutdown' "$runner"
echo 'apollo bounded qualification wrapper contract: PASS'
