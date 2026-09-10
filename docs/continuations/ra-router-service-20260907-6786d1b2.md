<!-- agent: ra | workstream: router-service (ADR-062) | repo: /Users/sirsimasterdev/Development/sirsi-pantheon (M1) | date: 2026-09-07 | session: 6786d1b2-1c1f-4213-bd0b-f0f1d0d651fb | thread: thr-df2a8cd5b1c61290 -->

# Ra — router-service continuation (2026-09-07T17:2xZ) — TRANSFERRED TO THE M1

Supersedes `ra-router-service-20260903-2d18bc60.md`. Owner 2026-09-07: "transfer this session to m1". The M5 (Astra) belongs to SNE; Ra runs on the M1 from now on.

## Who you are, where you run
Router agent `ra`. Host: M1 (`sirsimasterdev@192.168.1.180`, M1 Pro 16 GiB, macOS 26.6.2). Start Claude Code from `$HOME` (`/Users/sirsimasterdev`) so `~/.claude/projects/-Users-sirsimasterdev/memory/` loads. PATH for non-login shells: `/opt/homebrew/bin:$HOME/.local/bin`.
- `sirsi` = `~/.local/bin/sirsi`, built from main 68568e9d on 2026-09-07 (`go build -o ~/.local/bin/sirsi ./cmd/sirsi`).
- Repo `~/Development/sirsi-pantheon` on branch `main` (was detached at 92ad980 before the move). `gh` is LOGGED IN on the M1 as SirsiMaster since 2026-09-07T20:xxZ (https protocol) — push and PRs work from here.
- Session transcript copied to `~/.claude/projects/-Users-sirsimasterdev/6786d1b2-1c1f-4213-bd0b-f0f1d0d651fb.jsonl` (try `claude --resume 6786d1b2-1c1f-4213-bd0b-f0f1d0d651fb` from `$HOME`; if the app refuses, resume from this file + memory).
- Memory rsynced from the M5 (`~/.claude/projects/-Users-thekryptodragon/memory/` → M1). Paths inside still say `/Users/thekryptodragon`; read them as the M5.

## The ledger still lives on the M5 (until rs-18)
The `ra` ledger and every router item are in the M5 store `~/.sirsi/router.db` (this is exactly what ADR-062 fixes; the service is not deployed yet). From the M1, relay every router/thread verb:
```
ssh thekryptodragon@192.168.1.155 'export PATH=/opt/homebrew/bin:$HOME/.local/bin:$PATH; sirsi router ledger ra'
```
Thread registration for `ra` must also be on the M5 store (reuse the id, never mint): `… sirsi thread register --agent ra --surface claude --thread thr-df2a8cd5b1c61290 --watch ra,claude-home --consumer-capable --workstream router-service`, then `sirsi thread heartbeat --thread thr-df2a8cd5b1c61290` relayed the same way. The M1's own `~/.sirsi/router.db` (Aug 29) is NOT the ledger.

## Where the work stands (verified 2026-09-07T17:2xZ)
| Phase | State | Evidence |
|---|---|---|
| A–C rs-01..13 | merged | #682–#687, #703 (e7af6a04) |
| D rs-14 | done | owner "1a 2a 3a" 2026-09-03 |
| D rs-15 provision | **DONE 2026-09-07T23:5xZ** (see below; ledger closed with evidence). Earlier: scaffolding MERGED — **PR #704 → 68568e9d** (SSA ACCEPT at head 556bf92e, item 20260907-153958; sirsi-bind App review 2026-09-07T17:03Z; binding-hold run 33808373927 green). Ledger row stays `in-progress`: provision.sh has not run. | `scripts/router-service/{provision,deploy,grant-provisioner}.sh`, Dockerfile |
| D rs-16 first deploy | **DONE 2026-09-08T00:2xZ** — revision `sirsi-router-00003-nq4`, ledger closed | see below |
| D rs-17 rehearsals | **DONE 2026-09-08T00:5xZ** — rollback 5.5 s / forward 5.2 s warm, 39 s cold; revocation → 401 | see below |
| D rs-18 migrate | **import PROVEN 2026-09-08T00:5xZ** — cloud == M5 snapshot, hash-equal, re-import wrote={}; row stays in-progress until the read-only switch at cut-over | migrate-job.sh, executions 29jsc + f5kh4 |
| D rs-19 cut-over | **OWNER CARD SENT** item 20260908-005700 (1a/1b/1c when, 2a/2b scope) | |
| D rs-20, E rs-21..25 | pending | behind rs-19 |

