#!/usr/bin/env bash
# Entire stage entry point with synthetic commands only; never calls real gcloud.
set -euo pipefail
ROOT=$(git rev-parse --show-toplevel)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir "$tmp/bin"
export REAL_GIT="$(command -v git)" CALLS="$tmp/calls" DEPLOYED="$tmp/deployed" PROBED="$tmp/probed"
export PATH="$tmp/bin:$PATH"
cat >"$tmp/bin/git" <<'MOCK'
#!/usr/bin/env bash
if [ "${1:-}" = -C ] && [ "${3:-}" = status ]; then exit 0; fi
exec "$REAL_GIT" "$@"
MOCK
cat >"$tmp/bin/go" <<'MOCK'
#!/usr/bin/env bash
exit 0
MOCK
cat >"$tmp/bin/curl" <<'MOCK'
#!/usr/bin/env bash
printf 'curl %s\n' "$*" >>"$CALLS"
touch "$PROBED"
printf 200
MOCK
cat >"$tmp/bin/gcloud" <<'MOCK'
#!/usr/bin/env bash
printf 'gcloud %s\n' "$*" >>"$CALLS"
case "$*" in
  'run services list '*) printf '%s\n' '[{"metadata":{"name":"sirsi-router"}}]' ;;
  'run services describe sirsi-router '*)
    if [ ! -e "$DEPLOYED" ]; then printf '%s\n' "$BEFORE"
    elif [ ! -e "$PROBED" ]; then printf '%s\n' "$FIRST"
    else [ "${FINAL_ERROR:-0}" = 0 ] || exit 1; printf '%s\n' "$FINAL"; fi ;;
  'run deploy sirsi-router '*) touch "$DEPLOYED" ;;
  'run services update-traffic sirsi-router '*) : ;;
  *) echo 'unexpected gcloud call' >&2; exit 88 ;;
esac
MOCK
chmod +x "$tmp/bin/"*
tag="stg-$("$REAL_GIT" -C "$ROOT" rev-parse --short=8 HEAD)"
export BEFORE='{"metadata":{"name":"sirsi-router"},"status":{"traffic":[{"revisionName":"previous","percent":100}]}}'
export FIRST="{\"metadata\":{\"name\":\"sirsi-router\"},\"status\":{\"latestReadyRevisionName\":\"other-latest\",\"traffic\":[{\"revisionName\":\"previous\",\"percent\":100},{\"tag\":\"$tag\",\"revisionName\":\"staged-revision\",\"percent\":0,\"url\":\"https://staged.example.run.app\"}]}}"
export FINAL="$FIRST"
reset_fixture() { rm -f "$CALLS" "$DEPLOYED" "$PROBED"; }
reset_fixture
bash "$ROOT/scripts/router-service/stage.sh" >"$tmp/out" 2>"$tmp/err"
grep -q '^NEW=staged-revision$' "$tmp/out"
grep -q -- '--to-revisions staged-revision=100' "$CALLS"
! grep -q -- '--to-revisions other-latest=100' "$CALLS"
[ "$(grep -c '^curl ' "$CALLS")" = 3 ]
grep -q 'https://staged.example.run.app/v1/healthz' "$CALLS"
refuse() {
  reset_fixture
  if bash "$ROOT/scripts/router-service/stage.sh" >"$tmp/out" 2>"$tmp/err"; then echo 'expected stage refusal' >&2; exit 1; fi
  grep -q 'refusing traffic shift' "$tmp/err" || { echo 'unexpected stage failure' >&2; exit 1; }
  if grep -q 'run services update-traffic' "$CALLS"; then echo 'traffic shift after unsafe readback' >&2; exit 1; fi
}
FINAL=${FIRST//staged-revision/changed-revision}; export FINAL; refuse
FINAL=${FIRST//staged.example.run.app/changed.example.run.app}; export FINAL; refuse
FINAL=${FIRST//previous/other-serving}; export FINAL; refuse
FINAL='{"metadata":{"name":"sirsi-router"},"status":{"traffic":[]}}'; export FINAL; refuse
FINAL="$FIRST"; export FINAL FINAL_ERROR=1; refuse; unset FINAL_ERROR
original_first="$FIRST"
FIRST=${original_first//staged-revision/previous}; export FIRST; refuse
FIRST=${original_first//https:\/\/staged.example.run.app/http:\/\/staged.example.run.app}; export FIRST; refuse
FIRST="{\"metadata\":{\"name\":\"sirsi-router\"},\"status\":{\"traffic\":[{\"revisionName\":\"previous\",\"percent\":100},{\"tag\":\"$tag\",\"revisionName\":\"staged-revision\",\"url\":\"https://staged.example.run.app\"},{\"tag\":\"$tag\",\"revisionName\":\"other\",\"url\":\"https://other.example.run.app\"}]}}"; export FIRST; refuse
echo 'PASS: staged tag binds probe and shift; changed target refused'
