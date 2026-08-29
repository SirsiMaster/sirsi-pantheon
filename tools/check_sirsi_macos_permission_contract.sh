#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
failed=0

deny() {
  local label="$1" pattern="$2"; shift 2
  local hits
  hits="$(rg -n -i --glob '!node_modules/**' --glob '!dist/**' --glob '!build/**' \
    --glob '!vendor/**' --glob '!Pods/**' --glob '!public/assets/**' \
    --glob '!*.md' --glob '!**/*_test.*' \
    --glob '*.go' --glob '*.swift' --glob '*.m' --glob '*.mm' \
    "$pattern" "$@" 2>/dev/null || true)"
  if [[ -n "$hits" ]]; then
    printf 'permission_contract accepted=false rule=%s\n%s\n' "$label" "$hits" >&2
    failed=1
  fi
}

paths=()
if [[ -f "$ROOT/go.mod" || -d "$ROOT/macapp" ]]; then
  paths+=("$ROOT")
else
  for repo in sirsi-pantheon SirsiNexusApp sirsi-inference sne sirsi-hypergraph sirsi-io; do
    [[ -d "$ROOT/$repo" ]] && paths+=("$ROOT/$repo")
  done
fi

if [[ ${#paths[@]} -eq 0 ]]; then
  printf 'permission_contract accepted=false rule=no_repository_root root=%s\n' "$ROOT" >&2
  exit 1
fi

deny startup_tcc_probe \
  'registerForFullDiskAccess\(|CGRequestScreenCaptureAccess\(|AXIsProcessTrustedWithOptions|requestAccessForEntityType|requestWhen(InUse|Always)Authorization' \
  "${paths[@]}"

deny protected_store_probe \
  '(com\.apple\.TCC/TCC\.db|Library/Messages/chat\.db|Library/Safari/Bookmarks\.plist)' \
  "${paths[@]}"

deny automatic_permission_request \
  '(applicationDidFinishLaunching|init\(|onReady|startup|Start\().{0,120}(requestAuthorization|requestAccess|open\()' \
  "${paths[@]}"

if [[ "$failed" != 0 ]]; then
  exit 1
fi
printf 'permission_contract accepted=true repos=%d root=%s\n' "${#paths[@]}" "$ROOT"
