#!/usr/bin/env bash
# ADR-062 rs-19/rs-20 — cut the M5 (ledger of record) over to the router service, unattended.
# Owner decision 2026-09-08: Ra picks the quiet window, all M5 lanes in one window, report after.
#
# Runs FROM THE M1 (holds the provisioner SA key and the ssh key to the M5); every M5 step is one
# ssh command. Steps, each resumable with FROM=<n>:
#   1 preflight   service healthy; M5 binary speaks SIRSI_ROUTER_URL; no `sirsi router` process mid-write
#   2 freeze      M5 router.db: WAL checkpoint, journal_mode=DELETE, chmod a-w  (old writers now FAIL LOUDLY)
#   3 snapshot    .backup on the M5 → M1 → schema-advanced to the binary's version (v18)
#   4 empty       every router table on the destination: the final import lands on an empty ledger (the
#                 service carries no traffic before the cut-over; stale rehearsal rows make the gate fail)
#   5 import      migrate-job.sh REAL; gate = source sha256 == destination sha256
#   6 tokens+env  one host-bound token per Mac (M5 `Mac`, M1 `MacBookPro`) via the token job, into
#                 ~/.sirsi/router-service.env (0600) sourced from ~/.zshenv. The horus supervisor is
#                 `zsh -l -c`, ssh relays and interactive shells are zsh: one file reaches every lane
#   7 verify      `sirsi router status` over HTTPS from BOTH Macs equals the frozen snapshot; one write
#
#   bash scripts/router-service/cutover-m5.sh            # all steps
#   FROM=5 bash scripts/router-service/cutover-m5.sh     # resume at import
#   bash scripts/router-service/cutover-m5.sh rollback   # env lines out, router.db writable again (WAL restored)
#
# Rollback never touches the service: rows written there during the window stay there.
# The M5 binary must already be rebuilt from main at or after PR #711 (step 1 prints the recipe).
set -euo pipefail
PROJECT=${PROJECT:-sirsi-nexus-live}; REGION=${REGION:-us-central1}; INSTANCE=${INSTANCE:-sirsi-router}; CONN="$PROJECT:$REGION:$INSTANCE"
URL=${URL:-https://sirsi-router-6kdf4or4qq-uc.a.run.app}
M5=${M5:-thekryptodragon@192.168.1.155}; M5_HOST=${M5_HOST:-Mac}; M1_HOST=${M1_HOST:-$(hostname -s)}
G="gcloud --project=$PROJECT --quiet"
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
WORK=${WORK:-$HOME/.sirsi/cutover}; mkdir -p "$WORK"
# Only when SIRSI_ROUTER_URL is UNSET: a lane that chose spool:// (and unset its token) must keep both choices when
# its shell re-sources ~/.zshenv — observed 2026-09-10: codex-inference's sandbox shells were handed the https URL
# and the token back by this line and never reached the relay.
SRC_LINE='[ -z "$SIRSI_ROUTER_URL" ] && [ -r "$HOME/.sirsi/router-service.env" ] && . "$HOME/.sirsi/router-service.env"  # ADR-062 router service (only when unset)'
m5() { ssh "$M5" 'export PATH=/opt/homebrew/bin:$HOME/.local/bin:$PATH; '"$*"; }
step() { echo; echo "== $1. $2"; }
items() { grep -m1 'Items:' "$1"; }

# write_env "" | write_env <ssh target>: token on stdin, never in argv or a log line. The file
# travels on stdin too, so nothing is quoted across the ssh boundary.
write_env() {
  local tok; read -r tok
  local sh='umask 077; mkdir -p "$HOME/.sirsi"; cat >"$HOME/.sirsi/router-service.env"; grep -qF router-service.env "$HOME/.zshenv" 2>/dev/null || printf "%s\n" "$0" >>"$HOME/.zshenv"'
  local body; body=$(printf "export SIRSI_ROUTER_URL='%s'\nexport SIRSI_ROUTER_TOKEN='%s'\n" "$URL" "$tok")
  if [ -z "$1" ]; then printf '%s\n' "$body" | bash -c "$sh" "$SRC_LINE"
  else printf '%s\n' "$body" | ssh "$1" "bash -c $(printf %q "$sh") $(printf %q "$SRC_LINE")"; fi
}
# mint <host> <label>: token on stdout. The job's stdout line is the only other copy (Cloud Logging).
mint() {
  $G run jobs update sirsi-router-token --region="$REGION" --args="router,token,mint,$1,--label,$2" >/dev/null
  $G run jobs execute sirsi-router-token --region="$REGION" --wait >/dev/null
  local ex; ex=$($G run jobs executions list --job=sirsi-router-token --region="$REGION" --limit=1 --format='value(name)')
  sleep 30
  $G logging read "resource.type=\"cloud_run_job\" AND resource.labels.job_name=\"sirsi-router-token\" AND labels.\"run.googleapis.com/execution_name\"=\"$ex\"" \
    --limit 50 --format='value(textPayload)' | sed -n 's/^SIRSI_ROUTER_TOKEN=//p' | head -1
}

# restore_local — the node-local half of a rollback, run ON the host (bash -s over ssh, or locally).
# Env: DB (ledger path), FROZEN (retained frozen copy or empty), PREV (retained binary or empty), BIN (live binary).
# Fails loudly on every step (set -e) so the caller's exit status is real. Idempotent.
RESTORE_LOCAL='set -euo pipefail
: "${DB:?}" "${BIN:?}"
if [ -d "$DB" ]; then                                   # unopenable placeholder left by the cut-over
  [ -n "${FROZEN:-}" ] && [ -f "$FROZEN" ] || { echo "REFUSED: $DB is the cut-over placeholder directory and no frozen copy was given — see the runbook (Rollback — a node)" >&2; exit 2; }
  rmdir "$DB"; cp "$FROZEN" "$DB"                      # the frozen original stays untouched
fi
[ -f "$DB" ] || { echo "REFUSED: no ledger file at $DB" >&2; exit 2; }
chmod u+w "$DB"; sqlite3 "$DB" "PRAGMA journal_mode=wal;" >/dev/null
if [ -n "${PREV:-}" ] && [ -e "$PREV" ]; then rm -f "$BIN"; cp "$PREV" "$BIN"; fi
echo "restored: $DB ($(sqlite3 "$DB" "pragma user_version") schema, wal) binary=$BIN"
'

# remove_marker — run ON the host after restore_local succeeded. An ABSENT marker is permitted explicitly;
# a PRESENT marker that cannot be renamed is a failure (never `|| true`); absence is asserted before success.
# Env: MARKER (default ~/.sirsi/router-service.env), ZSHENV (default ~/.zshenv).
REMOVE_MARKER='set -euo pipefail
M=${MARKER:-$HOME/.sirsi/router-service.env}; Z=${ZSHENV:-$HOME/.zshenv}
if [ -e "$M" ]; then mv "$M" "$M.rolled-back-$(date -u +%Y%m%dT%H%M%SZ)"; fi
[ -f "$Z" ] && sed -i "" "/router-service.env/d" "$Z"
[ ! -e "$M" ] || { echo "FAILED: marker still present at $M" >&2; exit 3; }
echo "marker removed: $M"
'

if [ "${1:-}" = rollback ]; then
  echo "== rollback: restore the local ledger on both Macs FIRST, then remove the env markers"
  # Order matters: the marker is removed only after the local file is back, so a host is never
  # left with neither a service env nor an openable ledger. Failures propagate (no exit-0 lies).
  ssh "$M5" "DB=\$HOME/.sirsi/router.db FROZEN=\$(ls -t \$HOME/.sirsi/router.db.frozen-* 2>/dev/null | head -1) PREV=\$HOME/.sirsi/build/sirsi-prev BIN=\$HOME/.local/bin/sirsi bash -s" <<<"$RESTORE_LOCAL"
  DB=$HOME/.sirsi/router.db FROZEN=$(ls -t $HOME/.sirsi/router.db.old-* $HOME/.sirsi/router.db.frozen-* 2>/dev/null | head -1) PREV= BIN=$(command -v sirsi) bash -s <<<"$RESTORE_LOCAL"
  bash -s <<<"$REMOVE_MARKER"; ssh "$M5" bash -s <<<"$REMOVE_MARKER"
  rm -f "$WORK/activated"
  echo "rolled back: both Macs write their local file again; service rows written after the freeze are NOT copied back (rs-20b)"
  exit 0
fi
FROM=${FROM:-1}
# Already cut over? Refuse BEFORE any host mutation (SSA 2026-09-10): a default rerun must never reach the
# freeze/swap in step 2 or the truncate in step 4. `rollback` above is the only verb that runs on an activated host.
[ -e "$WORK/activated" ] && [ "$FROM" -le 4 ] && { echo "REFUSED: $WORK/activated exists (cut over $(cat "$WORK/activated")) — this host is live on the service; use \`rollback\` or FROM=5+ only" >&2; exit 1; }

if [ "$FROM" -le 1 ]; then
  step 1 preflight
  curl -fsS "$URL/v1/healthz" >/dev/null || { echo "service unhealthy: $URL/v1/healthz" >&2; exit 1; }; echo "   service healthy: $URL"
  # The service-capable binary is STAGED, not live: it refuses the v16 local store ("deployment event"),
  # so it must replace ~/.local/bin/sirsi only after the freeze (step 2), inside the window.
  m5 'strings "$HOME/.sirsi/build/sirsi-main" | grep -q SIRSI_ROUTER_URL' || {
    echo "stage the main-built binary first:" >&2
    echo "  ssh $M5 'cd ~/Development/sirsi-pantheon && git fetch origin main && git worktree add -f ~/.sirsi/build/pantheon-main origin/main; cd ~/.sirsi/build/pantheon-main && go build -o ~/.sirsi/build/sirsi-main ./cmd/sirsi'" >&2
    exit 1; }
  if m5 'pgrep -fl "sirsi router (send|task|thread|complete|claim)"'; then echo "M5 has a router command mid-flight; wait" >&2; exit 1; fi
  m5 'launchctl list | grep -E "sirsi|horus"; uptime'
fi

if [ "$FROM" -le 2 ]; then
  step 2 "freeze M5 router.db (old writers fail loudly from here on)"
  m5 'sqlite3 ~/.sirsi/router.db "PRAGMA wal_checkpoint(TRUNCATE); PRAGMA journal_mode=DELETE;" && chmod a-w ~/.sirsi/router.db && ls -la ~/.sirsi/router.db*'
  # Swap the binary now: rm then cp (cp over a live binary SIGKILLs it). Running processes keep the old inode.
  # sirsi-prev is the retained PRE-cutover binary: never overwrite an existing one.
  m5 '[ -e ~/.sirsi/build/sirsi-prev ] || cp ~/.local/bin/sirsi ~/.sirsi/build/sirsi-prev; rm ~/.local/bin/sirsi && cp ~/.sirsi/build/sirsi-main ~/.local/bin/sirsi && ls -la ~/.local/bin/sirsi'
fi

if [ "$FROM" -le 3 ]; then
  step 3 "snapshot → M1 → schema advanced to the binary's version"
  TS=$(date -u +%Y%m%dT%H%M%SZ); echo "$TS" >"$WORK/current"
  m5 "sqlite3 ~/.sirsi/router.db '.backup /tmp/router-cutover.db'"
  scp -q "$M5:/tmp/router-cutover.db" "$WORK/snap-$TS.db"; ssh "$M5" rm -f /tmp/router-cutover.db
  env -u SIRSI_ROUTER_URL -u SIRSI_ROUTER_TOKEN SIRSI_ALLOW_SCHEMA_MIGRATE=1 SIRSI_ROUTER_DB="$WORK/snap-$TS.db" sirsi router status >"$WORK/status-$TS.txt"
  echo "   schema v$(sqlite3 "$WORK/snap-$TS.db" 'pragma user_version');$(items "$WORK/status-$TS.txt")"
fi
TS=$(cat "$WORK/current")

if [ "$FROM" -le 4 ]; then
  step 4 "empty the destination (every router table, not only identity rows)"
  # 2026-09-10 lesson: the service still held the rehearsal snapshot; 6,422 rows had changed on the M5 since,
  # ON CONFLICT DO NOTHING kept the stale versions, the gate failed and the import rolled back. The final
  # import must land on an EMPTY ledger — the service carries no traffic of its own before the cut-over.
  # Executable guard (SSA 2026-09-10): an activated ledger has host tokens; the pre-cutover rehearsal state
  # has none (identity tables are cleared before every rehearsal import). A destination with tokens or a
  # local activation marker is live and is never emptied by this script.
  [ -e "$WORK/activated" ] && { echo "REFUSED: $WORK/activated exists — this host already cut over; emptying the service would destroy live work" >&2; exit 1; }
  SQL="DO \$\$ DECLARE n int; BEGIN SELECT count(*) INTO n FROM router.host_tokens; IF n > 0 THEN RAISE EXCEPTION 'REFUSED: destination has % host token(s) — it is an activated ledger, not rehearsal data', n; END IF; END \$\$; TRUNCATE router.items, router.agents, router.state, router.breakers, router.send_quota, router.counters, router.tasks, router.identifiers, router.requirements, router.wake_events, router.threads, router.sessions, router.lease_sessions, router.host_tokens CASCADE; SELECT (SELECT count(*) FROM router.items) items,(SELECT version FROM router.schema_version) v;"
  $G run jobs update sirsi-router-psql --region="$REGION" --args="^@^-v@ON_ERROR_STOP=1@-h@/cloudsql/$CONN@-U@router_migrator@-d@router@-c@$SQL" >/dev/null
  $G run jobs execute sirsi-router-psql --region="$REGION" --wait
fi

if [ "$FROM" -le 5 ]; then
  step 5 "import (real) + hash gate"
  MODE=import SNAP="$WORK/snap-$TS.db" bash "$ROOT/scripts/router-service/migrate-job.sh" | tee "$WORK/import-$TS.log"
  src=$(sed -n 's/^IMPORT  source \([0-9a-f]*\).*/\1/p' "$WORK/import-$TS.log" | tail -1)
  dst=$(sed -n 's/^destination \([0-9a-f]*\).*/\1/p' "$WORK/import-$TS.log" | tail -1)
  [ -n "$src" ] && [ "$src" = "$dst" ] || { echo "GATE FAILED source=$src destination=$dst — M5 stays frozen; fix and rerun FROM=4" >&2; exit 1; }
  echo "   hash-equal $src"
fi

if [ "$FROM" -le 6 ]; then
  step 6 "tokens + env (M5 host $M5_HOST, M1 host $M1_HOST)"
  mint "$M5_HOST" "M5-cutover-$TS" | write_env "$M5"
  mint "$M1_HOST" "M1-cutover-$TS" | write_env ""
  echo "   ~/.sirsi/router-service.env (0600) on both Macs, sourced from ~/.zshenv"
fi

if [ "$FROM" -le 7 ]; then
  step 7 "verify from both Macs over HTTPS"
  zsh -c 'sirsi router status' >"$WORK/verify-m1-$TS.txt"
  m5 'sirsi router status' >"$WORK/verify-m5-$TS.txt"
  for f in m1 m5; do [ "$(items "$WORK/status-$TS.txt")" = "$(items "$WORK/verify-$f-$TS.txt")" ] && echo "   $f == snapshot:$(items "$WORK/verify-$f-$TS.txt")" || { echo "   $f DIFFERS from snapshot" >&2; exit 1; }; done
  m5 'sirsi router task reclaim-expired' >/dev/null && echo "   M5 write over HTTPS ok"
  date -u +%Y-%m-%dT%H:%M:%SZ >"$WORK/activated"   # from here on step 4 refuses: the service is live
  echo "CUT OVER. Next: close rs-18/rs-19/rs-20 on the ledger; Bind #4; delete the migrate image printed above."
fi
