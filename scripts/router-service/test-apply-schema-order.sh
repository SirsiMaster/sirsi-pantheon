#!/usr/bin/env bash
# Hermetic fresh-project check for apply-schema-job.sh: a stub `gcloud` on PATH answers "not found" to every
# describe (nothing exists yet) and records every call; the SQL secret must be CREATED before any grant on it,
# and no real gcloud is ever reached. Exit 0 = ordering holds.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd); stub=$(mktemp -d); trap 'rm -rf "$stub"' EXIT
cat >"$stub/gcloud" <<'SH'
#!/usr/bin/env bash
echo "$*" >>"$STUB_LOG"
case "$*" in *" describe "*) exit 1;; esac   # fresh project: nothing exists
exit 0
SH
chmod +x "$stub/gcloud"; export STUB_LOG="$stub/calls.log"; : >"$STUB_LOG"
PATH="$stub:$PATH" bash "$here/apply-schema-job.sh" >/dev/null 2>&1
create=$(grep -n 'secrets create sirsi-router-schema-sql' "$STUB_LOG" | head -1 | cut -d: -f1)
grant=$(grep -n 'secrets add-iam-policy-binding sirsi-router-schema-sql' "$STUB_LOG" | head -1 | cut -d: -f1)
[ -n "$create" ] && [ -n "$grant" ] && [ "$create" -lt "$grant" ] || { echo "FAIL: create=$create grant=$grant"; cat "$STUB_LOG"; exit 1; }
echo "OK: sirsi-router-schema-sql created (call $create) before its grant (call $grant); $(wc -l <"$STUB_LOG") stub calls, no real gcloud"
