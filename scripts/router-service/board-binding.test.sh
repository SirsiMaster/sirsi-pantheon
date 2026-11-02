#!/usr/bin/env bash
# board.sh mint refuses unless JOB_SA holds an UNCONDITIONAL secretVersionAdder binding.
set -euo pipefail
B="$(dirname "$0")/board.sh"
SA=mint@sirsi-nexus-live.iam.gserviceaccount.com
M="serviceAccount:$SA"
ok() { printf '%s' "$2" | bash "$B" --has-adder-binding "$SA" || { echo "FAIL: $1 should pass" >&2; exit 1; }; }
no() { if printf '%s' "$2" | bash "$B" --has-adder-binding "$SA"; then echo "FAIL: $1 should be refused" >&2; exit 1; fi; }

ok "unconditional binding" '{"bindings":[{"role":"roles/secretmanager.secretVersionAdder","members":["'"$M"'"]}]}'
no "conditional binding" '{"bindings":[{"role":"roles/secretmanager.secretVersionAdder","members":["'"$M"'"],"condition":{"title":"never","expression":"false"}}]}'
no "other member" '{"bindings":[{"role":"roles/secretmanager.secretVersionAdder","members":["serviceAccount:other@x.iam.gserviceaccount.com"]}]}'
no "other role" '{"bindings":[{"role":"roles/secretmanager.secretAccessor","members":["'"$M"'"]}]}'
no "no bindings" '{}'
echo "board-binding: OK"
