#!/usr/bin/env bash
# Synthetic gcloud only: never contact or mutate a real service.
set -euo pipefail
ROOT=$(git rev-parse --show-toplevel)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir "$tmp/bin"
cat >"$tmp/bin/gcloud" <<'MOCK'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$CALLS"
case "$*" in
  'run services list '*) [ "${LIST_ERROR:-0}" = 0 ] || exit 1; printf '%s\n' "$LIST" ;;
  'run services describe sirsi-router '*)
    [ "${DESCRIBE_ERROR:-0}" = 0 ] || exit 1
    if [ "${CHANGE_AFTER_BUILD:-0}" = 1 ] && [ -e "$BUILD_DONE" ]; then printf '%s\n' "$CHANGED_SERVICE"; else printf '%s\n' "$SERVICE"; fi ;;
  *) echo 'unexpected mutating gcloud call' >&2; exit 88 ;;
esac
MOCK
chmod +x "$tmp/bin/gcloud"
export PATH="$tmp/bin:$PATH" CALLS="$tmp/calls"
export LIST='[{"metadata":{"name":"sirsi-router"}}]' SERVICE
target="$ROOT/scripts/router-service/traffic-target.sh"
expect_target() {
  local actual
  actual=$(bash "$target" --project=synthetic --region=synthetic)
  [ "$actual" = "$1" ] || { echo "unexpected target: $actual" >&2; exit 1; }
}
expect_failure() {
  if bash "$target" --project=synthetic --region=synthetic >"$tmp/out" 2>"$tmp/err"; then
    echo 'expected refusal' >&2; exit 1
  fi
}
SERVICE='{"metadata":{"name":"sirsi-router"},"status":{"traffic":[{"tag":"old","revisionName":"old","percent":0},{"revisionName":"sirsi-router-v144-full-70efdaad","percent":100}]}}'
expect_target sirsi-router-v144-full-70efdaad
SERVICE='{"metadata":{"name":"sirsi-router"},"status":{"traffic":[{"tag":"old","revisionName":"old"},{"revisionName":"current","percent":100}]}}'
expect_target current
SERVICE='{"metadata":{"name":"sirsi-router"},"status":{"traffic":[{"revisionName":"current","percent":100}]}}'
expect_target current
SERVICE='{"metadata":{"name":"sirsi-router"},"status":{"traffic":[{"revisionName":"a","percent":50},{"revisionName":"b","percent":50}]}}'
expect_failure
# The actual stage entry point must refuse before build or any mutating call.
if bash "$ROOT/scripts/router-service/stage.sh" >"$tmp/out" 2>"$tmp/err"; then echo 'stage accepted split traffic' >&2; exit 1; fi
grep -q 'cannot identify a single 100% serving revision' "$tmp/err" || { echo 'stage failed for an unexpected reason' >&2; exit 1; }
# Exercise a change during the build with fake git cleanliness and fake go.
# The real git is still used for all other reads; gcloud refuses any write.
export REAL_GIT="$(command -v git)" BUILD_DONE="$tmp/build-done" CHANGE_AFTER_BUILD=1
export CHANGED_SERVICE='{"metadata":{"name":"sirsi-router"},"status":{"traffic":[{"revisionName":"newer","percent":100}]}}'
cat >"$tmp/bin/git" <<'MOCK'
#!/usr/bin/env bash
if [ "${1:-}" = -C ] && [ "${3:-}" = status ]; then exit 0; fi
exec "$REAL_GIT" "$@"
MOCK
cat >"$tmp/bin/go" <<'MOCK'
#!/usr/bin/env bash
touch "$BUILD_DONE"
MOCK
chmod +x "$tmp/bin/git" "$tmp/bin/go"
SERVICE='{"metadata":{"name":"sirsi-router"},"status":{"traffic":[{"revisionName":"current","percent":100}]}}'
if bash "$ROOT/scripts/router-service/stage.sh" >"$tmp/out" 2>"$tmp/err"; then echo 'stage accepted traffic change during build' >&2; exit 1; fi
grep -q 'serving traffic changed during build' "$tmp/err" || { echo 'stage failed before drift check' >&2; exit 1; }
unset CHANGE_AFTER_BUILD
SERVICE='{"metadata":{"name":"sirsi-router"},"status":{"traffic":[]}}'
expect_failure
SERVICE='{"metadata":{"name":"sirsi-router"},"status":{"traffic":[{"revisionName":"a"}]}}'
expect_failure
SERVICE='{"metadata":{"name":"sirsi-router"},"status":{"traffic":[{"percent":100}]}}'
expect_failure
SERVICE='not-json'
expect_failure
SERVICE=''
expect_failure
export DESCRIBE_ERROR=1
expect_failure
unset DESCRIBE_ERROR
export LIST_ERROR=1
expect_failure
unset LIST_ERROR
LIST='not-json'
expect_failure
LIST=''
expect_failure
LIST='[{"metadata":{}}]'
expect_failure
LIST='[{"metadata":{"name":"sirsi-router"}},{"metadata":{"name":"sirsi-router"}}]'
expect_failure
LIST='[]'
expect_target ''
if grep -nE 'run (deploy|services update-traffic)' "$CALLS"; then echo 'unexpected mutation' >&2; exit 1; fi
echo 'PASS: synthetic router traffic selection and stage refusal'
