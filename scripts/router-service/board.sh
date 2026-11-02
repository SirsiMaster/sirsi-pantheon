#!/usr/bin/env bash
# Hosted router board (Cloud Run service sirsi-router-board): the owner's live
# router console, readable from any device, behind Google sign-in limited to
# sirsi.ai accounts (IAP). It reads the ledger through the router service with
# a read-only @observer token; no Mac builds or serves it.
#
# Mac controls never pass through the board. It lists every admitted Mac and links
# to that Mac's own console on the tailnet, which admits only signed-in sirsi.ai
# tailnet users (per Mac, host lane: `sirsi dashboard --tailnet-port 9120` and
# `tailscale serve --bg --https=443 http://127.0.0.1:9120`; never the 9119 port).
#
#   board.sh provision   one-time: service account, token secret, IAP binding target
#   board.sh mint        one-time (and on rotation): mint the @observer token straight
#                        into Secret Manager from a Cloud Run job on the service VPC;
#                        the plaintext is never printed or seen by anyone
#   board.sh deploy      build from committed source and deploy (repeatable)
#   DRY_RUN=1 board.sh … prints every gcloud command, mutates nothing remote
#
# Rollback: gcloud run services update-traffic sirsi-router-board --to-revisions PREV=100
# Revoke:   sirsi router token revoke <id> (job, as for mint) — effective on the next request.
set -euo pipefail
ROOT=$(git rev-parse --show-toplevel)
PROJECT=${PROJECT:-sirsi-nexus-live}; REGION=${REGION:-us-central1}; INSTANCE=${INSTANCE:-sirsi-router}
SERVICE=sirsi-router-board; ROUTER=${ROUTER:-sirsi-router}; DOMAIN=${DOMAIN:-sirsi.ai}
SA="$SERVICE@$PROJECT.iam.gserviceaccount.com"; SECRET=sirsi-router-board-token
TAILNET=${TAILNET:-taild727f7.ts.net}
JOB_SA=${JOB_SA:-claude-agent@$PROJECT.iam.gserviceaccount.com}
G="gcloud --project=$PROJECT --quiet"
run() { echo "+ $*" >&2; [ "${DRY_RUN:-0}" = 1 ] || "$@"; }
export GOPRIVATE=${GOPRIVATE:-github.com/SirsiMaster/sirsi-inference,github.com/SirsiMaster/sirsi-mercury}

context() {
  [ -z "$(git -C "$ROOT" status --porcelain --untracked-files=normal)" ] || {
    echo "board: clean committed source required" >&2; return 1; }
  mkdir -p "$1"
  # The same recipient projection the router image ships: no Mac paths, commands or session data.
  # Lane verdicts read only each agent's wake mechanism (router.PingLane).
  jq --sort-keys --arg source "$(git -C "$ROOT" rev-parse HEAD)" \
    '{schema:"sirsi-router-recipient-registry.v1",source_commit:$source,agents:(.agents|map_values({type,description,mail_to,wake:{mechanism:.wake.mechanism},delivery_contract}|with_entries(select(.value!=null))))}' \
    "$ROOT/.agents/idea-router/agents.json" >"$1/recipient-registry.json"
  # Admitted Macs, names only: the board links each to https://<name>.<tailnet>, so it
  # needs no addresses. A Mac joins the list when Ra admits it and the board redeploys.
  jq '{machines:[.machines[]|{id,model,registered_at,admitted_by}]}' \
    "$ROOT/.agents/idea-router/machines.json" >"$1/machines.json"
  cp "$ROOT/CHANGELOG.md" "$1/CHANGELOG.md"
  (cd "$ROOT" && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X github.com/SirsiMaster/sirsi-pantheon/internal/version.Version=$(git -C "$ROOT" describe --tags --always)" \
    -o "$1/sirsi" ./cmd/sirsi)
  printf 'FROM gcr.io/distroless/static-debian12:nonroot\nCOPY sirsi /sirsi\nCOPY recipient-registry.json /.agents/idea-router/agents.json\nCOPY machines.json /.agents/idea-router/machines.json\nCOPY CHANGELOG.md /CHANGELOG.md\nWORKDIR /\nENTRYPOINT ["/sirsi"]\nCMD ["router","board-serve"]\n' >"$1/Dockerfile"
}

case "${1:-}" in
provision)
  run $G services enable iap.googleapis.com
  $G iam service-accounts describe "$SA" >/dev/null 2>&1 || \
    run $G iam service-accounts create "$SERVICE" --display-name="Hosted router board (read-only)"
  $G secrets describe "$SECRET" >/dev/null 2>&1 || run $G secrets create "$SECRET" --replication-policy=automatic
  # Exactly one secret, nothing else: no Cloud SQL, no other secret.
  run $G secrets add-iam-policy-binding "$SECRET" --member="serviceAccount:$SA" --role=roles/secretmanager.secretAccessor
  ;;
