#!/usr/bin/env bash
# test-apply-schema-upgrade.sh — prove pg/schema.sql upgrades an EXISTING, populated
# ledger in place, exactly as apply-schema-job.sh applies it (one psql transaction,
# ON_ERROR_STOP, as router_migrator). SSA 2026-09-10, PR #724 P1.
#
# CLAIM: on a v18 database with rows in every touched table
#   1. a bundle that fails part-way leaves the database byte-for-byte at v18
#      (no version row change, no new column);
#   2. the real bundle lands v19 with sessions.thread_id and every row intact;
#   3. applying the real bundle again is a no-op that still reports v19.
#
# Needs pg_ctl/initdb/psql on PATH (brew postgresql@16). Runs a throwaway
# cluster in a temp dir on a random port; nothing touches any real database.
set -euo pipefail
export LC_ALL=C   # macOS: the postmaster "becomes multithreaded" under a Unicode locale
ROOT="$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
BASE="${BASE:-main}"                     # the schema the live ledger already has
PGBIN="${PGBIN:-$(dirname "$(command -v pg_ctl)")}"
T="$(mktemp -d)"; PORT=$((20000 + RANDOM % 20000))
cleanup() { "$PGBIN/pg_ctl" -D "$T/data" stop -m immediate -s >/dev/null 2>&1 || true; rm -rf "$T"; }
trap cleanup EXIT
"$PGBIN/initdb" -D "$T/data" -U postgres -A trust >/dev/null
"$PGBIN/pg_ctl" -D "$T/data" -o "-p $PORT -k $T -c listen_addresses=''" -l "$T/pg.log" start -s
export PGHOST="$T" PGPORT="$PORT" PGUSER=postgres PGDATABASE=router
q() { PGOPTIONS='-c search_path=router' psql -qtA -v ON_ERROR_STOP=1 -c "$1"; }   # the service sets search_path=router on its DSN
apply() { PGOPTIONS='-c client_min_messages=warning' psql -1 -q -v ON_ERROR_STOP=1 -U router_migrator -f "$1"; }   # exactly apply-schema-job.sh
psql -qtA -v ON_ERROR_STOP=1 -d postgres -c 'CREATE DATABASE router' >/dev/null
psql -q -v ON_ERROR_STOP=1 -f "$ROOT/internal/routerstore/pg/roles.sql" >/dev/null
q 'ALTER DATABASE router OWNER TO router_migrator' >/dev/null

echo "== 0. the ledger as deployed: $BASE schema, populated"
git -C "$ROOT" show "$BASE:internal/routerstore/pg/schema.sql" > "$T/base.sql"
apply "$T/base.sql" >/dev/null
q "INSERT INTO router.items(id,from_agent,to_agent,title) VALUES ('it-1','a','b','t')" >/dev/null
q "INSERT INTO router.threads(thread_id,agent,status,last_seen_at,payload) VALUES ('thr-1','a','active','2026-09-10T00:00:00Z','{}')" >/dev/null
q "INSERT INTO router.sessions(session_id,secret,host,agent,runtime_hash,created,last_seen) VALUES ('s-1','sec','m1','a','h','2026-09-10T00:00:00Z','2026-09-10T00:00:00Z')" >/dev/null
v0=$(q 'SELECT version FROM router.schema_version'); [ "$v0" = 18 ] || { echo "FAIL base version $v0"; exit 1; }
col() { q "SELECT count(*) FROM information_schema.columns WHERE table_schema='router' AND table_name='sessions' AND column_name='thread_id'"; }
[ "$(col)" = 0 ] || { echo "FAIL base already has thread_id"; exit 1; }
snap() { q "SELECT (SELECT count(*) FROM router.items)||'/'||(SELECT count(*) FROM router.threads)||'/'||(SELECT count(*) FROM router.sessions)||'/'||(SELECT count(*) FROM information_schema.columns WHERE table_schema='router')||'/'||(SELECT count(DISTINCT trigger_name) FROM information_schema.triggers WHERE trigger_schema='router')"; }
s0=$(snap); echo "   v18 rows/columns/triggers = $s0"

echo "== 1. a bundle that fails part-way rolls back completely"
{ cat "$ROOT/internal/routerstore/pg/schema.sql"; echo 'SELECT 1/0;'; } > "$T/broken.sql"
if apply "$T/broken.sql" 2>/dev/null; then echo "FAIL broken bundle applied"; exit 1; fi
[ "$(q 'SELECT version FROM router.schema_version')" = 18 ] || { echo "FAIL version published by a failed apply"; exit 1; }
[ "$(col)" = 0 ] || { echo "FAIL column survived a failed apply"; exit 1; }
[ "$(snap)" = "$s0" ] || { echo "FAIL shape changed after a failed apply: $(snap)"; exit 1; }
echo "   still v18, no thread_id, shape $s0"

echo "== 2. the real bundle upgrades in place"
apply "$ROOT/internal/routerstore/pg/schema.sql" >/dev/null
v1=$(q 'SELECT version FROM router.schema_version'); [ "$v1" = 19 ] || { echo "FAIL upgraded version $v1"; exit 1; }
[ "$(col)" = 1 ] || { echo "FAIL thread_id missing after upgrade"; exit 1; }
rows=$(q "SELECT (SELECT count(*) FROM router.items)||'/'||(SELECT count(*) FROM router.threads)||'/'||(SELECT count(*) FROM router.sessions)")
[ "$rows" = 1/1/1 ] || { echo "FAIL rows lost: $rows"; exit 1; }
[ "$(q "SELECT thread_id FROM router.sessions WHERE session_id='s-1'")" = "" ] || { echo "FAIL legacy session thread_id not blank"; exit 1; }
s1=$(snap); echo "   v19, thread_id present, rows/columns/triggers = $s1"

echo "== 3. applying again is a no-op"
apply "$ROOT/internal/routerstore/pg/schema.sql" >/dev/null
[ "$(q 'SELECT version FROM router.schema_version')" = 19 ] && [ "$(snap)" = "$s1" ] || { echo "FAIL re-apply changed shape"; exit 1; }
echo "   v19, shape unchanged"
echo "PASS: v18 populated → failed apply rolls back → v19 upgrade in place → re-apply no-op"
