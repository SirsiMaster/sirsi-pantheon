#!/usr/bin/env bash
# thoth-migrate.sh — safely ready a checkout to pull the thoth stats-projection
# change (PR #734), which edits the tracked .thoth/memory.yaml.
#
# It NEVER discards anything and NEVER classifies content. It preserves every
# local .thoth change — staged and unstaged, text or binary, any file — in a
# git stash plus an external backup, leaving .thoth clean so the pull applies.
# The operator then decides what to reapply. Fail-safe by construction: there is
# no path in which authored content is lost.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

ts=$(date +%Y%m%d-%H%M%S)
backup="${TMPDIR:-/tmp}/thoth-backup-$ts"

if git diff --quiet -- .thoth/ && git diff --cached --quiet -- .thoth/; then
  echo "thoth-migrate: .thoth already clean — safe to: git pull"
  exit 0
fi

cp -R .thoth "$backup"

# Preserve EVERYTHING under .thoth (index + worktree) in a stash. No discard,
# no classification — the stash and the backup hold all local state.
git stash push -- .thoth/ >/dev/null

# Postcondition: .thoth must now be clean, or we refuse loudly and restore.
if ! git diff --quiet -- .thoth/ || ! git diff --cached --quiet -- .thoth/; then
  echo "thoth-migrate: FAILED to fully stash .thoth changes — nothing discarded."
  echo "  Restore from $backup and reconcile manually before pulling."
  exit 1
fi

echo "thoth-migrate: local .thoth changes stashed (see 'git stash list'); backup at $backup."
echo "  Next:"
echo "    1) git pull"
echo "    2) if you had uncommitted AUTHORED decisions, reapply them:  git stash pop"
echo "       (resolve any conflict on the removed stat lines by keeping the incoming version)"
echo "       if it was only derived-stat churn, just discard it:       git stash drop"
