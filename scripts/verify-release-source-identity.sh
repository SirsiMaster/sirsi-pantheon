#!/usr/bin/env bash
set -euo pipefail

TAG_REF="${1:-${GITHUB_REF_NAME:-}}"
if [[ -z "$TAG_REF" || ! "$TAG_REF" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]; then
  echo "release_source_identity accepted=false reason=invalid_tag_ref tag=${TAG_REF:-<empty>}" >&2
  exit 1
fi

VERSION_FILE="$(cd "$(dirname "$0")/.." && pwd)/VERSION"
[[ -f "$VERSION_FILE" ]] || { echo "release_source_identity accepted=false reason=missing_VERSION" >&2; exit 1; }
VERSION="$(tr -d '\r\n' < "$VERSION_FILE")"
EXPECTED_VERSION="${TAG_REF#v}"
if [[ "$VERSION" != "$EXPECTED_VERSION" ]]; then
  echo "release_source_identity accepted=false reason=version_mismatch VERSION=${VERSION} tag_version=${EXPECTED_VERSION}" >&2
  exit 1
fi

HEAD_SHA="$(git rev-parse --verify HEAD^{commit})"
TAG_SHA="$(git rev-parse --verify "refs/tags/${TAG_REF}^{commit}" 2>/dev/null || true)"
if [[ -z "$TAG_SHA" ]]; then
  echo "release_source_identity accepted=false reason=tag_not_present tag=${TAG_REF}" >&2
  exit 1
fi
if [[ "$HEAD_SHA" != "$TAG_SHA" ]]; then
  echo "release_source_identity accepted=false reason=tag_head_mismatch tag_sha=${TAG_SHA} head_sha=${HEAD_SHA}" >&2
  exit 1
fi

echo "release_source_identity accepted=true tag=${TAG_REF} version=${VERSION} commit=${HEAD_SHA}"
