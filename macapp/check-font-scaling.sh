#!/usr/bin/env bash
# check-font-scaling.sh — structural guard: every user-visible font on the
# menubar surface must route through the width-derived type scale.
#
# WHY THIS EXISTS: #319 removed element/text scaling outright. #320 restored it
# for the 73 explicit `.font(.system(size:))` sites but missed 136 SEMANTIC ones
# (.caption/.caption2/.callout/.headline/...), so numerals scaled while body copy
# in the same card did not. The owner's report was "text is super tiny" — mixed
# scaling reads worse than none. A count-based check is the only thing that
# catches a single new `.font(.caption)` slipping back in during a later edit.
#
# The ONLY legal `.font(` calls are the two inside the ScaledFont /
# ScaledSemanticFont modifiers, which are what apply the scale.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
SRC="$HERE/Sources/SirsiMenubar/Views.swift"
[ -f "$SRC" ] || { echo "check-font-scaling: cannot find $SRC" >&2; exit 2; }

# Strip the sanctioned call sites (`content.font(...)` inside the modifiers),
# then anything still calling .font( is an unscaled, user-visible font.
# Comment lines are stripped first: the modifiers' own documentation mentions
# `.font(.system(size:weight:design:))` in prose, and a guard that trips on its
# own explanation is a guard people learn to ignore.
offenders="$(grep -n '\.font(' "$SRC" | grep -v 'content\.font(' | grep -vE '^[0-9]+:[[:space:]]*//' || true)"

if [ -n "$offenders" ]; then
  echo "✘ unscaled font site(s) — these bypass sirsiTypeScale and will not resize with the window:" >&2
  echo "$offenders" >&2
  echo >&2
  echo "  Use .sirsiFont(12, weight:) for explicit sizes, or .sirsiFont(.caption) for semantic styles." >&2
  exit 1
fi

scaled="$(grep -c '\.sirsiFont(' "$SRC" || true)"
echo "✔ all fonts scaled — $scaled sirsiFont site(s), 0 unscaled"
