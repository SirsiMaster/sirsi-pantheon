#!/usr/bin/env bash
set -euo pipefail

MODE="tagged"
if [[ "${1:-}" == "--pretag" ]]; then
  MODE="pretag"
  shift
fi

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
if [[ "$MODE" == "pretag" ]]; then
  TARGET_REF="${2:-}"
  if [[ ! "$TARGET_REF" =~ ^[0-9a-fA-F]{40}$ ]]; then
    echo "release_source_identity accepted=false reason=invalid_pretag_commit" >&2
    exit 1
  fi
  TARGET_SHA="$(git rev-parse --verify "${TARGET_REF}^{commit}" 2>/dev/null || true)"
  if [[ -z "$TARGET_SHA" ]]; then
    echo "release_source_identity accepted=false reason=missing_pretag_commit" >&2
    exit 1
  fi
  if [[ "$HEAD_SHA" != "$TARGET_SHA" ]]; then
    echo "release_source_identity accepted=false reason=pretag_head_mismatch target_sha=${TARGET_SHA} head_sha=${HEAD_SHA}" >&2
    exit 1
  fi

  EXISTING_TAG_SHA="$(git rev-parse --verify "refs/tags/${TAG_REF}^{commit}" 2>/dev/null || true)"
  if [[ -n "$EXISTING_TAG_SHA" && "$EXISTING_TAG_SHA" != "$TARGET_SHA" ]]; then
    echo "release_source_identity accepted=false reason=pretag_tag_target_mismatch tag_sha=${EXISTING_TAG_SHA} target_sha=${TARGET_SHA}" >&2
    exit 1
  fi

  TAG_STATE="absent"
  [[ -n "$EXISTING_TAG_SHA" ]] && TAG_STATE="already-bound"
  echo "release_source_identity accepted=true mode=pretag tag=${TAG_REF} version=${VERSION} commit=${TARGET_SHA} tag_state=${TAG_STATE}"
  exit 0
fi

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
