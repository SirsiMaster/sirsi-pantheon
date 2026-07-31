#!/usr/bin/env bash
# check-font-scaling.sh — structural guard: every user-visible font on the
# menubar surface must route through the width-derived type scale.
#
# WHY THIS EXISTS: #319 removed element/text scaling outright. #320 restored it
# in Views.swift but MISSED 16 sites across four other files in the same target,
# so numerals scaled while body copy in the same card did not. The owner's
# report was "text is super tiny" — mixed scaling reads worse than none.
#
# WHY IT SCANS THE WHOLE TARGET: the first version of this guard scanned only
# Views.swift and therefore reported "0 unscaled" while those 16 bypasses were
# live in MERGED code (codex re-review of #320). A guard scoped more narrowly
# than the invariant it claims is worse than no guard — it converts an unknown
# risk into a false assurance.
#
# The only legal `.font(` calls are the scaling primitives themselves. They are
# exempted by MARKER, not by filename, so moving one to another file cannot
# silently widen the exemption.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
SRC_DIR="$HERE/Sources/SirsiMenubar"
[ -d "$SRC_DIR" ] || { echo "check-font-scaling: cannot find $SRC_DIR" >&2; exit 2; }

# A `.font(` line carrying this marker IS a scaling implementation.
MARKER="sirsi:scaling-primitive"

offenders=""
while IFS= read -r file; do
  # Comment lines are stripped: the modifiers' own docs mention
  # `.font(.system(...))` in prose, and a guard that trips on its own
  # explanation is one people learn to delete rather than satisfy.
  hits="$(grep -n '\.font(' "$file" \
    | grep -vE '^[0-9]+:[[:space:]]*//' \
    | grep -v "$MARKER" || true)"
  if [ -n "$hits" ]; then
    offenders="${offenders}$(echo "$hits" | sed "s|^|$(basename "$file"):|")
"
  fi
done < <(find "$SRC_DIR" -name '*.swift' -type f | sort)

if [ -n "$(echo "$offenders" | tr -d '[:space:]')" ]; then
  echo "✘ unscaled font site(s) — these bypass sirsiTypeScale and will not resize with the window:" >&2
  printf '%s' "$offenders" >&2
  echo >&2
  echo "  Use .sirsiFont(12, weight:) for explicit sizes, or .sirsiFont(.caption) for semantic styles." >&2
  echo "  A scaling primitive must carry the '$MARKER' marker on its .font( line." >&2
  exit 1
fi

scaled="$(grep -rho '\.sirsiFont(' "$SRC_DIR" --include='*.swift' | wc -l | tr -d ' ')"
files="$(find "$SRC_DIR" -name '*.swift' -type f | wc -l | tr -d ' ')"
echo "✔ all fonts scaled — $scaled sirsiFont site(s) across $files file(s), 0 unscaled"
