#!/usr/bin/env bash
# Hermetic release proof for the split-brain repair.
# It deliberately runs without service configuration and proves that Pantheon
# refuses to create/open an implicit ~/.sirsi/router.db. It then runs the
# router package contract suite. No live service, database, or credentials are
# touched.
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
home="$tmp/home"; mkdir -p "$home"
gomodcache=$(go env GOMODCACHE)
gocache=$(go env GOCACHE)
set +e
out=$(cd "$root" && env -u SIRSI_ROUTER_URL -u SIRSI_ROUTER_DB HOME="$home" GOMODCACHE="$gomodcache" GOCACHE="$gocache" go run ./cmd/sirsi router status 2>&1)
rc=$?
set -e
[ "$rc" -ne 0 ] || { echo "FAIL: missing service configuration was accepted" >&2; exit 1; }
printf '%s\n' "$out" | grep -Eiq 'router|service|configured|ledger' || {
  echo "FAIL: refusal did not identify the missing router authority" >&2
  printf '%s\n' "$out" >&2
  exit 1
}
[ ! -e "$home/.sirsi/router.db" ] || { echo "FAIL: refusal created an implicit local ledger" >&2; find "$home/.sirsi" -print >&2; exit 1; }

(cd "$root" && go test ./internal/routerstore -run '^TestRemote' -count=1)
(cd "$root" && go test ./internal/routerstore ./internal/dispatch ./cmd/sirsi)
echo "PASS: no implicit local ledger; remote service round-trip and routerstore/dispatch/CLI contract suites pass"
