#!/usr/bin/env bash
# ADR-062 rs-16: deploy direct with gcloud (no GitHub Actions, owner 2026-09-02), with the full service config.
# The image is built locally and staged by stage.sh (no-traffic revision, health check, then 100%): Cloud Build
# cannot compile cmd/sirsi because it imports a private module (see stage.sh).
set -euo pipefail
export PROJECT=${PROJECT:-sirsi-nexus-live} REGION=${REGION:-us-central1}
INSTANCE=${INSTANCE:-sirsi-router}
SA_EMAIL="sirsi-router-svc@$PROJECT.iam.gserviceaccount.com"
export VERSION=${VERSION:-$(git describe --tags --always)}
# The service size lives here and nowhere else. A deploy whose size differs from the live
# service is refused: a stale size here once shrank the router to 512Mi and OOMed every
# instance (2026-11-03). To resize on purpose, change SIZE and deploy with RESIZE=1.
SIZE="cpu=2 memory=2Gi concurrency=8 min=3 max=12"
LIVE=$(gcloud --project="$PROJECT" run services describe sirsi-router --region="$REGION" --format=json 2>/dev/null |
  jq -r '.spec.template | "cpu=\(.spec.containers[0].resources.limits.cpu) memory=\(.spec.containers[0].resources.limits.memory) concurrency=\(.spec.containerConcurrency) min=\(.metadata.annotations["autoscaling.knative.dev/minScale"]) max=\(.metadata.annotations["autoscaling.knative.dev/maxScale"])"') || LIVE=""
if [ -n "$LIVE" ] && [ "$LIVE" != "$SIZE" ] && [ "${RESIZE:-0}" != 1 ]; then
  echo "deploy: live size is '$LIVE' but deploy.sh says '$SIZE'; refusing (update SIZE, or RESIZE=1 to resize)" >&2; exit 1
fi
read -r CPU MEM CONC MIN MAX <<<"$(sed 's/[a-z]*=//g' <<<"$SIZE")"
"$(dirname "$0")/stage.sh" \
  --service-account="$SA_EMAIL" --allow-unauthenticated \
  --add-cloudsql-instances="$PROJECT:$REGION:$INSTANCE" \
  --network=default --subnet=default --vpc-egress=private-ranges-only \
  --set-secrets="SIRSI_ROUTER_SERVE_TOKEN=sirsi-router-bootstrap-token:latest,SIRSI_ROUTER_STORE=sirsi-router-service-dsn:latest,SIRSI_ROUTER_GITHUB_TOKEN=ADMIN_GITHUB_PAT:latest" \
  --args="router,serve" \
  --min-instances="$MIN" --max-instances="$MAX" --cpu="$CPU" --memory="$MEM" --concurrency="$CONC" --timeout=90 \
  --labels=adr=062,workstream=router-service
# --allow-unauthenticated: nodes authenticate with per-host bearer tokens inside the service (rs-10/11);
# Cloud Run IAM would require a Google identity on every Mac, which the design rejects.
URL=$(gcloud --project="$PROJECT" run services describe sirsi-router --region="$REGION" --format="value(status.url)")
IMAGE=$(gcloud --project="$PROJECT" run services describe sirsi-router --region="$REGION" --format="value(spec.template.spec.containers[0].image)")
DIGEST=$(gcloud --project="$PROJECT" run services describe sirsi-router --region="$REGION" --format="value(status.latestReadyRevisionName,spec.template.spec.containers[0].image)")
echo "URL=$URL"; echo "REVISION/IMAGE=$DIGEST"
# Pin the token-mint job to the same image as the service on every deploy: the
# job is deployed separately (cutover-m5.sh only updates its --args), so left
# alone it drifts behind the service schema/binary (claude-io, 2026-09-15:
# "postgres ledger schema_version 22, this binary expects 18" on a stale job image).
if gcloud --project="$PROJECT" run jobs describe sirsi-router-token --region="$REGION" >/dev/null 2>&1; then
  gcloud --project="$PROJECT" run jobs update sirsi-router-token --region="$REGION" --image="$IMAGE" >/dev/null
  echo "sirsi-router-token job pinned to $IMAGE"
fi
echo "SPKI pin (release manifest):"; echo | openssl s_client -connect "${URL#https://}:443" -servername "${URL#https://}" 2>/dev/null | openssl x509 -pubkey -noout | openssl pkey -pubin -outform der | openssl dgst -sha256 -binary | base64
