#!/usr/bin/env bash
# check-pg-schema.sh — apply the Postgres router schema to a fresh database and
# prove it behaves (ADR-062 rs-05).
#
# CLAIM (A35): internal/routerstore/pg/{roles,schema}.sql apply cleanly as
# router_migrator on an empty database; every table, partial unique index and
# trigger from SQLite schema v16 exists; a wake_event is emitted in the same
# transaction as an item insert; router_service cannot run DDL.
#
# Needs a reachable Postgres. Connection comes from PGHOST/PGPORT/PGUSER (a
# superuser or CREATEDB+CREATEROLE role), e.g. the scratch server:
#   PGHOST=127.0.0.1 PGPORT=54329 PGUSER=sirsi bash scripts/check-pg-schema.sh
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
DB="routerstore_check_$$"
export PGHOST="${PGHOST:-127.0.0.1}" PGPORT="${PGPORT:-5432}" PGUSER="${PGUSER:-postgres}"
psqlq() { psql -v ON_ERROR_STOP=1 -qtA "$@"; }

cleanup() { psqlq -d postgres -c "DROP DATABASE IF EXISTS $DB;" >/dev/null 2>&1 || true; }
trap cleanup EXIT

psqlq -d postgres -c "CREATE DATABASE $DB;"
psqlq -d "$DB" -f "$ROOT/internal/routerstore/pg/roles.sql" >/dev/null
psqlq -d "$DB" -c "ALTER DATABASE $DB OWNER TO router_migrator;" >/dev/null
# DDL as the migrator, exactly as production will.
PGUSER=router_migrator psqlq -d "$DB" -f "$ROOT/internal/routerstore/pg/schema.sql" >/dev/null

tables=$(psqlq -d "$DB" -c "SELECT count(*) FROM information_schema.tables WHERE table_schema='router' AND table_type='BASE TABLE';")
triggers=$(psqlq -d "$DB" -c "SELECT count(*) FROM information_schema.triggers WHERE trigger_schema='router';")
partial=$(psqlq -d "$DB" -c "SELECT count(*) FROM pg_indexes WHERE schemaname='router' AND indexdef LIKE '%WHERE%';")
version=$(psqlq -d "$DB" -c "SELECT version FROM router.schema_version;")
# information_schema.triggers lists one row per (trigger, event); count distinct names.
trigger_names=$(psqlq -d "$DB" -c "SELECT count(DISTINCT trigger_name) FROM information_schema.triggers WHERE trigger_schema='router';")

[ "$tables" = 12 ]        || { echo "FAIL: expected 12 tables (11 + schema_version), got $tables"; exit 1; }
[ "$trigger_names" = 12 ] || { echo "FAIL: expected 12 triggers, got $trigger_names"; exit 1; }
[ "$partial" -ge 5 ]      || { echo "FAIL: expected >=5 partial indexes, got $partial"; exit 1; }
[ "$version" = 16 ]       || { echo "FAIL: schema_version should pair with SQLite v16, got $version"; exit 1; }

# Behaviour: an open item emits exactly one wake event (trigger), as
# router_service, and a duplicate event_key is ignored (ON CONFLICT DO NOTHING).
got=$(PGUSER=router_service psqlq -d "$DB" <<'SQL'
SET search_path = router;
INSERT INTO items(id,from_agent,to_agent,title,opened) VALUES('it-1','a','b','t','2026-01-01T00:00:00Z');
INSERT INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,created,updated)
  VALUES('dup','item:create:it-1','b','router_item','it-1','dup',router.now_rfc3339(),router.now_rfc3339())
  ON CONFLICT DO NOTHING;
SELECT count(*) FROM wake_events WHERE source_id='it-1' AND event_key='item:create:it-1';
SQL
)
[ "$got" = 1 ] || { echo "FAIL: item insert did not emit exactly one wake event (got $got)"; exit 1; }

# Claim acks the leased wake event (ack_wake_on_item_claim).
got=$(PGUSER=router_service psqlq -d "$DB" <<'SQL'
SET search_path = router;
INSERT INTO items(id,from_agent,to_agent,title,opened) VALUES('it-2','a','b','t','2026-01-01T00:00:00Z');
UPDATE wake_events SET status='leased' WHERE source_id='it-2';
UPDATE items SET lease_token='tok' WHERE id='it-2';
SELECT status||':'||ack_ref FROM wake_events WHERE source_id='it-2';
SQL
)
[ "$got" = "acked:router-lease:it-2:tok" ] || { echo "FAIL: claim did not ack the wake event (got '$got')"; exit 1; }

# Least privilege: the service role cannot run DDL in the router schema.
if PGUSER=router_service psql -qtA -d "$DB" -c "CREATE TABLE router.should_fail(x int);" >/dev/null 2>&1; then
  echo "FAIL: router_service was able to CREATE TABLE — least privilege is not enforced"; exit 1
fi

echo "OK: pg schema applies as router_migrator; $tables tables, $trigger_names triggers, $partial partial indexes, version $version; wake trigger + claim ack behave; router_service is DML-only."
