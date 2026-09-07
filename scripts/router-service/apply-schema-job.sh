#!/usr/bin/env bash
# ADR-062 rs-15 step 8 — apply internal/routerstore/pg/{roles,schema}.sql to the private-IP Cloud SQL
# instance from a one-shot Cloud Run job on the VPC. No Mac can reach a private-IP instance, and
# building an image just to carry 22 KB of SQL is waste: the public postgres image supplies psql and
# Cloud Run mounts the SQL from Secret Manager as a file. Idempotent (roles.sql guards CREATE ROLE,
# schema.sql is IF NOT EXISTS). DRY_RUN=1 prints only.
#
# Asserts the same four facts as scripts/check-pg-schema.sh (15 tables, 12 triggers, >=5 partial
# indexes, version 18) and that router_service cannot run DDL — which on Cloud SQL is NOT given:
# every gcloud-created user is a member of cloudsqlsuperuser, so the bundle revokes it.
set -euo pipefail
PROJECT=${PROJECT:-sirsi-nexus-live}; REGION=${REGION:-us-central1}; INSTANCE=${INSTANCE:-sirsi-router}
DB=router; JOB=sirsi-router-apply-schema; CONN="$PROJECT:$REGION:$INSTANCE"
# Runs as the provisioner SA: cloudsql.admin covers the connector, secretmanager.admin the two mounts.
JOB_SA=${JOB_SA:-claude-agent@$PROJECT.iam.gserviceaccount.com}
G="gcloud --project=$PROJECT --quiet"
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
run() { if [ "${DRY_RUN:-0}" = 1 ]; then echo "+ $*"; else echo "+ $*" >&2; "$@"; fi; }
exists() { "$@" >/dev/null 2>&1; }

echo "== 1. SQL bundle -> Secret Manager (sirsi-router-schema-sql)"
bundle=$(mktemp); trap 'rm -f "$bundle"' EXIT
{
  cat "$ROOT/internal/routerstore/pg/roles.sql" "$ROOT/internal/routerstore/pg/schema.sql"
  echo "REVOKE cloudsqlsuperuser FROM router_service;"
} >"$bundle"
if exists $G secrets describe sirsi-router-schema-sql; then
  run $G secrets versions add sirsi-router-schema-sql --data-file="$bundle"
else
  run $G secrets create sirsi-router-schema-sql --replication-policy=automatic --data-file="$bundle"
fi

echo "== 2. Job $JOB (postgres:16-alpine for psql, Cloud SQL connector, secrets mounted)"
read -r -d '' SCRIPT <<'SH' || true
set -e
H=/cloudsql/CONN
psql -v ON_ERROR_STOP=1 -q -h "$H" -U router_migrator -d router -f /sql/apply.sql
q() { psql -qtA -h "$H" -U router_migrator -d router -c "$1"; }
tables=$(q "SELECT count(*) FROM information_schema.tables WHERE table_schema='router' AND table_type='BASE TABLE'")
triggers=$(q "SELECT count(DISTINCT trigger_name) FROM information_schema.triggers WHERE trigger_schema='router'")
partial=$(q "SELECT count(*) FROM pg_indexes WHERE schemaname='router' AND indexdef LIKE '%WHERE%'")
version=$(q "SELECT version FROM router.schema_version")
echo "tables=$tables triggers=$triggers partial=$partial version=$version"
[ "$tables" = 15 ] && [ "$triggers" = 12 ] && [ "$partial" -ge 5 ] && [ "$version" = 18 ] || { echo FAIL-shape; exit 1; }
if PGPASSWORD="$SVCPW" psql -qtA -h "$H" -U router_service -d router -c 'CREATE TABLE router.ddl_probe(i int)' 2>/dev/null; then
  echo "FAIL router_service can DDL"; exit 1
fi
echo "OK router_service is DML-only"
SH
SCRIPT=${SCRIPT//CONN/$CONN}
run $G run jobs deploy "$JOB" --region="$REGION" --image=docker.io/library/postgres:16-alpine \
  --service-account="$JOB_SA" --set-cloudsql-instances="$CONN" \
  --network=default --subnet=default --vpc-egress=private-ranges-only \
  --set-secrets="/sql/apply.sql=sirsi-router-schema-sql:latest,PGPASSWORD=sirsi-router-router-migrator-password:latest,SVCPW=sirsi-router-router-service-password:latest" \
  --command=sh "--args=^@^-c@$SCRIPT" --max-retries=0 --task-timeout=10m --labels=adr=062,workstream=router-service

echo "== 3. Execute and wait"
run $G run jobs execute "$JOB" --region="$REGION" --wait
echo "Done. Logs: gcloud run jobs executions list --job=$JOB --region=$REGION"
