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
  # The deployment receipt must describe committed source, not an unpushed
  # working-tree experiment or an untracked Go file picked up by the compiler.
  [ -z "$(git -C "$ROOT" status --porcelain --untracked-files=normal)" ] || {
    echo "stage: clean committed source required; preserve changes and build an isolated checkout" >&2; return 1;
  }
  mkdir -p "$1"
  # Ship only the recipient admission projection, not Mac paths, consumer
  # commands, environment overrides or any live store/session data. FindRepoRoot
  # recognizes this immutable declared registry without a fake .git directory.
  jq --sort-keys --arg source "$(git -C "$ROOT" rev-parse HEAD)" \
    '{schema:"sirsi-router-recipient-registry.v1",source_commit:$source,agents:(.agents|map_values({type,description,mail_to,wake:{mechanism:.wake.mechanism},delivery_contract}|with_entries(select(.value!=null))))}' \
    "$ROOT/.agents/idea-router/agents.json" >"$1/recipient-registry.json"
  (cd "$ROOT" && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X github.com/SirsiMaster/sirsi-pantheon/internal/version.Version=$VERSION" -o "$1/sirsi" ./cmd/sirsi)
  printf "FROM gcr.io/distroless/static-debian12:nonroot\nCOPY sirsi /sirsi\nCOPY recipient-registry.json /.agents/idea-router/agents.json\nWORKDIR /\nENTRYPOINT [\"/sirsi\"]\nCMD [\"router\",\"serve\"]\n" >"$1/Dockerfile"
}

if [ "${1:-}" = --context-only ]; then
  [ -n "${2:-}" ] || { echo "usage: stage.sh --context-only DIR" >&2; exit 2; }
  build_context "$2"; exit 0
fi

RP=(--project="$PROJECT" --region="$REGION")
PREV=$(bash "$ROOT/scripts/router-service/traffic-target.sh" "${RP[@]}") || exit 1
CTX=$(mktemp -d); trap "rm -rf \"$CTX\"" EXIT
build_context "$CTX"
# Building may take minutes. Refuse if the serving state changed during it.
CURRENT=$(bash "$ROOT/scripts/router-service/traffic-target.sh" "${RP[@]}") || exit 1
[ "$CURRENT" = "$PREV" ] || { echo "stage: serving traffic changed during build; refusing deployment" >&2; exit 1; }
if [ -z "$PREV" ]; then
  # First deploy: a new service cannot start with --no-traffic.
  gcloud run deploy sirsi-router --source "$CTX" "${RP[@]}" --quiet "$@"
  echo "PREV="; echo "NEW=$(gcloud run services describe sirsi-router "${RP[@]}" --format="value(status.latestReadyRevisionName)")"
  exit 0
fi
# A fresh revision identity binds the deployment request to the tag readback.
# Cloud Run revision names are service name + suffix; revisions are immutable.
nonce=$(openssl rand -hex 12) || { echo "stage: cannot generate revision nonce" >&2; exit 1; }
[[ "$nonce" =~ ^[0-9a-f]{24}$ ]] || { echo "stage: invalid revision nonce" >&2; exit 1; }
SUFFIX="stg-$(git -C "$ROOT" rev-parse --short=8 HEAD)-$nonce"
TAG="$SUFFIX"
EXPECTED="sirsi-router-$SUFFIX"
gcloud run deploy sirsi-router --source "$CTX" "${RP[@]}" --no-traffic --quiet "$@" --tag "$TAG" --revision-suffix "$SUFFIX"
# Bind the URL actually probed to its tagged revision, while confirming the
# original serving target remains intact. Never use global latestReady here.
staged_target() {
  local service target
  service=$(gcloud run services describe sirsi-router "${RP[@]}" --format=json) || return 1
  target=$(jq -ser --arg tag "$TAG" --arg prev "$PREV" --arg expected "$EXPECTED" '
    if length != 1 or .[0].metadata.name != "sirsi-router" or (.[0].status.traffic | type) != "array" then error("invalid service") else .[0].status.traffic end
    | if all(.[]; type == "object" and
        (((.percent | type) == "number" and .percent >= 0 and .percent <= 100 and (.percent | floor) == .percent)
         or (.percent == null and (.tag | type == "string" and length > 0)))) then . else error("invalid traffic") end
    | . as $traffic
    | [ $traffic[] | select((.percent // 0) > 0) ] as $serving
    | [ $traffic[] | select(.tag == $tag) ] as $staged
    | if ($serving | length) == 1 and $serving[0].percent == 100 and $serving[0].revisionName == $prev
      and ($staged | length) == 1 and ($staged[0].percent // 0) == 0
      and ($staged[0].revisionName | type == "string" and test("^[a-z]([-a-z0-9]*[a-z0-9])?$"))
      and $staged[0].revisionName == $expected and $expected != $prev
      and ($staged[0].url | type == "string" and test("^https://[a-z0-9][a-z0-9.-]*[a-z0-9]$"))
      then [$staged[0].revisionName, $staged[0].url] | @tsv else error("ambiguous staged or serving traffic") end
  ' <<<"$service") || return 1
  [[ "$target" =~ ^[a-z][-a-z0-9]*$'\t'https://[a-z0-9][a-z0-9.-]*[a-z0-9]$ ]] || return 1
  printf '%s\n' "$target"
}
STAGED=$(staged_target) || { echo "stage: invalid staged tag or serving traffic; refusing traffic shift" >&2; exit 1; }
IFS=$'\t' read -r NEW TURL <<<"$STAGED"
for _ in 1 2 3; do
  code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 10 "$TURL/v1/healthz" || true)
  [ "$code" = 200 ] || { echo "stage: $NEW answered healthz $code at $TURL; traffic NOT shifted (still $PREV)" >&2; exit 1; }
done
CURRENT_STAGED=$(staged_target) || { echo "stage: staged tag or serving traffic changed; refusing traffic shift" >&2; exit 1; }
[ "$CURRENT_STAGED" = "$STAGED" ] || { echo "stage: staged target changed after health checks; refusing traffic shift" >&2; exit 1; }
gcloud run services update-traffic sirsi-router "${RP[@]}" --to-revisions "$NEW=100" --quiet
gcloud run services update-traffic sirsi-router "${RP[@]}" --remove-tags "$TAG" --quiet >/dev/null
echo "PREV=$PREV"; echo "NEW=$NEW"
echo "RACE=readbacks are not atomic with health probes or traffic update; single publisher required"
