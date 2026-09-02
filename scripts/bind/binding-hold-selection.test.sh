#!/usr/bin/env bash
# Verifies that the legacy GitHub reviewer gate is absent while its required
# status name remains available for branch-protection compatibility.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
WF="$ROOT/.github/workflows/binding-hold.yml"
[ -r "$WF" ] || { echo "✗ missing $WF"; exit 1; }
rg -q '^  binding-hold:' "$WF"
rg -q 'GitHub review gate disabled' "$WF"
! rg -q 'Require an independent bind|BINDERS|pulls/.*/reviews|binding-hold.*label' "$WF"
echo "✔ reviewer-identity gate is absent; compatibility status remains"