mint)
  # Same shape as migrate-job.sh: only VPC hosts reach the private-IP ledger. The job writes the new
  # token as a secret version itself (publishMintedRouterToken) and prints only its id and scope.
  if [ "${DRY_RUN:-0}" = 1 ]; then IMAGE="<dry-run:router-image>"; else
    IMAGE=$($G run services describe "$ROUTER" --region="$REGION" --format='value(spec.template.spec.containers[0].image)'); fi
  run $G run jobs deploy sirsi-router-board-mint --region="$REGION" --image="$IMAGE" --service-account="$JOB_SA" \
    --set-cloudsql-instances="$PROJECT:$REGION:$INSTANCE" --network=default --subnet=default --vpc-egress=private-ranges-only \
    --set-secrets="SIRSI_ROUTER_STORE=sirsi-router-service-dsn:latest" \
    --command=/sirsi --args="router,token,mint,cloud-board,--label,hosted router board (read-only),--agents,router-board,--agents,@observer,--secret-resource,projects/$PROJECT/secrets/$SECRET" \
    --max-retries=0 --task-timeout=5m --labels=workstream=router-board >/dev/null
  run $G run jobs execute sirsi-router-board-mint --region="$REGION" --wait
  ;;
deploy)
  URL=$($G run services describe "$ROUTER" --region="$REGION" --format='value(status.url)')
  [[ "$URL" =~ ^https:// ]] || { echo "board: cannot read the router service URL" >&2; exit 1; }
  CTX=$(mktemp -d); trap 'rm -rf "$CTX"' EXIT
  context "$CTX"
  PREV=$($G run services describe "$SERVICE" --region="$REGION" --format='value(status.traffic[0].revisionName)' 2>/dev/null || true)
  run $G beta run deploy "$SERVICE" --source "$CTX" --region="$REGION" --service-account="$SA" \
    --no-allow-unauthenticated --iap --min-instances=0 --max-instances=2 --cpu=1 --memory=512Mi --timeout=120 \
    --set-env-vars="SIRSI_ROUTER_URL=$URL,SIRSI_ROUTER_CLOUD_HOST=cloud-board,SIRSI_AGENT_ID=router-board,SIRSI_BOARD_TAILNET=$TAILNET" \
    --set-secrets="SIRSI_ROUTER_TOKEN=$SECRET:latest" --labels=workstream=router-board
  NUM=$($G projects describe "$PROJECT" --format='value(projectNumber)')
  # IAP must be allowed to invoke the service; only the sirsi.ai domain may pass IAP.
  run $G run services add-iam-policy-binding "$SERVICE" --region="$REGION" \
    --member="serviceAccount:service-$NUM@gcp-sa-iap.iam.gserviceaccount.com" --role=roles/run.invoker
  run $G beta iap web add-iam-policy-binding --resource-type=cloud-run --service="$SERVICE" --region="$REGION" \
    --member="domain:$DOMAIN" --role=roles/iap.httpsResourceAccessor
  echo "PREV=$PREV"
  echo "NEW=$($G run services describe "$SERVICE" --region="$REGION" --format='value(status.latestReadyRevisionName)')"
  echo "URL=$($G run services describe "$SERVICE" --region="$REGION" --format='value(status.url)')"
  ;;
--context-only)
  [ -n "${2:-}" ] || { echo "usage: board.sh --context-only DIR" >&2; exit 2; }
  context "$2"
  ;;
*) echo "usage: board.sh provision|mint|deploy|--context-only DIR" >&2; exit 2 ;;
esac
