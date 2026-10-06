#!/usr/bin/env bash
# release-prep-changelog.py cuts EVERY [Unreleased] block, drops entries a versioned
# section already carries, leaves exactly one empty [Unreleased], and never edits history.
# (Before 2026-10-06 it cut only the first block: 77 entries sat uncut for two months.)
set -euo pipefail
SCRIPT="$(cd "$(dirname "$0")" && pwd)/release-prep-changelog.py"
T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT
mkdir -p "$T/docs/stacklab/pantheon-pt/canon"
printf '# Changelog\n\nintro line\n\n## [Unreleased] — one\n\n- **alpha entry** first\n\n### Fixed\n- beta entry second\n\n## [Unreleased] — two\n\n- **gamma entry** third\n- delta already shipped in the old release\n\n## [Unreleased] — three\n\n- **alpha entry** first\n\n## [1.0.0] — 2026-01-01\n\n- delta already shipped in the old release\n' > "$T/CHANGELOG.md"
printf '# canon\n\n## 2026-01-01 — v1.0.0 release candidate\n\n- old\n' > "$T/docs/stacklab/pantheon-pt/canon/CHANGELOG.md"
(cd "$T" && python3 "$SCRIPT" 2.0.0 >/dev/null)
C="$T/CHANGELOG.md"
[ "$(grep -c '^## \[Unreleased\]' "$C")" = 1 ] || { echo "FAIL: want exactly one [Unreleased]"; exit 1; }
new="$(sed -n '/^## \[2.0.0\]/,/^## \[1.0.0\]/p' "$C")"
for want in "alpha entry" "beta entry" "gamma entry"; do echo "$new" | grep -q "$want" || { echo "FAIL: 2.0.0 lacks '$want'"; exit 1; }; done
[ "$(echo "$new" | grep -c 'alpha entry')" = 1 ] || { echo "FAIL: duplicate alpha"; exit 1; }
echo "$new" | grep -q "delta already shipped" && { echo "FAIL: re-released an entry 1.0.0 already carries"; exit 1; }
sed -n '/^## \[Unreleased\]/,/^## \[2.0.0\]/p' "$C" | grep -q '^- ' && { echo "FAIL: [Unreleased] not empty after the cut"; exit 1; }
sed -n '/^## \[1.0.0\]/,$p' "$C" | grep -q "delta already shipped" || { echo "FAIL: history edited"; exit 1; }
[ "$(cat "$T/VERSION")" = "2.0.0" ] || { echo "FAIL: VERSION"; exit 1; }
grep -q "v2.0.0 release candidate" "$T/docs/stacklab/pantheon-pt/canon/CHANGELOG.md" || { echo "FAIL: canon changelog"; exit 1; }
echo "ok: release-prep cuts every Unreleased block"
