#!/usr/bin/env bash
# ADR-062 rs-18 — copy a SQLite ledger snapshot into the Cloud SQL service backend with hash proof.
# `router migrate-store` needs a direct DSN on both sides and only VPC hosts reach the private-IP
# instance, so the snapshot rides inside a throwaway image (service image + one COPY) and runs as a
# one-shot Cloud Run job. Rehearsal = DRY_RUN=1 (report only). The real cut-over run is the same
# command on a snapshot taken AFTER the M5 has stopped writing (rs-19 owner gate).
#
#   SNAP=~/router-snap.db bash scripts/router-service/migrate-job.sh          # real import
#   DRY_RUN=1 SNAP=... bash scripts/router-service/migrate-job.sh             # report only
#
# The snapshot must already be at the binary's schema version (open it once with the same
# `sirsi` build and SIRSI_ALLOW_SCHEMA_MIGRATE=1). Take it with `sqlite3 router.db ".backup <file>"`.
set -euo pipefail
PROJECT=${PROJECT:-sirsi-nexus-live}; REGION=${REGION:-us-central1}; INSTANCE=${INSTANCE:-sirsi-router}
SERVICE=${SERVICE:-sirsi-router}; JOB=sirsi-router-migrate; CONN="$PROJECT:$REGION:$INSTANCE"
SNAP=${SNAP:?path to the SQLite snapshot}
JOB_SA=${JOB_SA:-claude-agent@$PROJECT.iam.gserviceaccount.com}
G="gcloud --project=$PROJECT --quiet"

echo "== 1. Base image = the service's current image"
BASE=$($G run services describe "$SERVICE" --region="$REGION" --format='value(spec.template.spec.containers[0].image)')
echo "   $BASE"

echo "== 2. Throwaway image: base + snapshot (never pushed anywhere but this project's registry)"
TAG="$REGION-docker.pkg.dev/$PROJECT/cloud-run-source-deploy/sirsi-router-migrate:$(date -u +%Y%m%dT%H%M%SZ)"
ctx=$(mktemp -d); trap 'rm -rf "$ctx"' EXIT
cp "$SNAP" "$ctx/src.db"
# alpine, not the distroless service image: the tool pins the source with a write, and a file in an image layer
# copies up to the overlay on first write (new inode → SQLite "readonly database (1544)"). A shell copies the
# snapshot to /tmp first. The static /sirsi binary runs unchanged.
printf 'FROM alpine:3\nCOPY --from=%s /sirsi /sirsi\nCOPY src.db /data/src.db\n' "$BASE" >"$ctx/Dockerfile"
# --async + poll: the provisioner SA cannot read the build log bucket, and a streaming submit exits 1 on a build that succeeds.
BUILD=$($G builds submit "$ctx" --tag "$TAG" --async --format='value(id)')
while :; do st=$($G builds describe "$BUILD" --format='value(status)'); case $st in QUEUED|WORKING|PENDING) sleep 10;; *) break;; esac; done
[ "$st" = SUCCESS ] || { echo "build $BUILD: $st" >&2; exit 1; }
echo "   $TAG"

echo "== 3. Job $JOB (${DRY_RUN:+DRY RUN}${DRY_RUN:-REAL IMPORT})"
cmd="cp /data/src.db /tmp/src.db && exec /sirsi router migrate-store --from /tmp/src.db --scrub-nul --json"
[ "${DRY_RUN:-0}" = 1 ] && cmd="$cmd --dry-run"
$G run jobs deploy "$JOB" --region="$REGION" --image="$TAG" --service-account="$JOB_SA" \
  --set-cloudsql-instances="$CONN" --network=default --subnet=default --vpc-egress=private-ranges-only \
  --set-secrets="SIRSI_ROUTER_STORE=sirsi-router-service-dsn:latest" \
  --command=sh "--args=^@^-c@$cmd" --max-retries=0 --task-timeout=30m --memory=1Gi --labels=adr=062,workstream=router-service >/dev/null
$G run jobs execute "$JOB" --region="$REGION" --wait
EXEC=$($G run jobs executions list --job="$JOB" --region="$REGION" --limit=1 --format='value(name)')
echo "== 4. Report (execution $EXEC; Cloud Logging lags ~30 s)"
sleep 30
$G logging read "resource.type=\"cloud_run_job\" AND resource.labels.job_name=\"$JOB\" AND labels.\"run.googleapis.com/execution_name\"=\"$EXEC\"" \
  --limit 1000 --order=asc --format='value(textPayload)' | grep -vE '^\s*$'
echo "Image $TAG holds a ledger copy — delete it once the report is filed:"
echo "  gcloud artifacts docker images delete $TAG --quiet"
