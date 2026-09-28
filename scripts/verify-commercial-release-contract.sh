#!/usr/bin/env bash
# Static source contract: a commercial artifact must never share a name or
# execution route with an ad-hoc development package.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
dmg="$root/scripts/build-dmg.sh"
pkg="$root/scripts/build-pkg.sh"
workflow="$root/.github/workflows/release.yml"
makefile="$root/Makefile"

for file in "$dmg" "$pkg" "$workflow" "$makefile"; do
    [[ -f "$file" ]] || { echo "missing release-contract source: $file" >&2; exit 1; }
done

for needle in \
    '--development' \
    '--release' \
    'DEVELOPER_ID_APPLICATION APPLE_ID APPLE_TEAM_ID APPLE_APP_PASSWORD' \
    'SirsiPantheon-${VERSION}-dev-${ARCH}.dmg' \
    'xcrun notarytool submit' \
    'xcrun stapler validate'; do
    /usr/bin/grep -Fq -- "$needle" "$dmg" || { echo "DMG release contract missing: $needle" >&2; exit 1; }
done

for needle in \
    '--development' \
    '--release' \
    'DEVELOPER_ID_INSTALLER APPLE_ID APPLE_TEAM_ID APPLE_APP_PASSWORD' \
    'SirsiPantheon-${VERSION}-dev-${ARCH}.pkg' \
    'xcrun notarytool submit' \
    'xcrun stapler validate'; do
    /usr/bin/grep -Fq -- "$needle" "$pkg" || { echo "PKG release contract missing: $needle" >&2; exit 1; }
done

/usr/bin/grep -Fq 'scripts/build-dmg.sh --release' "$workflow" || { echo "release workflow does not request release DMG mode" >&2; exit 1; }
/usr/bin/grep -Fq 'scripts/build-pkg.sh --release' "$workflow" || { echo "release workflow does not request release PKG mode" >&2; exit 1; }
for target in dmg-dev pkg-dev release-dmg release-pkg; do
    /usr/bin/grep -Eq "^${target}:" "$makefile" || { echo "Makefile target missing: $target" >&2; exit 1; }
done

echo "commercial release contract: pass"
