#!/usr/bin/env bash
# Hermetic regression tests for tagged and pre-tag release identity checks.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

repo="$tmp/repo"
mkdir -p "$repo/scripts"
cp "$root/scripts/verify-release-source-identity.sh" "$repo/scripts/"
printf '0.23.9-beta\n' >"$repo/VERSION"
git -C "$repo" init -q
git -C "$repo" config user.email test@example.invalid
git -C "$repo" config user.name release-identity-test
git -C "$repo" add VERSION scripts/verify-release-source-identity.sh
git -C "$repo" commit -qm fixture
head_sha="$(git -C "$repo" rev-parse HEAD)"

pretag_output="$(cd "$repo" && bash scripts/verify-release-source-identity.sh --pretag v0.23.9-beta "$head_sha")"
grep -Fq 'accepted=true mode=pretag' <<<"$pretag_output"
grep -Fq 'tag_state=absent' <<<"$pretag_output"

git -C "$repo" tag v0.23.9-beta
tagged_output="$(cd "$repo" && bash scripts/verify-release-source-identity.sh v0.23.9-beta)"
grep -Fq 'accepted=true tag=v0.23.9-beta' <<<"$tagged_output"

printf 'next\n' >"$repo/next"
git -C "$repo" add next
git -C "$repo" commit -qm next
if (cd "$repo" && bash scripts/verify-release-source-identity.sh --pretag v0.23.9-beta "$(git -C "$repo" rev-parse HEAD)") >/dev/null 2>&1; then
  echo 'expected mismatched existing tag to fail' >&2
  exit 1
fi

echo 'verify-release-source-identity tests: PASS'
