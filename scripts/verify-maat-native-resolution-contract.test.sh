#!/usr/bin/env bash
# Closed source fixtures only: no browser, server, model, credentials or service.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
fixture=$(mktemp -d "${TMPDIR:-/tmp}/ssa-maat-projection-guard.XXXXXXXX")
trap 'rm -rf "$fixture"' EXIT
paths=$(sed -n 's/^[a-zA-Z][a-zA-Z0-9_]*="\$root\/\([^"]*\)"$/\1/p' "$ROOT/scripts/verify-maat-native-resolution-contract.sh")
refresh() {
  mkdir -p "$fixture/scripts"
  cp "$ROOT/scripts/verify-maat-native-resolution-contract.sh" "$fixture/scripts/"
  while IFS= read -r relative; do
    case "$relative" in ''|/*|*..*) echo 'invalid source fixture path' >&2; exit 1;; esac
    mkdir -p "$fixture/$(dirname "$relative")"
    cp "$ROOT/$relative" "$fixture/$relative"
  done <<< "$paths"
}
refresh
bash "$fixture/scripts/verify-maat-native-resolution-contract.sh" > "$fixture/baseline.log" 2>&1
grep -Fq "Ma'at native resolution contract: pass" "$fixture/baseline.log"
echo 'PASS exact canonical module and mounted/served host'
negative() {
  local label="$1" path="$2" needle="$3" diagnostic="$4"
  refresh
  NEEDLE="$needle" perl -0pi -e 's/\Q$ENV{NEEDLE}\E//g' "$fixture/$path"
  if grep -Fq "$needle" "$fixture/$path"; then echo "mutation failed: $label" >&2; exit 1; fi
  if bash "$fixture/scripts/verify-maat-native-resolution-contract.sh" > "$fixture/negative.log" 2>&1; then
    echo "guard accepted missing $label" >&2; exit 1
  else
    test "$?" = 1
  fi
  grep -Fq "$diagnostic" "$fixture/negative.log"
  echo "PASS rejects missing $label"
}
negative page-load internal/dashboard/pages.go '<script src="/assets/maat-history.js"></script>' 'history mount missing'
negative mount internal/dashboard/pages.go 'window.MaatHistoryUI.mount(T)' 'history mount missing'
negative embed internal/dashboard/ui.go '//go:embed ui/index.html ui/app.css ui/app.js ui/maat-history.js' 'history serving missing'
negative route internal/dashboard/ui.go 'case "/assets/maat-history.js":' 'history serving missing'
negative served-file internal/dashboard/ui.go 's.serveEmbedded(w, "ui/maat-history.js", "application/javascript; charset=utf-8")' 'history serving missing'
negative next-step internal/dashboard/ui/maat-history.js 'prescribed next step' 'recovery guidance projection missing'
negative floor internal/dashboard/ui/maat-history.js 'deterministic floor' 'System One floor projection missing'
negative recovery-level internal/dashboard/ui/maat-history.js 'recovery level' 'three-level recovery projection missing'
echo 'PASS one positive and eight projection/host/serving negatives'
