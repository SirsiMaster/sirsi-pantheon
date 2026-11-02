#!/bin/bash
# Select a fresh private namespace for one Apollo pre-sign digest receipt.
set -euo pipefail

root="${1:-}"
[[ -n "$root" ]] || { echo "usage: $0 OUTPUT_ROOT" >&2; exit 2; }
mkdir -p "$root"
root="$(cd "$root" && pwd -P)"
receipt_dir="$(/usr/bin/mktemp -d "${root%/}/apollo-input-digests.XXXXXX")"
printf '%s\n' "$receipt_dir/pre-sign-digests.txt"
