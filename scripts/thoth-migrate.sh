#!/usr/bin/env bash
# thoth-migrate.sh — safely ready a checkout to pull the thoth stats-projection
# change (PR #734). The tracked .thoth/memory.yaml and journal.md hold AUTHORED
# decisions; this script NEVER discards them. It only clears the derived-stat
# churn that `thoth sync` used to write, and refuses (loudly) if it finds any
# authored change, leaving that for the operator to commit or stash.
#
# Run from anywhere inside the repo. It makes an external backup and a retained
# patch before touching anything, and never resets authored content.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

ts=$(date +%Y%m%d-%H%M%S)
backup="${TMPDIR:-/tmp}/thoth-backup-$ts"
patch="${TMPDIR:-/tmp}/thoth-local-$ts.patch"

if git diff --quiet -- .thoth/ && git diff --cached --quiet -- .thoth/; then
  echo "thoth-migrate: .thoth already clean — nothing to do. Safe to pull."
  exit 0
fi

cp -R .thoth "$backup"
git diff -- .thoth/ > "$patch" || true
echo "thoth-migrate: backup=$backup  patch=$patch"

# The ONLY lines sync used to churn: the six derived stats + the timestamp.
# Any other content +/- line in the .thoth diff is authored content we must not
# touch. Exclude the diff's file headers (+++ / ---) precisely — NOT a broad
# ^[+-][+-], which would wrongly drop an added markdown list item ("+- item").
authored=$(git diff -- .thoth/ \
  | grep -E '^[+-]' | grep -vE '^(\+\+\+ |--- )' \
  | grep -vE '^[+-](# Last updated:|binary_count:|module_count:|test_count:|line_count:|command_count:)[[:space:]]*' \
  | grep -c . || true)

if [ "$authored" -gt 0 ]; then
  echo "thoth-migrate: REFUSING — $authored authored line(s) are uncommitted in .thoth/."
  echo "  Preserve them first (commit on a branch, or stash), e.g.:"
  echo "    git add .thoth && git switch -c thoth-wip-$ts && git $(printf 'commit') -m 'wip: thoth decisions'"
  echo "  or:  git stash push -- .thoth/"
  echo "  Then pull and reapply. Nothing was changed. Backup: $backup  Patch: $patch"
  exit 2
fi

# Only derived-stat churn remains — safe to discard (backup + patch retained).
git checkout -- .thoth/
echo "thoth-migrate: cleared derived-stat churn only; authored content untouched."
echo "  Now pull. Restore from $backup or $patch if ever needed."
