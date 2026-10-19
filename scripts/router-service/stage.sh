#!/usr/bin/env bash
# Build the router service image LOCALLY and deploy it staged: no-traffic tagged revision, health check, then
# shift 100% of traffic. Cloud Build cannot compile cmd/sirsi: it imports the private
# github.com/SirsiMaster/sirsi-inference module and Cloud Build holds no credential for it (2026-10-18, the first
# deploy after #1109). So the binary is cross-compiled here, where the module is reachable (a developer module
# cache, or scripts/ci-private-sdk.sh in CI), and Cloud Build only COPYs it into the distroless image.
#
#   stage.sh --context-only DIR      build the image context into DIR and stop (tests, inspection)
#   stage.sh [gcloud run deploy flags...]
#
# Prints PREV=<revision that served before> and NEW=<revision now serving 100%> for receipts. Rollback:
#   gcloud run services update-traffic sirsi-router --to-revisions PREV=100 --region R --project P
set -euo pipefail
ROOT=$(git rev-parse --show-toplevel)
PROJECT=${PROJECT:-sirsi-nexus-live}; REGION=${REGION:-us-central1}
VERSION=${VERSION:-$(git -C "$ROOT" describe --tags --always)}
export GOPRIVATE=${GOPRIVATE:-github.com/SirsiMaster/sirsi-inference,github.com/SirsiMaster/sirsi-mercury}

build_context() {
  mkdir -p "$1"
  (cd "$ROOT" && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X github.com/SirsiMaster/sirsi-pantheon/internal/version.Version=$VERSION" -o "$1/sirsi" ./cmd/sirsi)
  printf "FROM gcr.io/distroless/static-debian12:nonroot\nCOPY sirsi /sirsi\nENTRYPOINT [\"/sirsi\"]\nCMD [\"router\",\"serve\"]\n" >"$1/Dockerfile"
}

if [ "${1:-}" = --context-only ]; then
  [ -n "${2:-}" ] || { echo "usage: stage.sh --context-only DIR" >&2; exit 2; }
  build_context "$2"; exit 0
fi

CTX=$(mktemp -d); trap "rm -rf \"$CTX\"" EXIT
build_context "$CTX"
RP=(--project="$PROJECT" --region="$REGION")
PREV=$(gcloud run services describe sirsi-router "${RP[@]}" --format="value(status.traffic[0].revisionName)" 2>/dev/null || true)
if [ -z "$PREV" ]; then
  # First deploy: a new service cannot start with --no-traffic.
  gcloud run deploy sirsi-router --source "$CTX" "${RP[@]}" --quiet "$@"
  echo "PREV="; echo "NEW=$(gcloud run services describe sirsi-router "${RP[@]}" --format="value(status.latestReadyRevisionName)")"
  exit 0
fi
TAG="stg-$(git -C "$ROOT" rev-parse --short=8 HEAD)"
gcloud run deploy sirsi-router --source "$CTX" "${RP[@]}" --no-traffic --tag "$TAG" --quiet "$@"
NEW=$(gcloud run services describe sirsi-router "${RP[@]}" --format="value(status.latestReadyRevisionName)")
[ -n "$NEW" ] && [ "$NEW" != "$PREV" ] || { echo "stage: no new ready revision (latest=$NEW); traffic still on $PREV" >&2; exit 1; }
TURL=$(gcloud run services describe sirsi-router "${RP[@]}" --format=json | jq -r --arg t "$TAG" ".status.traffic[] | select(.tag==\$t) | .url")
for _ in 1 2 3; do
  code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 10 "$TURL/v1/healthz" || true)
  [ "$code" = 200 ] || { echo "stage: $NEW answered healthz $code at $TURL; traffic NOT shifted (still $PREV)" >&2; exit 1; }
done
gcloud run services update-traffic sirsi-router "${RP[@]}" --to-revisions "$NEW=100" --quiet
gcloud run services update-traffic sirsi-router "${RP[@]}" --remove-tags "$TAG" --quiet >/dev/null
echo "PREV=$PREV"; echo "NEW=$NEW"
