#!/usr/bin/env bash
# Hermetic regression for the lifecycle evidence schema. It creates a dummy
# package and JSON fixtures only; it never mounts, installs, launches, or
# removes Pantheon.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

repo="$tmp/repo"
mkdir -p "$repo/scripts"
cp "$root/scripts/create-pantheon-lifecycle-plan.sh" "$root/scripts/validate-pantheon-lifecycle-receipt.sh" "$repo/scripts/"
git -C "$repo" init -q
git -C "$repo" config user.email lifecycle-contract@example.invalid
git -C "$repo" config user.name lifecycle-contract-test
printf 'fixture\n' > "$repo/README"
git -C "$repo" add .
git -C "$repo" commit -qm fixture
commit="$(git -C "$repo" rev-parse HEAD)"
tree="$(git -C "$repo" rev-parse 'HEAD^{tree}')"
git -C "$repo" tag -a v0.23.14-beta -m fixture

package="$repo/SirsiPantheon-0.23.14-beta-arm64.dmg"
printf 'fixture package\n' > "$package"
plan="$tmp/plan.json"
(cd "$repo" && bash scripts/create-pantheon-lifecycle-plan.sh --tag v0.23.14-beta --source-commit "$commit" --source-tree "$tree" --package "$package" --host-profile m5 --output "$plan")

sha="$(shasum -a 256 "$package" | awk '{print $1}')"
receipt="$tmp/receipt.json"
jq -n --arg commit "$commit" --arg tree "$tree" --arg sha "$sha" '
  {
    schema: "sirsi.pantheon.lifecycle-receipt.v2", decision: "accepted",
    release: {tag: "v0.23.14-beta", version: "0.23.14-beta", asset: "SirsiPantheon-0.23.14-beta-arm64.dmg"},
    source: {commit: $commit, tree: $tree},
    package: {name: "SirsiPantheon-0.23.14-beta-arm64.dmg", sha256: $sha},
    host: {profile: "m5"},
    phases: {install: "passed", upgrade: "passed", rollback: "passed", uninstall: "passed"},
    resource: {peak_rss_bytes: 0, swap_growth_bytes: 0, process_leak_free: true}
  }
' > "$receipt"

(cd "$repo" && bash scripts/validate-pantheon-lifecycle-receipt.sh --plan "$plan" --receipt "$receipt")
jq '.release.asset = "wrong.dmg"' "$receipt" > "$tmp/wrong.json"
if (cd "$repo" && bash scripts/validate-pantheon-lifecycle-receipt.sh --plan "$plan" --receipt "$tmp/wrong.json") >/dev/null 2>&1; then
  echo 'expected release-asset mismatch to fail' >&2
  exit 1
fi

echo 'validate-pantheon-lifecycle-contract tests: PASS'
