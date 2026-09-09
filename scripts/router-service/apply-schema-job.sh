#!/usr/bin/env bash
# ADR-062 rs-15 step 8 — apply internal/routerstore/pg/{roles,schema}.sql to the private-IP Cloud SQL
# instance from a one-shot Cloud Run job on the VPC. No Mac can reach a private-IP instance, and
# building an image just to carry 22 KB of SQL is waste: the public postgres image supplies psql and
# Cloud Run mounts the SQL from Secret Manager as a file. Idempotent (roles.sql guards CREATE ROLE,
# schema.sql is IF NOT EXISTS). DRY_RUN=1 prints only.
#
# Asserts the same four facts as scripts/check-pg-schema.sh (15 tables, 12 triggers, >=5 partial
# indexes, version 18) and then a CLOSED privilege audit of router_service (SSA finding 3, 2026-09-08):
# no role memberships (Cloud SQL makes every gcloud-created user a cloudsqlsuperuser member — the
# bundle revokes it, and the audit proves the revoke landed), no SUPERUSER/CREATEROLE/CREATEDB, no
# CREATE on schema router or on the database, no default-ACL grants beyond DML, and the executing
# migrator's authority to revoke is checked BEFORE it is exercised (it is itself a cloudsqlsuperuser
# member, which carries CREATEROLE).
#
# Identity: the job runs as its own service account, sirsi-router-schema@…, holding only
# roles/cloudsql.client and secretAccessor on the three secrets it mounts — never the provisioner.
set -euo pipefail
PROJECT=${PROJECT:-sirsi-nexus-live}; REGION=${REGION:-us-central1}; INSTANCE=${INSTANCE:-sirsi-router}
DB=router; JOB=sirsi-router-apply-schema; CONN="$PROJECT:$REGION:$INSTANCE"
JOB_SA_NAME=${JOB_SA_NAME:-sirsi-router-schema}; JOB_SA="$JOB_SA_NAME@$PROJECT.iam.gserviceaccount.com"
G="gcloud --project=$PROJECT --quiet"
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
run() { echo "+ $*" >&2; [ "${DRY_RUN:-0}" = 1 ] || "$@"; }
exists() { "$@" >/dev/null 2>&1; }

echo "== 0. Job identity $JOB_SA (cloudsql.client + secretAccessor on its three secrets, nothing else)"
exists $G iam service-accounts describe "$JOB_SA" || run $G iam service-accounts create "$JOB_SA_NAME" --display-name="router schema job (ADR-062)"
run $G projects add-iam-policy-binding "$PROJECT" --member="serviceAccount:$JOB_SA" --role=roles/cloudsql.client --condition=None >/dev/null
for sec in sirsi-router-schema-sql sirsi-router-router-migrator-password sirsi-router-router-service-password; do
  run $G secrets add-iam-policy-binding "$sec" --member="serviceAccount:$JOB_SA" --role=roles/secretmanager.secretAccessor >/dev/null
done

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
q() { psql -qtA -h "$H" -U router_migrator -d router -c "$1"; }
# Authority receipt BEFORE the bundle: the migrator can only revoke cloudsqlsuperuser membership if it
# holds CREATEROLE (via its own cloudsqlsuperuser membership on Cloud SQL). Refuse rather than "succeed" silently.
auth=$(q "SELECT rolcreaterole OR pg_has_role(current_user,'cloudsqlsuperuser','MEMBER') FROM pg_roles WHERE rolname=current_user")
[ "$auth" = t ] || { echo "FAIL router_migrator cannot revoke role membership (no CREATEROLE / cloudsqlsuperuser)"; exit 1; }
psql -v ON_ERROR_STOP=1 -q -h "$H" -U router_migrator -d router -f /sql/apply.sql
tables=$(q "SELECT count(*) FROM information_schema.tables WHERE table_schema='router' AND table_type='BASE TABLE'")
triggers=$(q "SELECT count(DISTINCT trigger_name) FROM information_schema.triggers WHERE trigger_schema='router'")
partial=$(q "SELECT count(*) FROM pg_indexes WHERE schemaname='router' AND indexdef LIKE '%WHERE%'")
version=$(q "SELECT version FROM router.schema_version")
echo "tables=$tables triggers=$triggers partial=$partial version=$version"
[ "$tables" = 15 ] && [ "$triggers" = 12 ] && [ "$partial" -ge 5 ] && [ "$version" = 18 ] || { echo FAIL-shape; exit 1; }
# Closed privilege audit of router_service: every DDL path, not one probe.
members=$(q "SELECT coalesce(string_agg(b.rolname, ','), '') FROM pg_auth_members m JOIN pg_roles b ON b.oid=m.roleid JOIN pg_roles r ON r.oid=m.member WHERE r.rolname='router_service'")
attrs=$(q "SELECT rolsuper||' '||rolcreaterole||' '||rolcreatedb||' '||rolbypassrls FROM pg_roles WHERE rolname='router_service'")
schema_create=$(q "SELECT has_schema_privilege('router_service','router','CREATE')")
db_create=$(q "SELECT has_database_privilege('router_service','router','CREATE')")
defacl=$(q "SELECT count(*) FROM pg_default_acl d, aclexplode(d.defaclacl) a JOIN pg_roles g ON g.oid=a.grantee WHERE g.rolname='router_service' AND a.privilege_type NOT IN ('SELECT','INSERT','UPDATE','DELETE')")
owned=$(q "SELECT count(*) FROM pg_class c JOIN pg_roles o ON o.oid=c.relowner WHERE o.rolname='router_service'")
echo "router_service: memberships=[$members] super/createrole/createdb/bypassrls=[$attrs] schema.CREATE=$schema_create db.CREATE=$db_create non-DML-default-acl=$defacl owned-objects=$owned"
[ -z "$members" ] && [ "$attrs" = "f f f f" ] && [ "$schema_create" = f ] && [ "$db_create" = f ] && [ "$defacl" = 0 ] && [ "$owned" = 0 ] || { echo "FAIL router_service holds a DDL path"; exit 1; }
if PGPASSWORD="$SVCPW" psql -qtA -h "$H" -U router_service -d router -c 'CREATE TABLE router.ddl_probe(i int)' 2>/dev/null; then
  echo "FAIL router_service can DDL"; exit 1
fi
echo "OK router_service is DML-only (audit closed)"
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
