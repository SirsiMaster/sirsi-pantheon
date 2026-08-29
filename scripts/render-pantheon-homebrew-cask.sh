#!/usr/bin/env bash
# Render a cask only after an exact uploaded release asset is visible in the
# supplied release JSON. The generated file can then be committed to the tap.
set -euo pipefail

die() { echo "pantheon_cask_render accepted=false reason=$1" >&2; exit 1; }

TAG=""; COMMIT=""; TREE=""; DMG=""; RELEASE_JSON=""; OUTPUT=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --tag) TAG="$2"; shift 2 ;;
    --source-commit) COMMIT="$2"; shift 2 ;;
    --source-tree) TREE="$2"; shift 2 ;;
    --dmg) DMG="$2"; shift 2 ;;
    --release-json) RELEASE_JSON="$2"; shift 2 ;;
    --output) OUTPUT="$2"; shift 2 ;;
    *) die "unknown_argument" ;;
  esac
done

[[ "$TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]] || die "invalid_tag"
[[ "$COMMIT" =~ ^[0-9a-f]{40}$ && "$TREE" =~ ^[0-9a-f]{40}$ ]] || die "invalid_source_identity"
[[ -f "$DMG" && -f "$RELEASE_JSON" && -n "$OUTPUT" ]] || die "missing_input"
command -v jq >/dev/null || die "missing_jq"

VERSION="${TAG#v}"
ASSET="SirsiPantheon-${VERSION}-arm64.dmg"
[[ "$(basename "$DMG")" == "$ASSET" ]] || die "dmg_name_mismatch"
[[ "$(git rev-parse --verify "refs/tags/${TAG}^{commit}" 2>/dev/null || true)" == "$COMMIT" ]] || die "tag_commit_mismatch"
[[ "$(git rev-parse --verify "${COMMIT}^{tree}" 2>/dev/null || true)" == "$TREE" ]] || die "source_tree_mismatch"
jq -e --arg tag "$TAG" --arg commit "$COMMIT" --arg asset "$ASSET" '
  .tagName == $tag and .targetCommitish == $commit and any(.assets[]?; .name == $asset and (.size | tonumber) > 0)
' "$RELEASE_JSON" >/dev/null || die "release_asset_missing_or_mismatched"

SHA="$(shasum -a 256 "$DMG" | awk '{print $1}')"
cat > "$OUTPUT" <<EOF
cask "sirsi-pantheon" do
  version "${VERSION}"
  sha256 "${SHA}"

  url "https://github.com/SirsiMaster/sirsi-pantheon/releases/download/v#{version}/SirsiPantheon-#{version}-arm64.dmg"
  name "Sirsi Pantheon"
  desc "Mac operational intelligence — native menu bar app + CLI"
  homepage "https://github.com/SirsiMaster/sirsi-pantheon"

  depends_on macos: :monterey

  app "Pantheon.app"
  binary "#{appdir}/Pantheon.app/Contents/MacOS/sirsi"

  uninstall launchctl: "ai.sirsi.pantheon",
            quit:      "ai.sirsi.pantheon"

  zap trash: [
    "~/.config/pantheon",
    "~/Library/LaunchAgents/ai.sirsi.pantheon.plist",
  ]
end
EOF
echo "pantheon_cask_render accepted=true tag=${TAG} asset=${ASSET} sha256=${SHA} output=${OUTPUT}"
