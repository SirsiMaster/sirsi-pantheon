#!/usr/bin/env bash
# Asserts scripts/ci-postgres.sh FAILS CLOSED when its PostgreSQL
# prerequisites (initdb/pg_ctl/psql) are missing from PATH, instead of the
# prior silent `exit 0` that let CI report green without ever running the
# PostgreSQL leg (ADR-062 rs-07b, A35 — a required check must fail when its
# prerequisite is absent, not quietly no-op).
#
# No real PostgreSQL needed: this only exercises the missing-tool guard, not
# a live cluster (the real cluster path is exercised by the CI step itself).
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fails=0

check_missing_tools_fail_closed() {
  local desc="only coreutils on PATH (no initdb/pg_ctl/psql)"
  local out rc
  out="$(PATH=/usr/bin:/bin bash "$ROOT/scripts/ci-postgres.sh" 2>&1)"
  rc=$?
  if [ "$rc" -ne 1 ]; then
    echo "FAIL — $desc: exit code = $rc, want 1"
    fails=$((fails + 1))
    return
  fi
  if ! grep -q "::error" <<<"$out"; then
    echo "FAIL — $desc: no ::error annotation in output: $out"
    fails=$((fails + 1))
    return
  fi
  if grep -q "^SKIP:" <<<"$out"; then
    echo "FAIL — $desc: the old silent-skip marker is still present: $out"
    fails=$((fails + 1))
    return
  fi
  echo "ok   — $desc: exit 1 with ::error::, no silent SKIP"
}

check_missing_tools_fail_closed

[ "$fails" = 0 ] && echo "all ci-postgres.sh fail-closed checks pass" || echo "$fails check(s) failed"
exit "$fails"
