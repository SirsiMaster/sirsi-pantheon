#!/usr/bin/env bash
# Verify the exact release tag/source/package/cask chain after a signed DMG has
# been uploaded. This script is read-only and fails closed on missing assets.
set -euo pipefail

die() { echo "pantheon_release_asset accepted=false reason=$1" >&2; exit 1; }

TAG=""; COMMIT=""; TREE=""; DMG=""; RELEASE_JSON=""; CASK=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --tag) TAG="$2"; shift 2 ;;
    --source-commit) COMMIT="$2"; shift 2 ;;
    --source-tree) TREE="$2"; shift 2 ;;
    --dmg) DMG="$2"; shift 2 ;;
    --release-json) RELEASE_JSON="$2"; shift 2 ;;
    --cask) CASK="$2"; shift 2 ;;
    *) die "unknown_argument" ;;
  esac
done

[[ "$TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]] || die "invalid_tag"
[[ "$COMMIT" =~ ^[0-9a-f]{40}$ ]] || die "invalid_source_commit"
[[ "$TREE" =~ ^[0-9a-f]{40}$ ]] || die "invalid_source_tree"
[[ -f "$DMG" ]] || die "missing_dmg"
[[ -f "$RELEASE_JSON" ]] || die "missing_release_json"
[[ -f "$CASK" ]] || die "missing_cask"
command -v jq >/dev/null || die "missing_jq"

VERSION="${TAG#v}"
ASSET="SirsiPantheon-${VERSION}-arm64.dmg"
[[ "$(basename "$DMG")" == "$ASSET" ]] || die "dmg_name_mismatch"
[[ "$(git rev-parse --verify "refs/tags/${TAG}^{commit}" 2>/dev/null || true)" == "$COMMIT" ]] || die "tag_commit_mismatch"
[[ "$(git rev-parse --verify "${COMMIT}^{tree}" 2>/dev/null || true)" == "$TREE" ]] || die "source_tree_mismatch"

jq -e --arg tag "$TAG" --arg commit "$COMMIT" --arg asset "$ASSET" '
  .tagName == $tag and .targetCommitish == $commit and
  any(.assets[]?; .name == $asset and (.size | tonumber) > 0)
' "$RELEASE_JSON" >/dev/null || die "release_asset_missing_or_mismatched"

SHA="$(shasum -a 256 "$DMG" | awk '{print $1}')"
grep -Fqx "  version \"${VERSION}\"" "$CASK" || die "cask_version_mismatch"
grep -Fqx "  sha256 \"${SHA}\"" "$CASK" || die "cask_sha256_mismatch"
grep -Fqx "  url \"https://github.com/SirsiMaster/sirsi-pantheon/releases/download/v#{version}/SirsiPantheon-#{version}-arm64.dmg\"" "$CASK" || die "cask_url_mismatch"

echo "pantheon_release_asset accepted=true tag=${TAG} commit=${COMMIT} tree=${TREE} asset=${ASSET} sha256=${SHA}"
