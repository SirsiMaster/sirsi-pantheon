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
cp "$tmp/server" "$tmp/sirsi"
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

bash -n "$runner"
"$runner" "${args[@]}" | grep -q '^validated '

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
if SIRSI_MAAT_RUN="$tmp/no-such-maat-run" "$runner" \
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
if "$runner" --supervised-child --attempt "$attempt" --nonce nonce \
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
echo 'apollo bounded qualification wrapper contract: PASS'