### rs-15 — the one thing Ra cannot do itself
Owner runs `scripts/router-service/grant-provisioner.sh` as sirsimaster@gmail.com (grants the provisioning roles to `claude-agent@sirsi-nexus-live`). On the M1: gcloud installed via `brew install --cask google-cloud-sdk`; SA key at `~/.config/gcloud/sirsi-nexus-live-claude-agent.json` (mode 600). After the grant:
```
cd ~/Development/sirsi-pantheon
export CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE=~/.config/gcloud/sirsi-nexus-live-claude-agent.json
DRY_RUN=1 bash scripts/router-service/provision.sh   # review, then run without DRY_RUN
bash scripts/router-service/deploy.sh                # rs-16
```
Probe first: `gcloud projects get-iam-policy sirsi-nexus-live --flatten=bindings --filter='bindings.members:claude-agent'` shows whether the grant landed. Region us-central1. cloud-sql-proxy 2.25.4 + psql 18.6 (`/opt/homebrew/opt/libpq/bin`) are installed on the M1.

**2026-09-07T20:3xZ probe (session 84ab1eaa, M1):** grant NOT landed — `sql instances list` denied, `secrets list` / `services list` / `iam service-accounts list` OK (pre-existing roles). `DRY_RUN=1 provision.sh` died at step 4 (read a secret the dry run had only echoed) — fixed by `secret_get` (dry-run placeholder); dry run now exits 0, negative control exits 1.

## Housekeeping
M5 main checkout still on foreign dirty branch `fix/broker-quarantine` — never work there. `worktrees/ra-rs01` (branch rs13-evidence) droppable. Owner item `20260902-211336-…-pr-678-blocked-signed-release…` still open. Charter ADR-063 (174dc7c8) in force: complete = verified release at rs-25; tokens are not progress.

## rs-15 + rs-16 executed for real (2026-09-07T23:1xZ – 2026-09-08T00:2xZ, session 84ab1eaa on the M1)
Owner granted the 11 provisioner roles from the IAM page in Safari at 23:19Z (phone; Cloud Shell in the mobile app had no gcloud account).
Probe that works: REST `projects:testIamPermissions` with the SA token.

**Live (sirsi-nexus-live, us-central1):** Cloud SQL `sirsi-router` POSTGRES_16 ENTERPRISE db-f1-micro, private IP 10.95.0.3, db `router`,
users router_migrator/router_service; secrets sirsi-router-{router-migrator-password,router-service-password,bootstrap-token,service-dsn,schema-sql};
runtime SA sirsi-router-svc@ (cloudsql.client, logWriter, accessor on token+dsn); Cloud Run job `sirsi-router-apply-schema` (execution pqg8b:
tables=15 triggers=12 partial=5 version=18, router_service DML-only); Cloud Run service `sirsi-router` revision **00003-nq4**, image
`…/cloud-run-source-deploy/sirsi-router@sha256:bceaeef6…`, URL https://sirsi-router-6kdf4or4qq-uc.a.run.app, SPKI
`78rPvnhm1Lb3jziI2hDDogyku5XoaVABHemUnWwOd7M=` (Google-managed cert — expect rotation; the pin is a receipt, not a lock).
Proof: `/v1/healthz` 200 · no bearer 401 · from a repo cwd with `SIRSI_ROUTER_URL` + `SIRSI_ROUTER_TOKEN=<bootstrap>`: `sirsi router status ra`
→ `Items: 0 open, 0 closed`. The bootstrap token lives only in Secret Manager; read it into env, never a file.

