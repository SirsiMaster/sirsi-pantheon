#!/bin/bash
# Static source contract: a commercial artifact must never share a name or
# execution route with an ad-hoc development package.
set -euo pipefail

# Keep the source verifier reproducible when it is invoked from a restricted
# project shell. It never inherits a caller-provided executable directory.
export PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"

root="$(cd "$(dirname "$0")/.." && pwd)"
dmg="$root/scripts/build-dmg.sh"
pkg="$root/scripts/build-pkg.sh"
workflow="$root/.github/workflows/release.yml"
makefile="$root/Makefile"
recipe="$root/contracts/stacklab/pantheon-release-artifact-recipe-v1.json"

for file in "$dmg" "$pkg" "$workflow" "$makefile" "$recipe"; do
    [[ -f "$file" ]] || { echo "missing release-contract source: $file" >&2; exit 1; }
done

for needle in \
    '#!/bin/bash' \
    'export PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"' \
    '--development' \
    '--release' \
    'DEVELOPER_ID_APPLICATION APPLE_ID APPLE_TEAM_ID APPLE_APP_PASSWORD' \
    'SirsiPantheon-${VERSION}-dev-${ARCH}.dmg' \
    'xcrun notarytool submit' \
    'xcrun stapler validate'; do
    /usr/bin/grep -Fq -- "$needle" "$dmg" || { echo "DMG release contract missing: $needle" >&2; exit 1; }
done

for needle in \
    '#!/bin/bash' \
    'export PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"' \
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
/usr/bin/grep -Fq 'go build ./cmd/sirsi' "$workflow" || { echo "release workflow does not compile the portable sirsi target" >&2; exit 1; }
/usr/bin/grep -Fq 'go build ./cmd/sirsi-agent' "$workflow" || { echo "release workflow does not compile the portable sirsi-agent target" >&2; exit 1; }
if /usr/bin/grep -Eq '^\s*go build \./\.\.\.\s*$' "$workflow"; then
    echo "release workflow tries to compile macOS-only GUI packages on Linux" >&2
    exit 1
fi
for target in dmg-dev pkg-dev release-dmg release-pkg; do
    /usr/bin/grep -Eq "^${target}:" "$makefile" || { echo "Makefile target missing: $target" >&2; exit 1; }
done

/usr/bin/jq -e '
  .schema == "sirsi.stacklab.recipe.v1" and
  .id == "stacklab.recipe.pantheon-release-artifact" and
  ([.components[].id] | sort) == [
    "commercial-sign-notary-publication-route",
    "release-artifact-class-contract",
    "release-native-payload-composition"
  ]
' "$recipe" >/dev/null || { echo "Stack Lab release-artifact recipe is incomplete" >&2; exit 1; }

# README is emitted through an expanding heredoc. Command-substitution markup
# in user-facing copy would execute during packaging and silently corrupt the
# staged artifact, so keep the CLI name literal and assert the safe wording.
/usr/bin/grep -Fq 'The bundle includes the menu bar app and the sirsi CLI' "$dmg" || {
    echo "DMG README does not name the bundled sirsi CLI safely" >&2; exit 1;
}
if /usr/bin/grep -Fq '`sirsi`' "$dmg"; then
    echo "DMG README contains executable command-substitution markup" >&2; exit 1
fi

echo "commercial release contract: pass"
