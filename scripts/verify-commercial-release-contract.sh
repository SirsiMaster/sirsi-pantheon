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
bootstrap="$root/scripts/install.sh"
bootstrap_test="$root/scripts/install.test.sh"

for file in "$dmg" "$pkg" "$workflow" "$makefile" "$recipe" "$cask_cmd" "$cask_package" "$package_inventory" "$package_inventory_cmd" "$package_inventory_adapter" "$bootstrap" "$bootstrap_test"; do
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
    'PANTHEON_SIGNING_EXECUTION:-' \
    'direct --release requires' \
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
/usr/bin/grep -Fq 'DEVELOPER_ID_INSTALLER is required for the commercial Pantheon PKG' "$workflow" || {
    echo "release workflow does not fail closed when installer signing is unavailable" >&2
    exit 1
}
/usr/bin/grep -Fq 'Preflight imported Developer ID Installer identity' "$workflow" || {
    echo "release workflow does not preflight the protected Installer identity before packaging" >&2
    exit 1
}
/usr/bin/grep -Fq 'Selected signing keychain does not contain a usable Team 9D382WV988 Developer ID Installer identity' "$workflow" || {
    echo "release workflow does not verify Installer identity availability in its temporary keychain" >&2
    exit 1
}
for required in \
    'test -f "bin/SirsiPantheon-${BUILD_VERSION}-arm64.dmg"' \
    'test -f "bin/SirsiPantheon-${BUILD_VERSION}-arm64.pkg"' \
    'gh release upload "$TAG_NAME"'; do
    /usr/bin/grep -Fq -- "$required" "$workflow" || {
        echo "release workflow does not require both native release artifacts: $required" >&2
        exit 1
    }
done
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
    "commercial-update-payload-eligibility",
    "release-artifact-class-contract",
    "release-native-payload-composition"
  ] and
  ([.components[] | select(.id == "release-native-payload-composition")][0] |
    (.inputs | index("macapp/Package.swift and native Swift menubar source")) != null and
    (.outputs | index("one Pantheon.app payload containing CLI, the canonical Swift menubar, LaunchAgent resource, and Stack Lab contracts")) != null and
    (.upgrade_recipe | index("require macapp/Package.swift and fail packaging instead of substituting the retired Go menubar")) != null)
' "$recipe" >/dev/null || { echo "Stack Lab release-artifact recipe is incomplete" >&2; exit 1; }

/usr/bin/jq -e '
  [.components[] | select(.id == "commercial-update-payload-eligibility")][0] |
  (.source | index("internal/updater/updater.go")) != null and
  (.source | index("internal/updater/install.go")) != null and
  (.source | index("cmd/sirsi/update.go")) != null and
  (.outputs | index("only a complete same-version Pantheon DMG and PKG pair is update-eligible")) != null and
  (.outputs | index("an explicit update request preserves the installed product and offers a recheck recovery when no complete payload exists")) != null and
  (.upgrade_recipe | index("keep every no-complete-release state non-destructive and actionable")) != null and
  (.upgrade_recipe | index("do not restore a standalone binary replacement route that can drift from the app payload")) != null
' "$recipe" >/dev/null || {
    echo "Stack Lab release route must bind complete commercial update eligibility" >&2; exit 1;
}

for required in 'ErrNoCompleteCommercialRelease' 'IsCompleteCommercialRelease' 'CommercialDMGAsset' 'CommercialPKGAsset'; do
    /usr/bin/grep -Fq -- "$required" "$root/internal/updater/updater.go" "$root/internal/updater/install.go" || {
        echo "commercial update eligibility is missing: $required" >&2; exit 1;
    }
done
for forbidden in 'installCLIRelease' 'schemaCompatibilityGate('; do
    if /usr/bin/grep -Fq -- "$forbidden" "$root/cmd/sirsi/update.go"; then
        echo "commercial updater retains standalone CLI replacement route: $forbidden" >&2; exit 1;
    fi
done

