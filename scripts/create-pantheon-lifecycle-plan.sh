#!/usr/bin/env bash
# Create a deterministic, non-mutating M1/M5 lifecycle qualification plan.
set -euo pipefail

die() { echo "pantheon_lifecycle_plan accepted=false reason=$1" >&2; exit 1; }

TAG=""; COMMIT=""; TREE=""; PACKAGE=""; PROFILE=""; OUTPUT=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --tag) TAG="$2"; shift 2 ;;
    --source-commit) COMMIT="$2"; shift 2 ;;
    --source-tree) TREE="$2"; shift 2 ;;
    --package) PACKAGE="$2"; shift 2 ;;
    --host-profile) PROFILE="$2"; shift 2 ;;
    --output) OUTPUT="$2"; shift 2 ;;
    *) die "unknown_argument" ;;
  esac
done

[[ "$TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]] || die "invalid_tag"
[[ "$COMMIT" =~ ^[0-9a-f]{40}$ ]] || die "invalid_source_commit"
[[ "$TREE" =~ ^[0-9a-f]{40}$ ]] || die "invalid_source_tree"
[[ -f "$PACKAGE" ]] || die "missing_package"
[[ "$PROFILE" == "m1" || "$PROFILE" == "m5" ]] || die "invalid_host_profile"
[[ -n "$OUTPUT" ]] || die "missing_output"
command -v jq >/dev/null || die "missing_jq"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
[[ "$(git rev-parse --verify "refs/tags/${TAG}^{commit}" 2>/dev/null || true)" == "$COMMIT" ]] || die "tag_commit_mismatch"
[[ "$(git rev-parse --verify "${COMMIT}^{tree}" 2>/dev/null || true)" == "$TREE" ]] || die "source_tree_mismatch"

VERSION="${TAG#v}"
ASSET="SirsiPantheon-${VERSION}-arm64.dmg"
[[ "$(basename "$PACKAGE")" == "$ASSET" ]] || die "package_name_mismatch"
SHA="$(shasum -a 256 "$PACKAGE" | awk '{print $1}')"
jq -n --arg tag "$TAG" --arg version "$VERSION" --arg asset "$ASSET" --arg commit "$COMMIT" --arg tree "$TREE" --arg package "$(basename "$PACKAGE")" --arg sha "$SHA" --arg profile "$PROFILE" '
  {
    schema: "sirsi.pantheon.lifecycle-plan.v2",
    mutation: false,
    release: {tag: $tag, version: $version, asset: $asset},
    source: {commit: $commit, tree: $tree},
    package: {name: $package, sha256: $sha},
    host: {profile: $profile},
    required_phases: ["install", "upgrade", "rollback", "uninstall"],
    required_resource_evidence: ["peak_rss_bytes", "swap_growth_bytes", "process_leak_free"],
    containment: "This plan creates no mount, install, launchd, Tailscale, security, or runtime change."
  }
' > "$OUTPUT"
echo "pantheon_lifecycle_plan accepted=true tag=${TAG} profile=${PROFILE} package_sha256=${SHA} output=${OUTPUT}"
