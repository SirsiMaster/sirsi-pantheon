#!/usr/bin/env bash
# check-adr-numbers.sh — CI gate for ADR number integrity.
#
# Two independent checks, because the 2026-08-06 ADR-054 incident proved that
# uniqueness alone is not enough:
#
#   1. UNIQUENESS — no two docs/ADR-NNN-*.md files share a numeric prefix,
#      except an explicitly sanctioned companion set (see SANCTIONED_SHARED).
#   2. HIGH-WATER MARK — the "Next available: ADR-NNN" pointer in ADR-INDEX.md
#      must be strictly greater than the largest number on disk.
#
# Check 2 is the one that actually prevents collisions. On 2026-08-06 the index
# advertised ADR-055 as next-available while ADR-055 was already registered in
# that same file and two ADR-054 documents existed on disk with zero index
# mentions. An allocator that under-reports its high-water mark hands out a
# number that is already taken — which is exactly how PRs #465 and #495 landed
# ten seconds apart on the same number. Renaming files fixes one collision;
# gating the pointer fixes the machine that produces them.
#
# Prefix rules:
#   ADR-031-LOCAL-MODELS.md    → prefix "031"
#   ADR-031-A-NEVER-EXHAUST.md → prefix "031-A"  (lettered sub-ADR; distinct from plain 031)
#   ADR-013-THOTH-FOLD.md      → prefix "013"
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
DOCS_DIR="$ROOT/docs"
INDEX="$DOCS_DIR/ADR-INDEX.md"

# Numeric prefixes permitted to appear on more than one file, because the
# documents are companions of a single decision rather than a collision. Adding
# a number here is a canon decision: it must be stated in ADR-INDEX.md so a
# future reader does not "fix" the pair apart.
#   054 — ADR-054 One Horus (the fabric) + ADR-054-CONTRACTS (identity
#         enforcement + ledger schema v7). Registered as sharing 054 by design.
# ponytail: a flat prefix list is enough at one sanctioned pair; if a second
# appears, key the allowlist on the exact filename set instead.
SANCTIONED_SHARED=("054")

is_sanctioned() {
    local key="$1" s
    for s in "${SANCTIONED_SHARED[@]}"; do
        [[ "$key" == "$s" ]] && return 0
    done
    return 1
}

# Emit "<prefix> <path>" for every ADR file.
extract() {
    while IFS= read -r f; do
        base="$(basename "$f")"
        inner="${base#ADR-}"
        inner="${inner%.md}"
        IFS='-' read -ra parts <<< "$inner"
        num="${parts[0]}"
        if [[ "${#parts[@]}" -ge 2 && "${parts[1]}" =~ ^[A-Z]$ ]]; then
            echo "${num}-${parts[1]} ${f}"
        else
            echo "${num} ${f}"
        fi
    done < <(find "$DOCS_DIR" -maxdepth 1 -name 'ADR-[0-9]*-*.md' | sort)
}

fail=0

# --- Check 1: uniqueness -----------------------------------------------------
prev_key=""; prev_file=""
while read -r key file; do
    if [[ "$key" == "$prev_key" ]]; then
        if is_sanctioned "$key"; then
            echo "note: ADR prefix $key shared by design (sanctioned companion set)"
            echo "      $prev_file"
            echo "      $file"
        else
            echo "ERROR: duplicate ADR prefix $key"
            echo "  file 1: $prev_file"
            echo "  file 2: $file"
            fail=1
        fi
    fi
    prev_key="$key"; prev_file="$file"
done < <(extract | sort)

if [[ "$fail" -ne 0 ]]; then
    echo ""
    echo "FAIL: duplicate ADR number(s) found. Rename the draft(s) to an unused number,"
    echo "      or — if the documents are companions of one decision — add the number to"
    echo "      SANCTIONED_SHARED here AND state the pairing in docs/ADR-INDEX.md."
    exit 1
fi

# --- Check 2: index high-water mark ------------------------------------------
# This is the allocator check. Without it, uniqueness passes right up until two
# PRs race for the number the index wrongly claims is free.
if [[ ! -f "$INDEX" ]]; then
    echo "FAIL: $INDEX not found — the allocator pointer cannot be verified."
    exit 1
fi

next="$(grep -oE 'Next available:[[:space:]]*ADR-[0-9]{3}' "$INDEX" | head -1 | grep -oE '[0-9]{3}' || true)"
if [[ -z "$next" ]]; then
    echo "FAIL: no 'Next available: ADR-NNN' pointer found in docs/ADR-INDEX.md."
    echo "      The allocator high-water mark is unverifiable, which is how the"
    echo "      2026-08-06 ADR-054 collision happened. Restore the pointer."
    exit 1
fi

max="$(extract | awk '{print $1}' | grep -oE '^[0-9]{3}' | sort -n | tail -1)"

# 10# forces base-10: "054" would otherwise parse as invalid octal.
if (( 10#$next <= 10#$max )); then
    echo "ERROR: ADR-INDEX 'Next available: ADR-$next' is not above the highest number on disk (ADR-$max)."
    echo ""
    echo "FAIL: the index under-reports the high-water mark, so it will hand out a"
    echo "      number that already exists. Advance 'Next available' past ADR-$max."
    exit 1
fi

echo "OK: ADR numbers unique; index next-available ADR-$next is above highest on disk ADR-$max."
