#!/usr/bin/env bash
# ADR-062 rs-16 — first deploy, direct with gcloud (no GitHub Actions — owner 2026-09-02).
set -euo pipefail
PROJECT=${PROJECT:-sirsi-nexus-live}; REGION=${REGION:-us-central1}; INSTANCE=${INSTANCE:-sirsi-router}
SA_EMAIL="sirsi-router-svc@$PROJECT.iam.gserviceaccount.com"
VERSION=${VERSION:-$(git describe --tags --always)}
gcloud --project="$PROJECT" run deploy sirsi-router --source . --region="$REGION" \
  --service-account="$SA_EMAIL" --allow-unauthenticated \
  --add-cloudsql-instances="$PROJECT:$REGION:$INSTANCE" \
  --network=default --subnet=default --vpc-egress=private-ranges-only \
  --set-secrets="SIRSI_ROUTER_SERVE_TOKEN=sirsi-router-bootstrap-token:latest,SIRSI_ROUTER_STORE=sirsi-router-service-dsn:latest" \
  --args="router,serve,--store,\$(SIRSI_ROUTER_STORE)" \
  --min-instances=0 --max-instances=2 --cpu=1 --memory=512Mi --concurrency=80 --timeout=90 \
  --labels=adr=062,workstream=router-service
# --allow-unauthenticated: nodes authenticate with per-host bearer tokens inside the service (rs-10/11);
# Cloud Run IAM would require a Google identity on every Mac, which the design rejects.
URL=$(gcloud --project="$PROJECT" run services describe sirsi-router --region="$REGION" --format='value(status.url)')
DIGEST=$(gcloud --project="$PROJECT" run services describe sirsi-router --region="$REGION" --format='value(status.latestReadyRevisionName,spec.template.spec.containers[0].image)')
echo "URL=$URL"; echo "REVISION/IMAGE=$DIGEST"
echo "SPKI pin (release manifest):"; echo | openssl s_client -connect "${URL#https://}:443" -servername "${URL#https://}" 2>/dev/null | openssl x509 -pubkey -noout | openssl pkey -pubin -outform der | openssl dgst -sha256 -binary | base64
