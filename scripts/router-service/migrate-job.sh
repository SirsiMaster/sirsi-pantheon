#!/usr/bin/env bash
# ADR-062 rs-18 — copy a SQLite ledger snapshot into the Cloud SQL service backend with hash proof.
# `router migrate-store` needs a direct DSN on both sides and only VPC hosts reach the private-IP
# instance, so the snapshot rides inside a throwaway image (service image + one COPY) and runs as a
# one-shot Cloud Run job.
#
#   MODE=report SNAP=~/router-snap.db bash scripts/router-service/migrate-job.sh   # rehearsal: job runs migrate-store --dry-run
#   MODE=import SNAP=...             bash scripts/router-service/migrate-job.sh   # the real import (cut-over)
#   DRY_RUN=1 ...                    bash scripts/router-service/migrate-job.sh   # prints every gcloud command, touches NOTHING
#
# DRY_RUN=1 is read-only on GCP (SSA finding 1, 2026-09-08); MODE=report still builds an image and runs
# a job, because that IS the rehearsal — it only never writes the ledger.
# The snapshot must already be at the binary's schema version (open it once with the same `sirsi`
# build and SIRSI_ALLOW_SCHEMA_MIGRATE=1). Take it with `sqlite3 router.db ".backup <file>"`.
# Source identity: the local sha256 of the snapshot is printed here and again by the job from inside
# the container before the import, so the report is bound to a named file, not "whatever was copied".
set -euo pipefail
PROJECT=${PROJECT:-sirsi-nexus-live}; REGION=${REGION:-us-central1}; INSTANCE=${INSTANCE:-sirsi-router}
SERVICE=${SERVICE:-sirsi-router}; JOB=sirsi-router-migrate; CONN="$PROJECT:$REGION:$INSTANCE"
SNAP=${SNAP:?path to the SQLite snapshot}; MODE=${MODE:-report}
case $MODE in report|import) ;; *) echo "MODE must be report or import" >&2; exit 2;; esac
JOB_SA=${JOB_SA:-claude-agent@$PROJECT.iam.gserviceaccount.com}
G="gcloud --project=$PROJECT --quiet"
run() { echo "+ $*" >&2; [ "${DRY_RUN:-0}" = 1 ] || "$@"; }
TAG="$REGION-docker.pkg.dev/$PROJECT/cloud-run-source-deploy/sirsi-router-migrate:$(date -u +%Y%m%dT%H%M%SZ)"
SNAP_SHA=$(shasum -a 256 "$SNAP" | cut -d' ' -f1)
echo "== snapshot $SNAP sha256 $SNAP_SHA ($MODE${DRY_RUN:+, DRY RUN: nothing is created})"

echo "== 1. Base image = the service's current image"
if [ "${DRY_RUN:-0}" = 1 ]; then BASE="<dry-run:service-image>"; else
  BASE=$($G run services describe "$SERVICE" --region="$REGION" --format='value(spec.template.spec.containers[0].image)'); fi
echo "   $BASE"

echo "== 2. Throwaway image: base + snapshot (never pushed anywhere but this project's registry)"
ctx=$(mktemp -d); trap 'rm -rf "$ctx"' EXIT
cp "$SNAP" "$ctx/src.db"
# alpine, not the distroless service image: the tool pins the source with a write, and a file in an image layer
# copies up to the overlay on first write (new inode → SQLite "readonly database (1544)"). A shell copies the
# snapshot to /tmp first. The static /sirsi binary runs unchanged.
printf 'FROM alpine:3\nCOPY --from=%s /sirsi /sirsi\nCOPY src.db /data/src.db\n' "$BASE" >"$ctx/Dockerfile"
# --async + poll: the provisioner SA cannot read the build log bucket, and a streaming submit exits 1 on a build that succeeds.
if [ "${DRY_RUN:-0}" = 1 ]; then run $G builds submit "$ctx" --tag "$TAG" --async; else
  BUILD=$($G builds submit "$ctx" --tag "$TAG" --async --format='value(id)')
  while :; do st=$($G builds describe "$BUILD" --format='value(status)'); case $st in QUEUED|WORKING|PENDING) sleep 10;; *) break;; esac; done
  [ "$st" = SUCCESS ] || { echo "build $BUILD: $st" >&2; exit 1; }
fi
echo "   $TAG"

# Cleanup receipt (SSA residue): the image holds a ledger copy. Deleting it needs artifactregistry.repoAdmin,
# which the provisioner SA does not hold; the receipt says exactly what was or was not removed.
cleanup() {
  echo "== cleanup"
  if run $G artifacts docker images delete "$TAG" --delete-tags 2>/dev/null; then echo "   image deleted: $TAG"
  else echo "   image NOT deleted (needs artifactregistry.repoAdmin) — owner: gcloud artifacts docker images delete $TAG --delete-tags"; fi
}
trap 'rc=$?; rm -rf "$ctx"; cleanup; exit $rc' EXIT

echo "== 3. Job $JOB ($MODE)"
cmd="cp /data/src.db /tmp/src.db && echo \"source sha256 \$(sha256sum /data/src.db | cut -d' ' -f1)\" && exec /sirsi router migrate-store --from /tmp/src.db --scrub-nul${JSON:+ --json}"
[ "$MODE" = report ] && cmd="$cmd --dry-run"
run $G run jobs deploy "$JOB" --region="$REGION" --image="$TAG" --service-account="$JOB_SA" \
  --set-cloudsql-instances="$CONN" --network=default --subnet=default --vpc-egress=private-ranges-only \
  --set-secrets="SIRSI_ROUTER_STORE=sirsi-router-service-dsn:latest" \
  --command=sh "--args=^@^-c@$cmd" --max-retries=0 --task-timeout=30m --memory=1Gi --labels=adr=062,workstream=router-service >/dev/null
run $G run jobs execute "$JOB" --region="$REGION" --wait
[ "${DRY_RUN:-0}" = 1 ] && exit 0
EXEC=$($G run jobs executions list --job="$JOB" --region="$REGION" --limit=1 --format='value(name)')
echo "== 4. Report (execution $EXEC; Cloud Logging lags ~30 s)"
sleep 30
$G logging read "resource.type=\"cloud_run_job\" AND resource.labels.job_name=\"$JOB\" AND labels.\"run.googleapis.com/execution_name\"=\"$EXEC\"" \
  --limit 1000 --order=asc --format='value(textPayload)' | grep -vE '^\s*$' | tee "$ctx/report.txt"
in_job=$(sed -n 's/^source sha256 \([0-9a-f]*\).*/\1/p' "$ctx/report.txt" | head -1)
[ "$in_job" = "$SNAP_SHA" ] && echo "== source identity bound: container sha256 == local $SNAP_SHA" || { echo "SOURCE IDENTITY MISMATCH: container=$in_job local=$SNAP_SHA" >&2; exit 1; }