**Six findings, one commit each on this PR (#711):** DRY_RUN placeholder · `--edition=ENTERPRISE` · SA-binding retry · new
`apply-schema-job.sh` (VPC egress flags required; REVOKE cloudsqlsuperuser) · `--store` defaults to `$SIRSI_ROUTER_STORE` because Cloud Run
does not expand `$(VAR)` for secret-backed env vars · health route `/v1/healthz` because run.app swallows `/healthz`.
SSA has the whole-PR bind request (item 20260907-235249; the earlier one-question item 203151 is superseded).

**rs-17 done (2026-09-08T00:5xZ).** Rollback: `run services update-traffic --to-revisions=sirsi-router-00002-qpc=100` 5.5 s, witness
`/v1/healthz` 404 on 00002 while the ledger call still served; `--to-latest` 5.2 s warm, 39.3 s cold. Revocation: token verbs run as Cloud
Run job `sirsi-router-token` on the service image (`token --store` now defaults to `$SIRSI_ROUTER_STORE`, 6c4c2ec); host tokens are bound
to the client's `os.Hostname()` (`MacBookPro`) — wrong host → 403; minted a4c532bb98d0f06f → call OK → revoke → HTTP 401; bootstrap
unaffected; earlier wrong-host token efe0f60f7adc7149 revoked → 401. Service now revision `sirsi-router-00004-f29`.

**Next = rs-18 migrate the M5 ledger.** Blocker is not the cloud: the M5 live store is schema v16 and `router migrate-store` ships in the
v18 binary, which refuses a v16 store without `SIRSI_ALLOW_SCHEMA_MIGRATE=1` — rebuilding `sirsi` on the M5 and migrating its store is a
change on the machine the owner wants quiet, so it goes to the owner as a card (options: do it in a quiet window / copy the store to the M1
and migrate from there with the M5 quiesced / defer). Old plan text: rollback (`gcloud run services update-traffic sirsi-router --to-revisions=sirsi-router-00002-qpc=100`, timed, then
back) and revocation (`sirsi router token revoke` for a host token → next call 401). Then rs-18 migrate the M5 ledger (`router migrate-store`
against the service — needs the M5's `sirsi` rebuilt to v18 first, see [[project-ra-moved-to-m1]] D8 notes).

## rs-18 (2026-09-08T00:2xZ – 00:5xZ): the M5 ledger is in the cloud
Recipe `scripts/router-service/migrate-job.sh`: `sqlite3 ~/.sirsi/router.db ".backup f"` on the M5 (read-only) → copy to the M1 → open once
with the v18 binary + `SIRSI_ALLOW_SCHEMA_MIGRATE=1` (upgrades the COPY 16→18) → throwaway alpine image (static /sirsi + snapshot, `cp` to
/tmp first: an image-layer file copies up on first write and SQLite reports 1544 readonly) → Cloud Run job on the VPC with the DSN from the
secret env. Results: dry run ggnfp (14,567 rows would_write, source untouched); real #1 8b98l wrote all rows but FAILED the gate — (a)
Cloud SQL `en_US.UTF8` collation orders text PKs unlike SQLite, fixed by sorting canonical lines in Go (416c3ab); (b) my rehearsal rows in
sessions/host_tokens/threads — cut-over rule: empty the identity tables right before the final import, mint tokens after. Real #2 29jsc:
hash-equal 232bfcd9…, wrote={threads:2}; real #3 f5kh4: hash-equal, wrote={}. From the M1 over HTTPS `sirsi router status ra` = 735 open /
5687 closed = the M5. Ad-hoc SQL on the VPC: job `sirsi-router-psql`. Owed: delete `sirsi-router-migrate:*` images (need repoAdmin).

## rs-19/rs-20 prep (2026-09-09T17:1xZ, session 84ab1eaa on the M1): runbook written, cut-over waits on #711
`scripts/router-service/cutover-m5.sh` (commit c745844) runs from the M1, seven resumable steps (`FROM=n`) + `rollback`. M5 facts that
shaped it: (1) the M5 `~/.local/bin/sirsi` is the Aug 22 build with NO service client (`strings` finds no SIRSI_ROUTER_URL) — step 1
refuses and prints the rebuild recipe (rm then cp; cp over a live binary SIGKILLs it); (2) there are no `router.wake.*` plists on the M5 —
the only launchd lanes are `ai.sirsi.horus.agent-router` (runs `/bin/zsh -l -c`, so it reads `~/.zshenv`), `ai.sirsi.pantheon` (menubar,
reads only) and `ai.sirsi.host-readiness-watch` (zsh script, never calls `sirsi router`); horus currently exits 127 because its plist
points at a `/private/tmp/pantheon-sne-product-integration-20260830/...` binary that no longer exists — REPORT, do not touch (owner
board 2026-08-09); (3) ssh relays and interactive shells are zsh and read `~/.zshenv` too — so ONE file, `~/.sirsi/router-service.env`
(0600) sourced from `~/.zshenv`, reaches every writer; no plist edits, no wake.go change; (4) the live router.db is WAL + user_version 16,
so freeze = `wal_checkpoint(TRUNCATE)` + `journal_mode=DELETE` + `chmod a-w` (readers of a read-only WAL db need a writable -shm).
Gate = the text report's `IMPORT  source <sha>` / `destination <sha>` lines (migrate-job.sh is text by default now, `JSON=1` opt-in: the
indented JSON is interleaved across Cloud Logging entries). Env writer unit-tested in a throwaway HOME (0600, no duplicate zshenv line,
zsh sees both vars); preflight run live = refused on the old M5 binary, as designed.
**Cold-start defect found and fixed (4fe6aa7):** healthz returned 503 once at 17:14:40Z — the cold instance missed its first private-IP
Cloud SQL connect inside the 5 s budget, exited(1), and Cloud Run started a second instance. Server now retries the open 6×5 s at boot;
the error no longer blames a missing schema for a connect failure. No client retries the 503 (`ErrServiceUnavailable` has no consumer).
Needs a deploy (deploy.sh) after #711 merges — do it BEFORE the cut-over so lanes never see that 503.
Order once #711 is merged: deploy.sh (rev 00007 with the retry) → rebuild M5 sirsi from main → pick a quiet window (M5 `uptime` load,
`pgrep -fl claude` count, no `sirsi router` mid-flight) → `bash scripts/router-service/cutover-m5.sh` → close rs-18/19/20 with the
verify output as evidence → Bind #4 → report to the owner.

