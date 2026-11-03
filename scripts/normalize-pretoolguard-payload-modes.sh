#!/bin/bash
# Normalize only the two private staged payload copies, never installed hooks.
set -euo pipefail
[[ "$#" == 1 && "$1" = /* && -d "$1" && ! -L "$1" ]] || { echo 'absolute private staging directory required' >&2; exit 2; }
dir=$1
[[ $(/usr/bin/stat -f '%u:%Lp' "$dir") == "$(id -u):700" ]] || { echo 'private owned staging directory required' >&2; exit 1; }
for name in sirsi-display-power-guard sirsi-pretoolguard-install; do
  file="$dir/$name"
  [[ -f "$file" && ! -L "$file" && $(/usr/bin/stat -f '%l' "$file") == 1 ]] || { echo 'missing single-link regular staged payload' >&2; exit 1; }
done
for name in sirsi-display-power-guard sirsi-pretoolguard-install; do
  file="$dir/$name"
  /bin/chmod 0755 "$file"
  [[ $(/usr/bin/stat -f '%Lp' "$file") == 755 ]] || { echo 'staged mode verification failed' >&2; exit 1; }
done
