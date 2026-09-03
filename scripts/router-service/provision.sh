#!/usr/bin/env bash
# ADR-062 rs-15 — provision the router service in GCP. Idempotent: every step checks before it creates.
# Owner decisions 2026-09-03 (rs-14 "1a 2a 3a"): Cloud SQL Postgres smallest tier, private IP, backups;
# bootstrap token in Secret Manager only; dedicated least-privilege service account.
# Run as an account with Owner on the project (goal doc: sirsimaster@gmail.com). DRY_RUN=1 prints only.
set -euo pipefail
PROJECT=${PROJECT:-sirsi-nexus-live}
REGION=${REGION:-us-central1}
INSTANCE=${INSTANCE:-sirsi-router}
DB=router
SA=sirsi-router-svc
SA_EMAIL="$SA@$PROJECT.iam.gserviceaccount.com"
NETWORK=${NETWORK:-default}
G="gcloud --project=$PROJECT --quiet"
run() { if [ "${DRY_RUN:-0}" = 1 ]; then echo "+ $*"; else echo "+ $*" >&2; "$@"; fi; }
exists() { "$@" >/dev/null 2>&1; }
secret_put() { # name, value-from-stdin
  if exists $G secrets describe "$1"; then run $G secrets versions add "$1" --data-file=-; else run $G secrets create "$1" --replication-policy=automatic --data-file=-; fi
}

echo "== 1. APIs"
run $G services enable sqladmin.googleapis.com run.googleapis.com secretmanager.googleapis.com \
  servicenetworking.googleapis.com compute.googleapis.com artifactregistry.googleapis.com cloudbuild.googleapis.com iam.googleapis.com

echo "== 2. Private services access on $NETWORK (needed for a private-IP Cloud SQL instance)"
if ! exists $G compute addresses describe google-managed-services-$NETWORK --global; then
  run $G compute addresses create google-managed-services-$NETWORK --global --purpose=VPC_PEERING --prefix-length=16 --network=$NETWORK
fi
run $G services vpc-peerings connect --service=servicenetworking.googleapis.com --ranges=google-managed-services-$NETWORK --network=$NETWORK || true

echo "== 3. Cloud SQL $INSTANCE (POSTGRES_16, db-f1-micro, zonal, private IP only, daily backups, PITR)"
if ! exists $G sql instances describe $INSTANCE; then
  run $G sql instances create $INSTANCE --database-version=POSTGRES_16 --tier=db-f1-micro --region=$REGION \
    --availability-type=zonal --storage-size=10GB --storage-auto-increase \
    --backup-start-time=08:00 --enable-point-in-time-recovery --retained-backups-count=7 \
    --network=projects/$PROJECT/global/networks/$NETWORK --no-assign-ip --deletion-protection
fi
exists $G sql databases describe $DB --instance=$INSTANCE || run $G sql databases create $DB --instance=$INSTANCE

echo "== 4. Roles + passwords (pg/roles.sql shape: router_migrator owns DDL, router_service is DML-only)"
for role in router_migrator router_service; do
  secret="sirsi-router-${role//_/-}-password"
  if ! exists $G secrets describe "$secret"; then
    pw=$(openssl rand -base64 33 | tr -d '/+=' | cut -c1-40)
    printf '%s' "$pw" | secret_put "$secret"
  fi
  pw=$($G secrets versions access latest --secret="$secret")
  if exists $G sql users describe $role --instance=$INSTANCE; then
    run $G sql users set-password $role --instance=$INSTANCE --password="$pw"
  else
    run $G sql users create $role --instance=$INSTANCE --password="$pw"
  fi
done

echo "== 5. Bootstrap token (Secret Manager only; never installed on a node — runbook)"
if ! exists $G secrets describe sirsi-router-bootstrap-token; then
  openssl rand -hex 32 | tr -d '\n' | secret_put sirsi-router-bootstrap-token
fi

echo "== 6. Service DSN secret (unix socket via the Cloud Run Cloud SQL connector)"
svcpw=$($G secrets versions access latest --secret=sirsi-router-router-service-password)
printf 'postgres://router_service:%s@/%s?host=/cloudsql/%s:%s:%s' "$svcpw" "$DB" "$PROJECT" "$REGION" "$INSTANCE" | secret_put sirsi-router-service-dsn

echo "== 7. Service account $SA_EMAIL — cloudsql.client + accessor on exactly two secrets + logs"
exists $G iam service-accounts describe "$SA_EMAIL" || run $G iam service-accounts create $SA --display-name="sirsi router serve (ADR-062)"
run $G projects add-iam-policy-binding $PROJECT --member="serviceAccount:$SA_EMAIL" --role=roles/cloudsql.client >/dev/null
run $G projects add-iam-policy-binding $PROJECT --member="serviceAccount:$SA_EMAIL" --role=roles/logging.logWriter >/dev/null
for s in sirsi-router-bootstrap-token sirsi-router-service-dsn; do
  run $G secrets add-iam-policy-binding $s --member="serviceAccount:$SA_EMAIL" --role=roles/secretmanager.secretAccessor >/dev/null
done

cat <<MSG
== 8. Schema (one-off, as router_migrator): the instance has no public IP, so apply from a host on the VPC or
   via the Cloud SQL Auth Proxy with --private-ip from a VPC-attached machine:
     cloud-sql-proxy --private-ip $PROJECT:$REGION:$INSTANCE --port 54329 &
     PGPASSWORD=\$(gcloud secrets versions access latest --secret=sirsi-router-router-migrator-password --project=$PROJECT) \\
       psql -h 127.0.0.1 -p 54329 -U router_migrator -d $DB -f internal/routerstore/pg/roles.sql -f internal/routerstore/pg/schema.sql
   Alternative without a VPC host: scripts/router-service/apply-schema-job.sh (Cloud Run job on the VPC).
Done. Next: scripts/router-service/deploy.sh (rs-16).
MSG
