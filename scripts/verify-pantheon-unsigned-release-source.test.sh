#!/usr/bin/env bash
# Hermetic contract regression for the unsigned source gate. It creates no
# package, cask, tag, credential, or installed-host state.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

repo="$tmp/repo"
mkdir -p "$repo/scripts" "$repo/homebrew/Casks"
cp "$root/scripts/verify-release-source-identity.sh" "$repo/scripts/"
cp "$root/scripts/verify-pantheon-unsigned-release-source.sh" "$repo/scripts/"
for file in build-dmg.sh verify-pantheon-package-identity.sh render-pantheon-homebrew-cask.sh verify-pantheon-release-asset.sh; do
  printf '#!/usr/bin/env bash\n' > "$repo/scripts/$file"
done
printf '0.23.14-beta\n' > "$repo/VERSION"
cat > "$repo/homebrew/Casks/sirsi-pantheon.rb" <<'EOF'
cask "sirsi-pantheon" do
  version "0.23.8-beta"
end
EOF
git -C "$repo" init -q
git -C "$repo" config user.email test@example.invalid
git -C "$repo" config user.name unsigned-release-source-test
git -C "$repo" add .
git -C "$repo" commit -qm fixture
head_sha="$(git -C "$repo" rev-parse HEAD)"

output="$(cd "$repo" && bash scripts/verify-pantheon-unsigned-release-source.sh --pretag v0.23.14-beta "$head_sha")"
grep -Fq 'accepted=true' <<<"$output"
grep -Fq 'cask_transition=blocked_until_signed_stapled_uploaded_asset' <<<"$output"

sed -i.bak 's/0.23.8-beta/0.23.14-beta/' "$repo/homebrew/Casks/sirsi-pantheon.rb"
if (cd "$repo" && bash scripts/verify-pantheon-unsigned-release-source.sh --pretag v0.23.14-beta "$head_sha") >/dev/null 2>&1; then
  echo 'expected unsigned cask claim to fail' >&2
  exit 1
fi

echo 'verify-pantheon-unsigned-release-source tests: PASS'
