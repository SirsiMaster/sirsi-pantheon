#!/bin/bash
# Build an unsigned standalone guard for the existing signing custody workflow.
# This does not sign, publish, install, or modify any hook.
set -euo pipefail

if [ "$#" -ne 2 ]; then
  echo "usage: bash scripts/build-pretoolguard-artifact.sh <exact-commit> <existing-output-directory>" >&2
  exit 2
fi
expected=$1
output=$2
[[ "$expected" =~ ^[0-9a-f]{40}$ ]] || { echo "expected a full lowercase commit ID" >&2; exit 2; }
[[ "$output" = /* && -d "$output" && ! -L "$output" ]] || { echo "output must be an existing absolute non-symlink directory" >&2; exit 2; }
root=$(git rev-parse --show-toplevel)
cd "$root"
actual=$(git rev-parse 'HEAD^{commit}')
[[ "$actual" == "$expected" ]] || { echo "HEAD differs from expected commit" >&2; exit 1; }
[[ -z "$(git status --porcelain --untracked-files=normal)" ]] || { echo "worktree is not clean" >&2; exit 1; }
[[ $(uname -s) == Darwin && $(uname -m) == arm64 ]] || { echo "native Darwin arm64 required" >&2; exit 1; }
dir=$(mktemp -d "$output/pretoolguard.XXXXXXXX")
echo "Retaining build directory: $dir"
printf '%s\n' 'INCOMPLETE: do not use; inspect retained evidence' > "$dir/INCOMPLETE"
# A fresh output directory is retained even on failure for custody/audit.
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 GOWORK=off \
  go build -trimpath -o "$dir/sirsi-display-power-guard" ./cmd/sirsi-pretoolguard
# The installer needs the supported libSystem ACL bridge; a no-cgo build
# intentionally refuses metadata inspection and is not an install artifact.
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 GOWORK=off \
  go build -trimpath -o "$dir/sirsi-pretoolguard-install" ./cmd/sirsi-pretoolguard-install
printf 'source_commit=%s\n' "$actual" > "$dir/source.txt"
shasum -a 256 "$dir/sirsi-display-power-guard" > "$dir/unsigned.sha256"
shasum -a 256 "$dir/sirsi-pretoolguard-install" > "$dir/installer-unsigned.sha256"
printf '%s\n' 'ACCEPTED: local unsigned build checks completed' > "$dir/INCOMPLETE"
mv "$dir/INCOMPLETE" "$dir/ACCEPTED"
echo "Unsigned artifact only; signing and installation are separate controlled operations."
