#!/usr/bin/env bash
# Entire stage entry point with synthetic commands only; never calls real gcloud.
set -euo pipefail
ROOT=$(git rev-parse --show-toplevel)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir "$tmp/bin"
export REAL_GIT="$(command -v git)" CALLS="$tmp/calls" DEPLOYED="$tmp/deployed" PROBED="$tmp/probed" TAG_FILE="$tmp/tag" REV_FILE="$tmp/revision"
export PATH="$tmp/bin:$PATH"
cat >"$tmp/bin/git" <<'MOCK'
#!/usr/bin/env bash
if [ "${1:-}" = -C ] && [ "${3:-}" = status ]; then exit 0; fi
exec "$REAL_GIT" "$@"
MOCK
cat >"$tmp/bin/go" <<'MOCK'
#!/usr/bin/env bash
printf 'go %s\n' "$*" >>"$CALLS"
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
  'run services list '*) if [ "${SERVICE_ABSENT:-0}" = 1 ]; then printf '%s\n' '[]'; else printf '%s\n' '[{"metadata":{"name":"sirsi-router"}}]'; fi ;;
  'run services describe sirsi-router '*)
    if [ ! -e "$DEPLOYED" ]; then printf '%s\n' "$BEFORE"
    else
      if [ ! -e "$PROBED" ]; then value="$FIRST"; else [ "${FINAL_ERROR:-0}" = 0 ] || exit 1; value="$FINAL"; fi
      tag=$(<"$TAG_FILE"); revision=$(<"$REV_FILE")
      value=${value//__tag__/$tag}; value=${value//__revision__/$revision}
      printf '%s\n' "$value"
    fi ;;
  'run deploy sirsi-router '*)
    while [ "$#" -gt 0 ]; do
      case "$1" in --tag) printf '%s' "$2" >"$TAG_FILE"; shift ;; --revision-suffix) printf 'sirsi-router-%s' "$2" >"$REV_FILE"; shift ;; esac
      shift
    done
    tag=$(<"$TAG_FILE"); service=sirsi-router
    [ $((${#tag} + ${#service})) -le 46 ] || { echo 'traffic tag and service name exceed Cloud Run limit' >&2; exit 1; }
    touch "$DEPLOYED" ;;
  'run services update-traffic sirsi-router '*) : ;;
  *) echo 'unexpected gcloud call' >&2; exit 88 ;;
esac
MOCK
chmod +x "$tmp/bin/"*
export BEFORE='{"metadata":{"name":"sirsi-router"},"status":{"traffic":[{"revisionName":"previous","percent":100}]}}'
export FIRST='{"metadata":{"name":"sirsi-router"},"status":{"latestReadyRevisionName":"other-latest","traffic":[{"revisionName":"previous","percent":100},{"tag":"__tag__","revisionName":"__revision__","percent":0,"url":"https://staged.example.run.app"}]}}'
export FINAL="$FIRST"
reset_fixture() { rm -f "$CALLS" "$DEPLOYED" "$PROBED"; }
reset_fixture
bash "$ROOT/scripts/router-service/stage.sh" >"$tmp/out" 2>"$tmp/err"
expected=$(<"$REV_FILE")
grep -q "^NEW=$expected$" "$tmp/out"
grep -q -- "--to-revisions $expected=100" "$CALLS"
grep -q -- '--remove-tags ' "$CALLS"
! grep -q -- '--to-revisions other-latest=100' "$CALLS"
[ "$(grep -c '^curl ' "$CALLS")" = 3 ]
grep -q 'https://staged.example.run.app/v1/healthz' "$CALLS"
grep -q '^RACE=.*single publisher required$' "$tmp/out"
refuse() {
  reset_fixture
  if bash "$ROOT/scripts/router-service/stage.sh" >"$tmp/out" 2>"$tmp/err"; then echo 'expected stage refusal' >&2; exit 1; fi
  grep -q 'refusing traffic shift' "$tmp/err" || { echo 'unexpected stage failure' >&2; exit 1; }
  if grep -q 'run services update-traffic' "$CALLS"; then echo 'traffic shift after unsafe readback' >&2; exit 1; fi
}
FINAL=${FIRST//__revision__/changed-revision}; export FINAL; refuse
FINAL=${FIRST//staged.example.run.app/changed.example.run.app}; export FINAL; refuse
FINAL=${FIRST//previous/other-serving}; export FINAL; refuse
FINAL='{"metadata":{"name":"sirsi-router"},"status":{"traffic":[]}}'; export FINAL; refuse
FINAL="$FIRST"; export FINAL FINAL_ERROR=1; refuse; unset FINAL_ERROR
original_first="$FIRST"
# Retarget before the first tag read: fail before probing or traffic shift.
FIRST=${original_first//__revision__/changed-revision}; export FIRST; refuse
if grep -q '^curl ' "$CALLS"; then echo 'probed a tag retargeted before first read' >&2; exit 1; fi
[ "$(<"$REV_FILE")" != "$expected" ] || { echo 'revision nonce was reused' >&2; exit 1; }
FIRST=${original_first//__revision__/previous}; export FIRST; refuse
FIRST=${original_first//https:\/\/staged.example.run.app/http:\/\/staged.example.run.app}; export FIRST; refuse
FIRST='{"metadata":{"name":"sirsi-router"},"status":{"traffic":[{"revisionName":"previous","percent":100},{"tag":"__tag__","revisionName":"__revision__","url":"https://staged.example.run.app"},{"tag":"__tag__","revisionName":"other","url":"https://other.example.run.app"}]}}'; export FIRST; refuse

# Stage-only retains the verified tagged revision at zero traffic and emits the
# identity tuple needed for a separately owned promotion decision.
FIRST="$original_first"
FINAL="$FIRST"
export FIRST FINAL
reset_fixture
bash "$ROOT/scripts/router-service/stage.sh" --stage-only >"$tmp/out" 2>"$tmp/err"
stage_only_revision=$(<"$REV_FILE")
stage_only_tag=$(<"$TAG_FILE")
[[ "$stage_only_tag" =~ ^s-[0-9a-f]{24}$ ]] || { echo 'stage-only tag lost its 96-bit nonce' >&2; exit 1; }
grep -q "^PREV=previous$" "$tmp/out"
grep -q "^NEW=$stage_only_revision$" "$tmp/out"
grep -q "^TAG=$stage_only_tag$" "$tmp/out"
grep -q '^TURL=https://staged.example.run.app$' "$tmp/out"
grep -q '^PHASE=zero-traffic$' "$tmp/out"
! grep -q 'run services update-traffic' "$CALLS" || { echo 'stage-only changed traffic or removed its tag' >&2; exit 1; }
[ "$(grep -c '^curl ' "$CALLS")" = 3 ] || { echo 'stage-only skipped health probes' >&2; exit 1; }

# Caller-supplied traffic, tag, and revision identity overrides are rejected
# before any service read, build, or deploy in stage-only mode.
for conflicting_flag in --no-traffic=false --traffic=100 --to-latest --tag=caller-tag --revision-suffix=caller-suffix; do
  reset_fixture
  if bash "$ROOT/scripts/router-service/stage.sh" --stage-only "$conflicting_flag" >"$tmp/out" 2>"$tmp/err"; then
    echo "expected stage-only refusal for $conflicting_flag" >&2; exit 1
  fi
  grep -q 'reserved in --stage-only mode' "$tmp/err"
  [ ! -e "$CALLS" ] || { echo "performed work before rejecting $conflicting_flag" >&2; exit 1; }
done
reset_fixture
if bash "$ROOT/scripts/router-service/stage.sh" --stage-only --tag caller-tag >"$tmp/out" 2>"$tmp/err"; then
  echo 'expected stage-only refusal for split --tag argument' >&2; exit 1
fi
grep -q 'reserved in --stage-only mode' "$tmp/err"
[ ! -e "$CALLS" ] || { echo 'performed work before rejecting split --tag argument' >&2; exit 1; }

# First-service stage-only refusal occurs after listing, before build/deploy.
reset_fixture
SERVICE_ABSENT=1; export SERVICE_ABSENT
if bash "$ROOT/scripts/router-service/stage.sh" --stage-only >"$tmp/out" 2>"$tmp/err"; then
  echo 'expected first-service stage-only refusal' >&2; exit 1
fi
grep -q 'requires an existing service' "$tmp/err"
! grep -q '^go ' "$CALLS" || { echo 'built before refusing first-service stage-only' >&2; exit 1; }
! grep -q 'run deploy sirsi-router' "$CALLS" || { echo 'deployed first service in stage-only mode' >&2; exit 1; }
! grep -q '^curl ' "$CALLS" || { echo 'probed a nonexistent first-service tag' >&2; exit 1; }
unset SERVICE_ABSENT
echo 'PASS: staged tag binding, stage-only handoff, and first-service refusal'
