#!/usr/bin/env bash
# Self-check for changelog-assemble.sh. Runs against a throwaway copy of the
# repo layout so it never touches the real CHANGELOG.
set -euo pipefail

script=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/changelog-assemble.sh
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/scripts" "$tmp/changelog.d"
cp "$script" "$tmp/scripts/"

fail() { echo "FAIL: $*" >&2; exit 1; }

printf '# Changelog\n\n## [Unreleased]\n\n- pre-existing entry\n' > "$tmp/CHANGELOG.md"
cat > "$tmp/changelog.d/README.md" <<'R'
directory docs, must never be consumed
R
echo '- **older entry** (a)' > "$tmp/changelog.d/20260101-older.md"
echo '- **newer entry** (b)' > "$tmp/changelog.d/20260202-newer.md"

( cd "$tmp" && ./scripts/changelog-assemble.sh >/dev/null )

out=$(cat "$tmp/CHANGELOG.md")
grep -q 'older entry' <<<"$out" || fail "older entry missing"
grep -q 'newer entry' <<<"$out" || fail "newer entry missing"
grep -q 'pre-existing entry' <<<"$out" || fail "clobbered existing content"

# newest first
newer_at=$(grep -n 'newer entry' <<<"$out" | cut -d: -f1)
older_at=$(grep -n 'older entry' <<<"$out" | cut -d: -f1)
[[ $newer_at -lt $older_at ]] || fail "not newest-first ($newer_at !< $older_at)"

# entries consumed, README kept
[[ ! -f "$tmp/changelog.d/20260101-older.md" ]] || fail "entry not removed"
[[ -f "$tmp/changelog.d/README.md" ]] || fail "README.md was consumed — it is docs, not an entry"

# empty dir is a no-op, not an error (release scripts run this unconditionally)
( cd "$tmp" && ./scripts/changelog-assemble.sh >/dev/null ) || fail "empty run should exit 0"
before=$(cat "$tmp/CHANGELOG.md")
( cd "$tmp" && ./scripts/changelog-assemble.sh >/dev/null )
[[ "$before" == "$(cat "$tmp/CHANGELOG.md")" ]] || fail "empty run mutated CHANGELOG"

# missing marker is a loud failure, not a silent no-op
printf '# Changelog\n\nno marker here\n' > "$tmp/CHANGELOG.md"
echo '- entry' > "$tmp/changelog.d/20260303-x.md"
if ( cd "$tmp" && ./scripts/changelog-assemble.sh >/dev/null 2>&1 ); then
  fail "missing marker should exit non-zero"
fi

echo "PASS: changelog-assemble.sh"
