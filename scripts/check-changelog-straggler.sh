#!/usr/bin/env bash
# check-changelog-straggler.sh — CHANGELOG.md is assembled, not hand-edited
# (changelog.d/README.md). A non-release branch that diffs CHANGELOG.md
# directly re-introduces the conflict class changelog.d/ exists to make
# structurally impossible (A7/A26) — the entry belongs in changelog.d/ instead.
#
# Runs in CI (lint job) and the Ma'at pre-push gate. `--self-test` plants a
# violation in a temp repo and expects this script to go red.
set -euo pipefail

SELF_TEST=0
BASE=""
HEAD="HEAD"
BRANCH_OVERRIDE=""
for a in "$@"; do
  case "$a" in
    --self-test) SELF_TEST=1 ;;
    --base=*) BASE="${a#--base=}" ;;
    --head=*) HEAD="${a#--head=}" ;;
    --branch=*) BRANCH_OVERRIDE="${a#--branch=}" ;;
  esac
done

self_test() {
  TMP="$(mktemp -d)"
  trap 'rm -rf "$TMP"' EXIT
  git init -q -b main "$TMP"
  (cd "$TMP" && git config user.email t@t.com && git config user.name t
   echo "# CHANGELOG" > CHANGELOG.md
   git add CHANGELOG.md && git commit -q -m base
   git checkout -q -b feature/x
   echo "- straggler entry" >> CHANGELOG.md
   git commit -q -am "edit changelog directly")
  if (cd "$TMP" && bash "$SCRIPT" --base=main --head=feature/x) ; then
    echo "  ❌ self-test FAILED: straggler CHANGELOG.md edit was not caught"
    exit 1
  fi
  echo "  ✅ self-test passed: straggler edit on feature/x correctly rejected"
  exit 0
}

SCRIPT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/$(basename "${BASH_SOURCE[0]}")"
[ "$SELF_TEST" = "1" ] && self_test

BRANCH="${BRANCH_OVERRIDE:-$(git rev-parse --abbrev-ref "$HEAD" 2>/dev/null || echo "$HEAD")}"
case "$BRANCH" in
  release/*) exit 0 ;;  # the release train is the only legitimate CHANGELOG.md writer
esac

if [ -z "$BASE" ]; then
  BASE=$(git merge-base "$HEAD" origin/main 2>/dev/null || git merge-base "$HEAD" main 2>/dev/null || echo "")
fi
[ -z "$BASE" ] && exit 0  # no base to diff against (e.g. root commit) — nothing to check

if git diff --name-only "$BASE" "$HEAD" -- CHANGELOG.md 2>/dev/null | grep -q .; then
  echo "  ❌ CHANGELOG.md was edited directly on '$BRANCH'."
  echo "     Add a fragment under changelog.d/ instead (see changelog.d/README.md)."
  echo "     Only a release/* branch (scripts/release-train.sh) assembles CHANGELOG.md."
  exit 1
fi
exit 0
