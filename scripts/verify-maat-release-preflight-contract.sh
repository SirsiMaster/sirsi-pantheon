#!/usr/bin/env bash
# Static contract for Ma'at's non-executing release-source observer.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
core="$root/internal/maat/releasepreflight.go"
cli="$root/cmd/sirsi/maatpreflight.go"
recipe="$root/contracts/stacklab/maat-system-one-recipe-v1.json"

for file in "$core" "$cli" "$recipe"; do
    [[ -f "$file" ]] || { echo "missing Ma'at release preflight source: $file" >&2; exit 1; }
done

for needle in \
    'func PreflightReleaseContract' \
    'os.Lstat' \
    'os.SameFile' \
    'release-contract:sha256=' \
    'local:maat-release-contract' \
    'does not invoke make, a shell script, a compiler'; do
    /usr/bin/grep -Fq -- "$needle" "$core" || { echo "Ma'at release preflight contract missing: $needle" >&2; exit 1; }
done

if /usr/bin/grep -Eq 'os/exec|exec\.Command|os\.StartProcess' "$core"; then
    echo "Ma'at release preflight must not execute the assessed release surface" >&2
    exit 1
fi

for needle in \
    'sirsi maat preflight release' \
    'PreflightReleaseContract' \
    '--confirm' \
    'newMaatDecisionJournal' \
    'does not authorize a release'; do
    /usr/bin/grep -Fq -- "$needle" "$cli" || { echo "Ma'at release preflight CLI contract missing: $needle" >&2; exit 1; }
done

/usr/bin/jq -e '
  ([.components[] | select(.id == "maat-release-contract-preflight")]) as $components |
  ($components | length == 1) and
  ($components[0].source | index("internal/maat/releasepreflight.go")) != null and
  ($components[0].tests | index("scripts/verify-maat-release-preflight-contract.sh")) != null
' "$recipe" >/dev/null || { echo "Ma'at release preflight Stack Lab component is incomplete" >&2; exit 1; }

echo "Ma'at release preflight contract: pass"
