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
cask_cmd="$root/cmd/sirsi/cask_release.go"
cask_package="$root/internal/caskrelease/cask.go"
package_inventory="$root/internal/packageinventory/inventory.go"
package_inventory_cmd="$root/cmd/sirsi/packageinventorycmd.go"
package_inventory_adapter="$root/internal/packageinventorycmd/verify.go"

for file in "$dmg" "$pkg" "$workflow" "$makefile" "$recipe" "$cask_cmd" "$cask_package" "$package_inventory" "$package_inventory_cmd" "$package_inventory_adapter"; do
    [[ -f "$file" ]] || { echo "missing release-contract source: $file" >&2; exit 1; }
done

# Casks are owned by the release workflow's remote-tap transaction. A tracked
# local mirror becomes an unreviewed second publication source and can mislead
# operators into installing stale development-era bytes.
[[ ! -e "$root/homebrew/Casks/sirsi-pantheon.rb" ]] || {
    echo "stale local cask mirror must not coexist with the canonical remote-tap route" >&2
    exit 1
}

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

for file in "$dmg" "$pkg"; do
    /usr/bin/grep -Fq 'package-inventory' "$file" || {
        echo "package builder does not invoke canonical payload inventory: $file" >&2
        exit 1
    }
done
for target in dmg-dev pkg-dev release-dmg release-pkg; do
    /usr/bin/grep -Eq "^${target}:" "$makefile" || { echo "Makefile target missing: $target" >&2; exit 1; }
done

# Packaging is required to ship the native SwiftUI application.  The retained
# Go/systray source is compatibility history, not an alternate product payload.
/usr/bin/grep -Fq 'swift build -c release' "$dmg" || {
    echo "DMG build does not compile the canonical Swift menubar" >&2
    exit 1
}
if /usr/bin/grep -Fq 'go build -ldflags="${GO_LDFLAGS}" -o "${BUILD_DIR}/sirsi-menubar"' "$dmg"; then
    echo "DMG build retains the retired Go menubar fallback" >&2
    exit 1
fi
/usr/bin/grep -A5 '^build-menubar:' "$makefile" | /usr/bin/grep -Fq 'swift build -c release' || {
    echo "Makefile build-menubar does not compile the canonical Swift surface" >&2
    exit 1
}

/usr/bin/jq -e '
  .schema == "sirsi.stacklab.recipe.v1" and
  .id == "stacklab.recipe.pantheon-release-artifact" and
  ([.components[].id] | sort) == [
    "canonical-cask-publication",
    "commercial-sign-notary-publication-route",
    "release-artifact-class-contract",
    "release-native-payload-composition"
  ]
' "$recipe" >/dev/null || { echo "Stack Lab release-artifact recipe is incomplete" >&2; exit 1; }

# The cask is rendered and verified by one typed source route after the signed
# DMG has been uploaded. Two independent workflow mutations can race and leave
# Homebrew with an unverified version/hash pair.
[[ $(/usr/bin/grep -Ec '^  bump-cask:$' "$workflow") -eq 1 ]] || {
    echo "release workflow must contain exactly one cask publication job" >&2; exit 1;
}
/usr/bin/grep -Fq 'cask-release render' "$workflow" || {
    echo "release workflow does not use the canonical cask renderer" >&2; exit 1;
}
/usr/bin/grep -Fq 'cask-release verify' "$workflow" || {
    echo "release workflow does not read back and verify published cask bytes" >&2; exit 1;
}
if /usr/bin/grep -Fq 'Bump Homebrew Cask in tap' "$workflow" || \
   /usr/bin/grep -Fq 'perl -0pi' "$workflow" || \
   /usr/bin/grep -Fq 'git clone --depth 1' "$workflow"; then
    echo "release workflow retains a duplicate or mutable cask update route" >&2
    exit 1
fi

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
