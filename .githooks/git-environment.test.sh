#!/usr/bin/env bash
set -euo pipefail
hook="$(cd "$(dirname "$0")" && pwd)/pre-push"
definition=$(awk '/^run_git_isolated_tests\(\) \(/,/^\)/' "$hook")
test -n "$definition"
eval "$definition"
# This fixture itself creates foreign repositories. Clear inherited hook-local
# Git variables before the first init, not only inside the function under test.
initial_git_names=$(git rev-parse --local-env-vars)
for name in $initial_git_names; do unset "$name"; done
fixture=$(mktemp -d "${TMPDIR:-/tmp}/sirsi-hook-env.XXXXXX")
cleanup() { rm -rf -- "$fixture"; }
trap cleanup EXIT
mkdir "$fixture/outer" "$fixture/foreign"
git -C "$fixture/outer" init -q
git -C "$fixture/outer" config user.name original
before=$(git -C "$fixture/outer" config user.name)
export GIT_DIR="$fixture/outer/.git"
export GIT_COMMON_DIR="$fixture/outer/.git"
export GIT_WORK_TREE="$fixture/outer"
export SIRSI_AGENT_ID=fixture-parent
run_git_isolated_tests bash -c '
  test -z "${GIT_DIR+x}" && test -z "${GIT_COMMON_DIR+x}" && test -z "${GIT_WORK_TREE+x}"
  test -z "${SIRSI_AGENT_ID+x}"
  git -C "$1" init -q
  git -C "$1" config user.name foreign
  test "$(git -C "$1" config user.name)" = foreign
' _ "$fixture/foreign"
test "$(git config user.name)" = "$before"
test "$GIT_DIR" = "$fixture/outer/.git"
test "$SIRSI_AGENT_ID" = fixture-parent
if run_git_isolated_tests bash -c 'exit 23'; then exit 1; else test "$?" = 23; fi
# RED control: without isolation a foreign-path command mutates the parent.
git -C "$fixture/foreign" config user.name contaminated
test "$(git config user.name)" = contaminated
# A broken environment discovery must refuse before running the workload.
(
  git() { return 9; }
  if run_git_isolated_tests touch "$fixture/should-not-exist"; then exit 1; fi
  test ! -e "$fixture/should-not-exist"
)
echo 'PASS: foreign Git isolation, parent identity preservation, exit propagation, RED contamination, discovery fail-closed'