## SSA whole-PR review of #711 — REJECT (2026-09-08T21:01Z, item 20260908-210131) — answered 2026-09-09T17:5xZ
Five blocking findings, all addressed on the branch (see the bind request for the mapping): (1) `migrate-job.sh` DRY_RUN mutated GCP →
`DRY_RUN=1` is print-only via `run()`, the rehearsal is `MODE=report` (job with `--dry-run`), the real import `MODE=import`; the job
prints the snapshot's sha256 from inside the container and the script asserts it equals the local file (source identity bound); a
cleanup receipt tries the image delete and prints exactly what the owner still owes when the SA lacks repoAdmin. (2) `provision.sh`
dry run read Secret Manager → `secret_get` never reads under DRY_RUN; passwords go to `gcloud sql users …` via `--prompt-for-password`
on stdin so no argv/log/ps carries them. (3) DML-only unproven → `apply-schema-job.sh` runs as its own SA `sirsi-router-schema@`
(cloudsql.client + secretAccessor on its three secrets), checks the migrator's authority to revoke BEFORE the bundle, and closes the
audit: memberships=[], super/createrole/createdb/bypassrls all f, no CREATE on schema or db, no non-DML default ACL, owns nothing —
then the probe. (4) no atomic rollback → `MigrateStore` runs the import, trigger cleanup and the destination dump inside ONE
transaction; commit only on hash-equal; `Conflicts`/`ConflictCount` name the pre-existing keys; test proves a diverged row leaves the
destination byte-identical. (5) quiescence not ownership-bound → an existing marker is accepted only if it reads
`migrate-store <RFC3339>` younger than 1 h, else refused with the unquarantine hint; tested. Note: the schema job's new SA has NOT
been re-run against the live instance yet (the schema is already applied); it runs at the next apply, or on demand as evidence.
Re-bind request = router item 20260909-172731 (ra → sirsi-software-admin). Correction on record: Ra first ran `scripts/bind/sirsi-bind.sh`
thinking it sent the request; it recorded an APPROVED review as sirsi-bind[bot] on c8e4271 (review 5157708582), dismissed one minute
later with the reason. The SSA reject stands until SSA re-reviews; nobody but the named binder runs sirsi-bind.sh.

## CUT OVER — 2026-09-10T01:59Z–02:2xZ (session 84ab1eaa on the M1): the router service is the ledger of record
Owner (native picker, after "get this done"): merge #711 now on owner authority → 6492a65a; repoint horus on the M5 → done (plist backup
`.bak-20260910`, pid alive). Deploy rev 00007 (boot retry). M5 binary staged at ~/.sirsi/build/sirsi-main, swapped INSIDE the freeze (the
new binary refuses the v16 local store). Run: freeze 21:59 local → snapshot 753/5717 v18 → import #1 v94gq ROLLED BACK (rehearsal rows on
the service, 6,422 diverged keys — the transaction did its job) → TRUNCATE all 14 tables (psql sj98z) → import #2 gpld5 hash-equal
e69c7de3, every row written, source identity bound → tokens Mac + MacBookPro → ~/.sirsi/router-service.env (0600) via ~/.zshenv on both
Macs → both Macs `status` = 753/5717 over HTTPS, M5 write ok → horus kickstarted on new binary with SIRSI_ROUTER_URL. rs-18/19/20 closed
from the M1 with no relay (run task verbs from a repo root: `no .agents/idea-router/` otherwise). Bind #4 routed to SSA with PR #713.
Lessons: never edit a bash script while it runs (bash reads incrementally — step 6 died on a syntax error and was resumed FROM=6); the
final import needs an EMPTY destination; `git bundle create` needs ref names. Still open: rs-21..25 (Phase E); SSA source review of #711
(3 items, post-merge); owner: delete migrate images 020154Z/021020Z, dismiss card 005700. Existing M5/M1 shells need `source ~/.zshenv`.
Rollback = `bash scripts/router-service/cutover-m5.sh rollback` (env out, router.db writable, WAL back, previous binary restored).
