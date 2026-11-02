#!/usr/bin/env bash
# Hosted router board (Cloud Run service sirsi-router-board): the owner's live
# router console, usable from any device, behind Google sign-in limited to
# sirsi.ai accounts (IAP). It reads the ledger through the router service with
# a read-only @observer token. Mac controls run on the Mac a viewer picks: the
# board joins the tailnet as tag:cloud-board and forwards them to that Mac's own
# console (tailscale serve → 127.0.0.1:9119), which applies its own action
# registry and confirm guard.
#
# Per Mac, once (host lane): the console LaunchAgent (ai.sirsi.router.surface)
# and `tailscale serve --bg 9119`. Owner, once: a reusable ephemeral Tailscale
# key tagged tag:cloud-board into secret sirsi-router-board-tailscale-key, and an
# ACL letting tag:cloud-board reach the Macs on 443 only.
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
# Created by the owner (Tailscale admin → Keys: reusable, ephemeral, tag:cloud-board) and stored
# with `gcloud secrets versions add sirsi-router-board-tailscale-key --data-file=-`; never by an agent.
TS_SECRET=sirsi-router-board-tailscale-key
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
  cp "$ROOT/CHANGELOG.md" "$1/CHANGELOG.md"
  (cd "$ROOT" && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X github.com/SirsiMaster/sirsi-pantheon/internal/version.Version=$(git -C "$ROOT" describe --tags --always)" \
    -o "$1/sirsi" ./cmd/sirsi)
  # The board joins the owner's tailnet (userspace, no TUN) as tag:cloud-board so it can drive each
  # Mac's console at https://<mac>.<tailnet>.ts.net; without TS_AUTHKEY it starts read-only.
  cat >"$1/board-entry.sh" <<'ENTRY'
#!/bin/sh
set -eu
if [ -n "${TS_AUTHKEY:-}" ]; then
  tailscaled --tun=userspace-networking --socks5-server=127.0.0.1:1055 --state=mem: --statedir=/tmp/tailscale &
  tailscale up --authkey="$TS_AUTHKEY" --hostname=sirsi-router-board --advertise-tags=tag:cloud-board --accept-dns=false
  export SIRSI_BOARD_TS_PROXY=socks5://127.0.0.1:1055
else
  unset SIRSI_BOARD_MACS
fi
exec /sirsi router board-serve
ENTRY
  chmod +x "$1/board-entry.sh"
  printf 'FROM tailscale/tailscale:v1.76.6\nCOPY sirsi /sirsi\nCOPY board-entry.sh /board-entry.sh\nCOPY recipient-registry.json /.agents/idea-router/agents.json\nCOPY CHANGELOG.md /CHANGELOG.md\nWORKDIR /\nENTRYPOINT ["/board-entry.sh"]\n' >"$1/Dockerfile"
}

case "${1:-}" in
provision)
  run $G services enable iap.googleapis.com
  $G iam service-accounts describe "$SA" >/dev/null 2>&1 || \
    run $G iam service-accounts create "$SERVICE" --display-name="Hosted router board"
  # Exactly two secrets, nothing else: no Cloud SQL, no other secret. The Tailscale key's VALUE is
  # added by the owner; this only creates the empty secret and the board's read access to it.
  for s in "$SECRET" "$TS_SECRET"; do
    $G secrets describe "$s" >/dev/null 2>&1 || run $G secrets create "$s" --replication-policy=automatic
    run $G secrets add-iam-policy-binding "$s" --member="serviceAccount:$SA" --role=roles/secretmanager.secretAccessor
  done
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
  # MACS names each Mac console on the tailnet, e.g. M1=https://m1.tail1234.ts.net,M5=…,Studio=…
  MACS=${MACS:?"MACS=name=https://<mac>.<tailnet>.ts.net,… (each Mac: tailscale serve --bg 9119)"}
  SECRETS="SIRSI_ROUTER_TOKEN=$SECRET:latest"
  $G secrets describe "$TS_SECRET" >/dev/null 2>&1 && SECRETS="$SECRETS,TS_AUTHKEY=$TS_SECRET:latest"
  # --no-cpu-throttling keeps the tailnet connection alive between requests; min-instances 0 still
  # scales the board to nothing when nobody is looking.
  run $G beta run deploy "$SERVICE" --source "$CTX" --region="$REGION" --service-account="$SA" \
    --no-allow-unauthenticated --iap --min-instances=0 --max-instances=2 --cpu=1 --memory=512Mi --timeout=300 \
    --no-cpu-throttling --set-env-vars="^@^SIRSI_ROUTER_URL=$URL@SIRSI_ROUTER_CLOUD_HOST=cloud-board@SIRSI_AGENT_ID=router-board@SIRSI_BOARD_MACS=$MACS" \
    --set-secrets="$SECRETS" --labels=workstream=router-board
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
