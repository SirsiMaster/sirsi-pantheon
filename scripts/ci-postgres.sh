#!/usr/bin/env bash
# ci-postgres.sh — the Postgres leg of the router store suite (ADR-062 rs-07b).
#
# CLAIM (A35): on this runner, a fresh PostgreSQL 14 cluster accepts
# pg/roles.sql + pg/schema.sql as router_migrator (structure, trigger
# behaviour, router_service least privilege — scripts/check-pg-schema.sh, which
# includes its own negative controls), and the entire internal/routerstore
# suite passes against it under SIRSI_TEST_PG_DSN. Runs on the self-hosted
# macOS runners, which have Homebrew postgresql@14 and no Docker.
#
# Usage: bash scripts/ci-postgres.sh          (start → checks → tests → stop)
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
PORT="${SIRSI_CI_PG_PORT:-54331}"
DATA="${RUNNER_TEMP:-${TMPDIR:-/tmp}}/sirsi-ci-pg-$$"
SOCK=/tmp
export PGHOST=127.0.0.1 PGPORT="$PORT" PGUSER=sirsi

for bin in initdb pg_ctl psql; do
  command -v "$bin" >/dev/null || { echo "SKIP: $bin not on PATH (install postgresql@14); Postgres leg not run" >&2; exit 0; }
done

cleanup() { pg_ctl -D "$DATA" -m fast stop >/dev/null 2>&1 || true; rm -rf "$DATA"; }
trap cleanup EXIT

initdb -D "$DATA" -U sirsi --auth=trust -E UTF8 >/dev/null
pg_ctl -D "$DATA" -o "-p $PORT -k $SOCK -c listen_addresses=127.0.0.1" -l "$DATA/pg.log" start >/dev/null
for i in $(seq 1 30); do psql -d postgres -qtAc 'select 1' >/dev/null 2>&1 && break; sleep 1; done
psql -d postgres -qtAc 'select 1' >/dev/null || { echo "FAIL: postgres did not come up"; cat "$DATA/pg.log"; exit 1; }
echo "postgres $(psql -d postgres -qtAc 'show server_version') on :$PORT"

# 1. schema + behaviour + least-privilege (with its negative controls)
bash "$ROOT/scripts/check-pg-schema.sh"

# 2. the whole routerstore suite on the Postgres driver
psql -d postgres -qtAc "CREATE DATABASE routerstore_test ENCODING 'UTF8' TEMPLATE template0" >/dev/null
( cd "$ROOT" && SIRSI_TEST_PG_DSN="postgres://sirsi@127.0.0.1:$PORT/routerstore_test" go test ./internal/routerstore/ -count=1 )
echo "OK: routerstore suite green on PostgreSQL"