/usr/bin/jq -e '
  [.components[] | select(.id == "commercial-sign-notary-publication-route")][0] |
  (.upgrade_recipe | index("ship the tagged commercial route as one macOS payload contract; reject variable-gated Windows installer jobs until a separate platform release contract exists")) != null and
  (.upgrade_recipe | index("create or revalidate the exact GitHub release record only after successful commercial DMG and PKG construction")) != null and
  (.outputs | index("one exact GitHub release record and the same-payload assets suitable for cask binding")) != null
' "$recipe" >/dev/null || {
    echo "Stack Lab release route must record the macOS-only tag-artifact boundary" >&2
    exit 1
}

/usr/bin/jq -e '
  [.components[] | select(.id == "canonical-cask-publication")][0] |
  (.source | index("scripts/install.sh")) != null and
  (.tests | index("scripts/install.test.sh")) != null and
  (.outputs | index("Cask-linked bundled sirsi CLI")) != null and
  (.upgrade_recipe | index("link the CLI from the installed Pantheon.app rather than publishing a separate archive")) != null
' "$recipe" >/dev/null || {
    echo "Stack Lab Cask component does not bind the supported bootstrap route" >&2
    exit 1
}

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
/usr/bin/grep -Fq 'binary "#{appdir}/Pantheon.app/Contents/MacOS/sirsi", target: "sirsi"' "$cask_package" || {
    echo "canonical Cask does not link the CLI from Pantheon.app" >&2; exit 1;
}
for required in 'brew install --cask' 'brew upgrade --cask' 'sirsimaster/tools/sirsi-pantheon' 'Homebrew installed the Cask but did not expose its bundled CLI'; do
    /usr/bin/grep -Fq -- "$required" "$bootstrap" || {
        echo "supported bootstrap is missing: $required" >&2; exit 1;
    }
done
for forbidden in 'goreleaser' 'sirsi-menubar_' 'sirsi-pantheon_${LATEST#v}' 'go install "github.com/${REPO}/cmd/sirsi@latest"'; do
    if /usr/bin/grep -Fq -- "$forbidden" "$bootstrap"; then
        echo "bootstrap retains an unsupported separate artifact route: $forbidden" >&2
        exit 1
    fi
done
if /usr/bin/grep -Fq 'Bump Homebrew Cask in tap' "$workflow" || \
   /usr/bin/grep -Fq 'perl -0pi' "$workflow" || \
   /usr/bin/grep -Fq 'git clone --depth 1' "$workflow"; then
    echo "release workflow retains a duplicate or mutable cask update route" >&2
    exit 1
fi

# Pantheon ships one supported commercial macOS payload: the signed native
# Pantheon.app in its DMG and PKG. A dormant Windows tag job silently widens a
# release into a second, unaudited installer product. Platform expansion must
# arrive as a separately designed and reviewed release contract, never via a
# repository variable that changes a macOS tag at runtime.
for forbidden in 'windows-installer:' 'windows-latest' 'ENABLE_WINDOWS_BUILD' 'makensis' 'windows-setup.exe'; do
    if /usr/bin/grep -Fq -- "$forbidden" "$workflow"; then
        echo "release workflow retains an unsupported Windows tag artifact: $forbidden" >&2
        exit 1
    fi
done

# The source runner can validate portable Go packages, but it must never mint a
# partial release. The release record and every published product asset are
# created only after the macOS job has successfully built the commercial DMG
# and PKG from the same Pantheon.app bundle.
/usr/bin/grep -Fq 'Create or verify the exact GitHub release record' "$workflow" || {
    echo "release workflow does not create the release record after native packaging" >&2; exit 1;
}
/usr/bin/grep -Fq -- '--target "$EXPECTED_COMMIT"' "$workflow" || {
    echo "release workflow does not bind the release record to the checked-out tag commit" >&2; exit 1;
}
for forbidden in 'goreleaser-action' 'Run GoReleaser' 'sirsi-menubar_${BUILD_VERSION}_darwin_arm64.tar.gz'; do
    if /usr/bin/grep -Fq -- "$forbidden" "$workflow"; then
        echo "release workflow retains a partial or duplicate release publisher: $forbidden" >&2
        exit 1
    fi
done

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
