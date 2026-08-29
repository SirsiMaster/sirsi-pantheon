#!/usr/bin/env bash
# Verify an unsigned Pantheon release-source candidate before an annotated tag,
# Developer ID signing, notarization, asset upload, or cask mutation. This is
# deliberately a SOURCE gate: it proves intent and transition mechanics, never
# package bytes or installation.
set -euo pipefail

die() { echo "pantheon_unsigned_release_source accepted=false reason=$1" >&2; exit 1; }

if [[ "${1:-}" != "--pretag" ]]; then
  die "usage_requires_pretag"
fi
TAG="${2:-}"
COMMIT="${3:-}"
[[ -n "$TAG" && -n "$COMMIT" ]] || die "missing_identity"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

# The shared verifier owns SemVer, VERSION, exact HEAD, and any pre-existing
# tag target check. Do not duplicate those rules and let the two gates drift.
source_identity="$(bash scripts/verify-release-source-identity.sh --pretag "$TAG" "$COMMIT")" || exit $?
TREE="$(git rev-parse --verify "${COMMIT}^{tree}" 2>/dev/null)" || die "missing_source_tree"
BRANCH="$(git branch --show-current 2>/dev/null || true)"
[[ -n "$BRANCH" ]] || BRANCH="detached"

for path in \
  scripts/build-dmg.sh \
  scripts/verify-pantheon-package-identity.sh \
  scripts/render-pantheon-homebrew-cask.sh \
  scripts/verify-pantheon-release-asset.sh \
  homebrew/Casks/sirsi-pantheon.rb; do
  [[ -f "$path" ]] || die "missing_contract_input:${path}"
done

VERSION="${TAG#v}"
CASK_VERSION="$(sed -nE 's/^[[:space:]]*version "([^"]+)".*/\1/p' homebrew/Casks/sirsi-pantheon.rb | head -1)"
[[ -n "$CASK_VERSION" ]] || die "unparseable_current_cask"

# A checked-in cask must never get ahead of a signed, visible DMG. It is valid
# for it to describe an older release while this pretag source is credential
# gated; rendering the new cask is owned by the post-upload verifier instead.
[[ "$CASK_VERSION" != "$VERSION" ]] || die "cask_already_claims_unsigned_version"

ASSET="SirsiPantheon-${VERSION}-arm64.dmg"
printf '%s\n' \
  "pantheon_unsigned_release_source accepted=true" \
  "${source_identity}" \
  "tree=${TREE}" \
  "branch=${BRANCH}" \
  "asset=${ASSET}" \
  "current_cask_version=${CASK_VERSION}" \
  "cask_transition=blocked_until_signed_stapled_uploaded_asset" \
  "owner_gate=Developer_ID_Application_and_notarization_credentials" \
  "lifecycle_gate=clean_M1_M5_install_upgrade_rollback_uninstall_resource_crash_accessibility"
