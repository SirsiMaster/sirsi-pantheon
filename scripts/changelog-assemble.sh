#!/usr/bin/env bash
# Assemble changelog.d/*.md into CHANGELOG.md under "## [Unreleased]".
#
# Entries are inserted newest-first by filename (YYYYMMDD- prefix sorts
# correctly), then the consumed files are removed. Idempotent on an empty
# changelog.d/: it is a no-op, so it is safe to run in a release script
# unconditionally.
#
# ponytail: plain concatenation, no towncrier/changesets dependency — a
# directory plus `cat` is the whole feature; adopt a tool only if the release
# process grows requirements this cannot express.
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo_root"

dir=changelog.d
target=CHANGELOG.md
marker='## [Unreleased]'

shopt -s nullglob
entries=("$dir"/*.md)
# README.md documents the directory; it is not an entry.
filtered=()
for f in "${entries[@]}"; do
  [[ $(basename "$f") == "README.md" ]] && continue
  filtered+=("$f")
done

if [[ ${#filtered[@]} -eq 0 ]]; then
  echo "changelog.d/: no entries to assemble — nothing to do."
  exit 0
fi

grep -qF "$marker" "$target" || { echo "error: '$marker' not found in $target" >&2; exit 1; }

# Newest first: reverse filename order (YYYYMMDD- prefix makes this chronological).
# ponytail: while-read, not mapfile — macOS ships bash 3.2 and mapfile is 4.0+.
sorted=()
while IFS= read -r line; do sorted+=("$line"); done < <(printf '%s\n' "${filtered[@]}" | sort -r)

# ponytail: head/cat/tail splice rather than awk -v — awk cannot carry a
# multiline value through -v, and these entries are paragraphs.
line=$(grep -nF -m1 "$marker" "$target" | cut -d: -f1)

tmp=$(mktemp)
{
  head -n "$line" "$target"
  echo
  for f in "${sorted[@]}"; do cat "$f"; echo; done
  tail -n +$((line + 1)) "$target"
} > "$tmp"
mv "$tmp" "$target"

rm -f "${sorted[@]}"
echo "assembled ${#sorted[@]} entr$([[ ${#sorted[@]} -eq 1 ]] && echo y || echo ies) into $target"
