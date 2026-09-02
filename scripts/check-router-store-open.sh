#!/usr/bin/env bash
# check-router-store-open.sh — the direct-open gate (ADR-062 §1, rs-04).
#
# CLAIM (scoped, A35): no non-test Go file outside internal/routerstore opens
# the router ledger except through routerstore.Resolve(); the read-only path
# resolver LocalPath() is used only by the two local-only diagnostics listed
# in ALLOW_LOCALPATH. Anything else is a split-brain path: a node pointed at
# the router service would write a local file behind the service's back.
#
# Runs in CI (lint job) and in the Ma'at pre-push gate. `--self-test` plants a
# violation in a temp copy and expects this script to go red — a guard that has
# never been shown red is an untested guard.
set -euo pipefail

SELF_TEST=0
ROOT=""
for a in "$@"; do
  case "$a" in
    --self-test) SELF_TEST=1 ;;
    *) ROOT="$a" ;;
  esac
done
ROOT="${ROOT:-$(git rev-parse --show-toplevel)}"
SCRIPT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/$(basename "${BASH_SOURCE[0]}")"
cd "$ROOT"

# Constructors that bypass Resolve(). OpenPath is for tests and Resolve only;
# OpenReadOnly is a read surface primitive with no production caller today.
FORBIDDEN='routerstore\.(OpenPath|OpenReadOnly|Open|DefaultStorePath)\('
# LocalPath is read-only by contract; only these files may call it.
ALLOW_LOCALPATH='^(cmd/sirsi/schemacheckcmd\.go|cmd/sirsi/selfupdate\.go):'

scan() {
  local rc=0
  # grep -E -n: file:line:text; exclude tests and the package itself.
  local hits
  hits=$(grep -rEn --include='*.go' --exclude='*_test.go' --exclude-dir=.git "$FORBIDDEN" . \
         | grep -v '^\./internal/routerstore/' || true)
  if [ -n "$hits" ]; then
    echo "ERROR: router store opened outside routerstore.Resolve() (ADR-062 §1):"
    echo "$hits" | sed 's/^/  /'
    rc=1
  fi
  local lp
  lp=$(grep -rEn --include='*.go' --exclude='*_test.go' --exclude-dir=.git 'routerstore\.LocalPath\(' . \
       | grep -v '^\./internal/routerstore/' | sed 's#^\./##' | grep -vE "$ALLOW_LOCALPATH" || true)
  if [ -n "$lp" ]; then
    echo "ERROR: routerstore.LocalPath() is read-only diagnostics only; not allowed here:"
    echo "$lp" | sed 's/^/  /'
    rc=1
  fi
  return $rc
}

if [ "$SELF_TEST" = 1 ]; then
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT
  # Copy only what the scan reads: Go sources under a couple of dirs is enough.
  mkdir -p "$tmp/cmd/planted" "$tmp/internal/routerstore"
  printf 'package planted\n\nimport "x/routerstore"\n\nfunc f() { _, _ = routerstore.OpenPath("db") }\n' > "$tmp/cmd/planted/p.go"
  if bash "$SCRIPT" "$tmp" >/dev/null 2>&1; then
    echo "SELF-TEST FAIL: planted direct open was NOT detected"; exit 1
  fi
  echo "self-test OK: planted routerstore.OpenPath() call is detected"
  exit 0
fi

if ! scan; then
  exit 1
fi
echo "OK: every production store open goes through routerstore.Resolve(); LocalPath confined to local diagnostics."
