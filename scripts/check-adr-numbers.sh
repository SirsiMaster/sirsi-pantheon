#!/usr/bin/env bash
# Fail if any two ADR files share the same effective ADR key.
#
# Key rules:
#   docs/ADR-NNN-TITLE.md          → key = NNN
#   docs/ADR-NNN-X-TITLE.md        → key = NNN-X   (lettered sub-ADR; X = single A-Z)
#
# Two files with the same key are a collision and fail CI.
# ADR-031-LOCAL-MODELS... (key 031) and ADR-031-A-NEVER-EXHAUST... (key 031-A) are distinct — OK.
set -euo pipefail

keys=$(ls docs/ADR-[0-9][0-9][0-9]-*.md 2>/dev/null \
  | sed -E 's|docs/ADR-([0-9]{3})-([A-Z])-.*|\1-\2|; s|docs/ADR-([0-9]{3})-.*|\1|' \
  | sort)

dups=$(echo "$keys" | uniq -d)

if [ -n "$dups" ]; then
  echo "ERROR: Duplicate ADR keys detected:" >&2
  echo "$dups" >&2
  echo "" >&2
  echo "Conflicting files:" >&2
  for d in $dups; do
    if [[ "$d" =~ ^([0-9]{3})-([A-Z])$ ]]; then
      num="${BASH_REMATCH[1]}"
      letter="${BASH_REMATCH[2]}"
      ls docs/ADR-"$num"-"$letter"-*.md 2>/dev/null >&2 || true
    else
      ls docs/ADR-"$d"-*.md 2>/dev/null | grep -v -E "docs/ADR-$d-[A-Z]-" >&2 || true
    fi
  done
  exit 1
fi

total=$(echo "$keys" | wc -l | tr -d ' ')
unique=$(echo "$keys" | sort -u | wc -l | tr -d ' ')
echo "ADR numbers OK ($total ADR files, $unique unique keys)"
