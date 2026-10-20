#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
source_path="${1:-$root/scripts/release-train.sh}"
verify() {
  if grep -Eq 'launchctl[[:space:]]+(kickstart|bootout|stop|kill)|MAAT_WINDOW_OVERRIDE[[:space:]]*=' "$1"; then
    echo 'release train must not force controller retirement or override a measurement window' >&2
    return 1
  fi
  grep -q 'installation alone is not rollout completion' "$1" || return 1
  grep -q 'loaded-image adoption remains unverified' "$1" || return 1
}
# Exercise the exact former hazards without invoking launchctl or git push.
for unsafe in 'launchctl kickstart -k gui/501/ai.sirsi.router.wake.example' 'MAAT_WINDOW_OVERRIDE=1 git push origin main'; do
  if verify <(printf '%s\n' "$unsafe") 2>/dev/null; then
    echo 'unsafe release control unexpectedly passed its negative control' >&2
    exit 1
  fi
done
verify "$source_path"
echo 'release consumer preservation: PASS'
