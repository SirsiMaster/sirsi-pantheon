#!/usr/bin/env bash
set -euo pipefail

# Firebase no longer provisions a default Hosting site for newly-created
# projects. Keep first automated deploys deterministic and idempotent.
project_id="${1:?usage: $0 PROJECT_ID SITE_ID}"
site_id="${2:?usage: $0 PROJECT_ID SITE_ID}"
firebase_cli_version="${FIREBASE_CLI_VERSION:-15.25.1}"

list_json="$(npx --yes "firebase-tools@${firebase_cli_version}" hosting:sites:list \
  --json --project "${project_id}")"

if printf '%s' "${list_json}" | node -e '
  const fs = require("node:fs");
  const site = process.argv[1];
  const payload = JSON.parse(fs.readFileSync(0, "utf8"));
  const sites = payload?.result?.sites ?? payload?.sites ?? [];
  const found = sites.some((entry) => {
    const name = String(entry.name ?? "");
    return name === site || name.endsWith(`/sites/${site}`);
  });
  process.exit(found ? 0 : 1);
' "${site_id}"; then
  echo "Firebase Hosting site already exists: ${site_id} (${project_id})"
  exit 0
fi

echo "Creating Firebase Hosting site: ${site_id} (${project_id})"
npx --yes "firebase-tools@${firebase_cli_version}" hosting:sites:create "${site_id}" \
  --project "${project_id}"
