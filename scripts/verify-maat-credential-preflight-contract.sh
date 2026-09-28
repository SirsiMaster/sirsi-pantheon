#!/usr/bin/env bash
# Static contract for Ma'at's public-only release credential observer.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
core="$root/internal/maat/credentialpreflight.go"
cli="$root/cmd/sirsi/maatpreflight.go"
native="$root/macapp/Sources/SirsiMenubar/MaatCasebookView.swift"
recipe="$root/contracts/stacklab/maat-system-one-recipe-v1.json"

for file in "$core" "$cli" "$native" "$recipe"; do
    [[ -f "$file" ]] || { echo "missing Ma'at credential preflight surface: $file" >&2; exit 1; }
done

for needle in \
    'PantheonDeveloperTeamID = "9D382WV988"' \
    'exec.Command("/usr/bin/security", "find-identity", "-v")' \
    'Developer ID Application' \
    'Developer ID Installer' \
    'NotarizationObserved: false' \
    'protected release workflow'; do
    /usr/bin/grep -Fq -- "$needle" "$core" || { echo "Ma'at credential core contract missing: $needle" >&2; exit 1; }
done

for needle in \
    'sirsi maat preflight credentials' \
    'PreflightReleaseCredentials' \
    'newMaatDecisionJournal' \
    'private keys,' \
    'keychain passwords, notarization credentials, or contacting Apple'; do
    /usr/bin/grep -Fq -- "$needle" "$cli" || { echo "Ma'at credential CLI contract missing: $needle" >&2; exit 1; }
done

for needle in \
    'Check signing readiness' \
    'Record this release credential readiness check?' \
    'does not read private keys, passwords, notarization material, or contact Apple'; do
    /usr/bin/grep -Fq -- "$needle" "$native" || { echo "Ma'at credential native contract missing: $needle" >&2; exit 1; }
done

/usr/bin/jq -e '
  ([.components[] | select(.id == "maat-release-credential-preflight")]) as $components |
  ($components | length == 1) and
  ($components[0].source | index("internal/maat/credentialpreflight.go")) != null and
  ($components[0].tests | index("scripts/verify-maat-credential-preflight-contract.sh")) != null and
  ($components[0].writes | index("only one confirmed Ma\u0027at decision journal record through sirsi maat preflight credentials --confirm")) != null
' "$recipe" >/dev/null || { echo "Ma'at credential preflight Stack Lab component is incomplete" >&2; exit 1; }

echo "Ma'at credential preflight contract: pass"
