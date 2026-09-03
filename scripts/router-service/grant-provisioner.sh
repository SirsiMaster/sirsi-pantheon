#!/usr/bin/env bash
# ADR-062 rs-15 — owner decision 2026-09-03: provisioning runs as the automation SA, not a human login.
# Run ONCE by a project Owner (sirsimaster@gmail.com). Grants claude-agent@ exactly what
# provision.sh + deploy.sh call: enable APIs, VPC peering range, Cloud SQL, secrets, create the
# runtime SA and bind it, deploy Cloud Run from source (Cloud Build + Artifact Registry).
set -euo pipefail
PROJECT=${PROJECT:-sirsi-nexus-live}
SA=${SA:-claude-agent@$PROJECT.iam.gserviceaccount.com}
for role in \
  roles/serviceusage.serviceUsageAdmin \
  roles/compute.networkAdmin \
  roles/cloudsql.admin \
  roles/secretmanager.admin \
  roles/iam.serviceAccountAdmin \
  roles/iam.serviceAccountUser \
  roles/resourcemanager.projectIamAdmin \
  roles/run.admin \
  roles/cloudbuild.builds.editor \
  roles/artifactregistry.writer \
  roles/storage.admin; do
  gcloud projects add-iam-policy-binding "$PROJECT" --member="serviceAccount:$SA" --role="$role" --condition=None >/dev/null
  echo "granted $role"
done
