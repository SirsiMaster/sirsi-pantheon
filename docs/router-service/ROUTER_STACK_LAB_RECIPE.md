<!-- agent: ra | workstream: router-service (ADR-062) | recipe revision: 3 | authority: this file is the SINGLE authority for the router runtime recipe; screenshots, dashboards and summaries are projections of it. -->

# Router Service — Stack Lab Recipe

**Status:** Active operating recipe (revision 3)
**Owner:** `ra` (router / worker-plane; ADR-062, ADR-063)
**Classification:** Core platform foundation — the router is critical infrastructure; if it fails, many threads' business goes undone. It is therefore built and maintained under Stack Lab methodology: a reproducible recipe assembled from *identified* components, not an untraceable build.
**Observation:** all live identities below captured 2026-09-11 ~02:30–02:40 UTC from the M1 (`sirsimasterdev`) and M5 (`thekryptodragon`) hosts and from `gcloud` (project `sirsi-nexus-live`, region `us-central1`), unless a row is marked *reconstructed* or *unverified*.

## What this document is (and is not)

The router is a runtime: a Cloud Run service, a Cloud SQL ledger, per-host client binaries, and per-host relays. This file is an **operating inventory with explicit reconstruction gaps** — every component named and hash-identified, each fact labelled by how it is known: **verified** (observed live now), **Ra-reported** (observed this session, retained receipt pending), **reconstructed** (inferred, e.g. a source→binary edge with no retained build receipt), or **pending** (not done). It is **not yet** a full reproduce-from-this-file-alone recipe: the source→binary build receipts (dirty state, exact command, output hash) and the pinned server-image digests are not captured (Reconciliation D3/D5). Where a gap exists it is stated, and the residual is tracked in `rs-27`.

## Stack Lab rules applied here

- **One recipe, one current build state.** Superseded identities move to the Change Log, not a parallel doc.
- **One authority per fact.** Source commit, per-host binary hashes, image digest, revision, schema version, role authority, host, and evidence receipts live here.
- **Distinct evidence states.** Each component is proven at **compiled**, **applied/deployed**, **verified** — each with a concrete receipt. A green build proves only *compiled*. A runnable command is a *procedure*, not evidence it passed.
- **No sprawl / identified payloads.** Secrets referenced by Secret Manager name, never value. Images/schema by digest/version.

## 1. Components (current identities)

### Source & toolchains
| # | Component | Identity | Provenance | Evidence state |
|---|-----------|----------|------------|----------------|
| C1 | Source | `origin/main` @ `3d535ebcdd7663f125efcf6ea53109aa29878afc` (PR #754 squash, 2026-09-13T08:08:56Z; previously `bd5a4614`) | GitHub SirsiMaster/sirsi-pantheon | verified — CI green on head `d85f8fe` (all five checks), merged on owner-waived bind |
| C2a | Client toolchain, M1 | Go `go1.27.1`, `GOARCH=arm64` (from `go version -m ~/.local/bin/sirsi`) | host M1 | verified |
| C2b | Client toolchain, M5 | Go `go1.26.2` (from `go version -m ~/.local/bin/sirsi`) | host M5 | verified |
| C2s | Server toolchain | build `golang:1.25-alpine`, runtime `gcr.io/distroless/static-debian12:nonroot` (repo `Dockerfile`) | Cloud Run source build | *reconstructed from Dockerfile* — both tags are **mutable**; the exact builder/runtime image **digests** are not pinned and not captured (see Reconciliation D3) |

The three toolchains differ on purpose (per-host client builds + a container server build); the binaries are therefore **not** byte-identical across hosts even from the same C1. Recorded, not a defect.

### Client binaries (per host — distinct identities)
| # | Host | sha256 | Built from | Evidence state |
|---|------|--------|-----------|----------------|
| C3a | M1 `~/.local/bin/sirsi` | sha256 `341b7ae052808886146da36fa3cf2bfadb5ff1ce40778b49561f071529e2f9aa`; `go version -m`: go1.27.1, arm64, trimpath=true (previous `b1911d04…` retained as `~/.sirsi/candidates/sirsi-pre-3d535ebc`) | *reconstructed* C1→C3a: built 2026-09-13 from a clean checkout at C1 `3d535ebc` with `go build -trimpath -o ~/.local/bin/sirsi.new ./cmd/sirsi` then atomic `mv`; no `vcs.revision` in the binary, so lineage is Ra-reported, not receipted (D5 still open) | hash+toolchain verified; carries `router acknowledge` (v22 client); live-verify receipt: see Change Log 2026-09-13 (merge) |
| C3b | M5 `~/.local/bin/sirsi` | sha256 `59688f20bcbd490a2f6a5204cc30b3d0f7ffbc19d307d9f9d8c03af3ae72cc15` (go1.26.2, arm64, trimpath) — **rolled BACK to this on 2026-09-13** from the C1-`3d535ebc` build `2ea58a1c…` (withdrawn: rs-38 `mkdirTrusted` regression crash-looped all 9 wake loops); retained as `~/.sirsi/candidates/sirsi-pre-3d535ebc` | *reconstructed*: pre-#753-era build (Sep 12 03:42), lineage not receipted; the recipe previously listed `3419d20d…` here, which was already stale | hash verified; **live-verified** 2026-09-13 04:22Z — relay pid 47925 forwards 200, all 9 wake loops running exit 0 on this binary. Re-roll to the rs-38 fix once merged. |

### Service
| # | Component | Identity | Provenance | Evidence state |
|---|-----------|----------|-----------|----------------|
| C4 | Service image | `us-central1-docker.pkg.dev/sirsi-nexus-live/cloud-run-source-deploy/sirsi-router@sha256:bf3fbd307966a253bd6c77e479a82343f955553844ac9e480bfa9cc3d49bde3c` (previous `sha256:39eac505…`) | *reconstructed*: built 2026-09-13 by `deploy.sh --source .` (Cloud Build) from the clean M5 worktree at C1 **`3d535ebc`** — server code changed in this C1 (routerstore `AckItem`, `postgresSchemaVersion` 22). Builder/runtime image digests still not pinned (D3); the C1→C4 edge remains a last-code-change marker, not a source-build receipt. | deployed (image digest verified from `deploy.sh` output; lineage reconstructed) |
| C5 | Service revision | `sirsi-router-00013-nhn`, 100% traffic (latestRevision) | C4 image: `00012-pfx` was the 2026-09-13 `deploy.sh` rollout; `00013-nhn` is that image + `gcloud run services update --min-instances=1` re-asserted immediately after (D1: the live override is the authority). Previous: `00011-x7t` | verified — serving; startup probe passed, `router serve: listening on :8080` with the postgres store open (revision logs); M1 `sirsi router status ra` returned 114 open / 7428 closed and `audience --since 30m` returned (2026-09-13 ~08:25Z) |
| C6 | Gate mode | `SIRSI_ROUTER_RULE_OF_RA` **unset ⇒ default `log`** (observe, never refuse) | serve.go default; not in the service env | verified — audit reports `mode: log` |
| C11 | TLS pin | SPKI `78rPvnhm1Lb3jziI2hDDogyku5XoaVABHemUnWwOd7M=` | `deploy.sh` release manifest | verified (openssl from M5) |

### Ledger, roles & jobs (recovery-critical)
| # | Component | Identity | Authority / provenance | Evidence state |
|---|-----------|----------|------------------------|----------------|
| C7 | Schema | Cloud SQL db `router` @ version **22** (`items.acked_at` present; `audience_log`, `project_id`/`router_namespace` present) | `apply-schema-job.sh` (transactional, re-runnable) `pg/schema.sql` from C1 `3d535ebc` | applied + verified — exec `sirsi-router-apply-schema-mlccq` (2026-09-13T08:17Z) asserted `tables=16 triggers=12 partial=5 version=22`, then exited 1 on **FAIL-shape** because the script's own pin still read `version = 20` (stale since v21) — the schema landed; the pin is corrected in the rs-38 change. Prior receipt: exec `…-ndlkn` (v20, 2026-09-10) |
| C8 | Ledger instance | Cloud SQL `sirsi-router`, `POSTGRES_16`, private IP `10.95.0.3` | `provision.sh` | verified |
| C8r | DB roles | `router_migrator` (owns DDL; needs CREATEROLE + CREATEDB + **ADMIN OPTION on `router_service`**), `router_service` (DML-only, no CREATEROLE/CREATEDB) | `pg/roles.sql`; ADMIN OPTION granted once via the grant job below | verified — closed privilege audit in exec `…-ndlkn` |
| C12 | apply-schema job | Cloud Run job `sirsi-router-apply-schema`, image `postgres:16-alpine`, SA **`sirsi-router-schema@sirsi-nexus-live.iam.gserviceaccount.com`** (roles/cloudsql.client + secretAccessor on its three secrets), mounts `sirsi-router-schema-sql`, `sirsi-router-router-migrator-password`, `sirsi-router-router-service-password` | `apply-schema-job.sh` | applied — last exec `sirsi-router-apply-schema-mlccq` (2026-09-13T08:17Z; applied v22 then exited 1 on the script's stale `version = 20` shape pin — see C7); previous `…-ndlkn` |
| C13 | grant job | Cloud Run job `sirsi-router-grant-admin`, `postgres:16-alpine`, same SA, mounts `sirsi-router-grant-sql` + `sirsi-router-postgres-password` | `scripts/router-service/` grant flow (owner-run 2026-09-10) | applied — grant `router_service TO router_migrator WITH ADMIN OPTION` landed; ALTER of NOCREATEDB/NOCREATEROLE completed by the re-run apply-schema |
| C9 | Relay (per host) | LaunchAgent `ai.sirsi.router.relay`, 0600 plist; running M5 pid observed, running M1 | `sirsi router relay install` (running-binary only) | deployed + verified — M5 relay forwarding (horus-supervisor/codex-inference/SSA lanes, HTTP 200) |

### Secrets (Secret Manager, `sirsi-nexus-live` — identities only, never values)
| Secret | Consumer | Verified how |
|--------|----------|--------------|
| `sirsi-router-bootstrap-token` | C5 service (`SIRSI_ROUTER_SERVE_TOKEN`) | verified — C5 boots |
| `sirsi-router-service-dsn` | C5 service (`SIRSI_ROUTER_STORE`) | verified — C5 boots |
| `sirsi-router-router-migrator-password` | C12 apply-schema job | *unverified by service boot* — used only by the separate job (last exec `…-ndlkn` succeeded) |
| `sirsi-router-router-service-password` | C12 apply-schema job | *unverified by service boot* — as above |
| `sirsi-router-schema-sql` | C12 apply-schema job | *unverified by service boot* |
| `sirsi-router-postgres-password`, `sirsi-router-grant-sql` | C13 grant job | *unverified by service boot* — used only by the grant job |

SA note: the service runs as `sirsi-router-svc@sirsi-nexus-live.iam.gserviceaccount.com`; the jobs run as `sirsi-router-schema@…` — distinct identities.

## 2. Provenance chain

```
C1 source (bd5a4614)
  ├─ go build -trimpath (M1 C2a) ─▶ C3a (b1911d04…) ─install─▶ M1  ─speaks to─▶ C5
  ├─ go build -trimpath (M5 C2b) ─▶ C3b (3419d20d…) ─install─▶ M5, relay
  └─ Dockerfile (C2s, golang:1.25-alpine→distroless) at server commit 397eb638 ─▶ C4 (sha256:39eac5…) ─▶ C5 (00011-x7t) ─serves─▶ ledger
apply-schema-job (C12, SA sirsi-router-schema@) with pg/schema.sql,roles.sql ─▶ C7 schema v20 + C8r roles on C8 instance
grant job (C13) ─▶ ADMIN OPTION on router_service (recovery prerequisite for C12's ALTER)
```

## 3. Evidence-state receipts

Each labelled by kind. A resolvable receipt is a run/exec ID or a file path; a timestamp alone or a runnable command is not a receipt.

- **compiled** — per-PR hosted CI on each merge commit: `#732` (`397eb638`, server/gate → C4), `#734`/`#735` (client → C3). *Exact CI run IDs are not transcribed here* (pending — retrievable via `gh run list` per commit); the merge itself is the gate.
- **applied (schema v20)** — resolvable: job execution **`sirsi-router-apply-schema-ndlkn`** (2026-09-10T19:04:21Z); its logs asserted 16 tables / 12 triggers / version 20 / `router_service` DML-only. This is the receipt for **C7 schema v20** and **C8r role closure**.
- **deployed (service code → C4)** — resolvable historical receipt: `docs/evidence/ADR-062-RS22G-RULE-OF-RA-SERVICE-ROLLOUT-20260910.md`. **Scope caution:** that document records revision **`00008-dtt`**, **schema 19**, exec **`4bqj2`**, binaries from **`20c9db60`**, and *unresolved* privilege closure — it is the evidence for the **initial log-mode rollout**, NOT for the current 00011/schema-20/closed-privilege state. It stands as history only. The current server code (C4, from `#732` `397eb638`) landed at revision **`00010-s6c`**; **`00011-x7t`** is C4's image + my `min-instances=1` update.
- **deployed (current revision 00011)** — *Ra-reported, retained receipt pending*: I observed `00011-x7t` serving 100% traffic and the closed privilege audit (exec `…-ndlkn`) this session; a dedicated 00011 evidence doc is not yet written (residual in `rs-27`).
- **verified (live, host-scoped, Ra-reported this session 2026-09-11 ~02:30–02:40Z)** — M1 `sirsi router status ra` and `sirsi router audience --since 30m` returned; M5 spool round-trip for the `codex-pantheon` lane forwarded MintSession+Inbox HTTP 200. Per-request/per-run IDs not retained here (residual in `rs-27`).

## 4. Build & maintenance procedure (recipe → build → deploy → verify → receipt)

1. **Recipe update** — branch off C1; name which components change.
2. **Compile** — full `-race -short` on the three packages + lint; land via PR + SSA bind + CI (never `--no-verify`).
3. **Apply (schema changed)** — from a clean `origin/main` checkout on the M5: `bash scripts/router-service/apply-schema-job.sh` (transactional, re-runnable; asserts shape + closed privilege audit); capture the exec id. Recovery prerequisite: C8r `router_migrator` must hold CREATEROLE+CREATEDB+ADMIN OPTION on `router_service` (the C13 grant job establishes ADMIN OPTION once, as the Cloud SQL `postgres` superuser).
4. **Deploy (server code changed)** — from the M5 `origin/main` checkout: `bash scripts/router-service/deploy.sh`; **then immediately check traffic** (`gcloud --project=sirsi-nexus-live run services describe sirsi-router --region=us-central1 --format='value(status.traffic)'`) — a named-revision pin from a rollback rehearsal silently keeps the OLD revision serving (this bit us 00007→00008).
5. **Roll the client (CLI changed)** — rebuild per host; **`rm` then `cp`** into `~/.local/bin/sirsi` (never `cp` over the live binary → SIGKILL/exit 137); restart relay + wake loops so they run the new binary. Record the new per-host hash in C3.
6. **Verify** — live audit + a positive control; confirm no new would-refuse class.
7. **Receipt** — update §1 + Change Log; add a `docs/evidence/` doc for a material change; update the ledger task (§6).
8. **Rollback** —
   - **Service** (quoted; name the known previous revision): `gcloud --project=sirsi-nexus-live run services update-traffic sirsi-router --region=us-central1 --to-revisions="sirsi-router-00010-s6c=100"` (the revision before C5; `00010-s6c` is C4's image without the `min-instances=1` override).
   - **Client** (retained candidate, full hash, host scope) — verify the candidate, stage it, then **atomic rename** (never `cp` over the executing binary → SIGKILL/exit 137; `mv` on the same filesystem swaps the inode and the running process keeps its old one until exit). On the M1, to roll back to candidate `sirsi-f72a3aaf`:
     ```sh
     cand="$HOME/.sirsi/candidates/sirsi-f72a3aaf"
     [ "$(shasum -a 256 "$cand" | cut -d' ' -f1)" = 7dd994bab3aea047b537e5e11972c8e629dda212b9f25ef32b3f721cf9fa21b4 ] \
       && cp "$cand" "$HOME/.local/bin/sirsi.new" \
       && chmod 755 "$HOME/.local/bin/sirsi.new" \
       && mv "$HOME/.local/bin/sirsi.new" "$HOME/.local/bin/sirsi" \
       && echo "rolled back to sirsi-f72a3aaf" \
       || echo "ABORTED — candidate missing or hash mismatch; installed binary untouched"
     ```
     The installed binary is only replaced after the candidate is confirmed present and hash-matched, so a bad candidate never leaves the host without a router client. The M5 keeps its own `~/.sirsi/candidates/` set (paths/hashes not catalogued here — pending, `rs-27`); use the same verify→stage→rename flow with the M5 candidate hash.
   - **Schema** is forward-only (re-runnable, never destructive).

## 5. Observability (read the live state)

- Service traffic: `gcloud --project=sirsi-nexus-live run services describe sirsi-router --region=us-central1 --format='value(status.traffic)'`
- Gate signal: `SIRSI_AGENT_ID=ra SIRSI_THREAD_ID=<thread> sirsi router audience --since 24h`
- Ledger: `sirsi router status ra` / `sirsi router board`
- Relay: `~/.sirsi/logs/router-relay.log`

## 6. Ledger binding & publication

- Tracked as ra task `rs-27-router-stack-lab-recipe` (recipe = task; provenance = links).
- **Build Recipe Contract:** this recipe adopts the intent of `SIRSI_BUILD_RECIPE_CONTRACT_V1.md` (owner-ratified, owned by `codex-inference`). It is **not in this repo**; the sync dependency is `codex-inference` mirroring it to the canonical repo. This recipe links it directly once mirrored; linking it in its own canonical location can precede a Pantheon mirror.
- **Publication state:** canonical repo = this file (on merge — the authority). Owner Reading Room (Desktop) mirror = routine sync by `ra` post-merge (not a new permission gate). Google Workspace copy = pending the Workspace share dependency (owner-held share to the `claude-agent` SA; recorded as not-yet-published, not claimed).

## 7. Reconciliation — recipe-vs-reality drift (open)

| Drift | Recipe/target | Reality | Action |
|-------|---------------|---------|--------|
| D1 Service scale | `deploy.sh --min-instances=0` | live `min-instances=1` (warm mitigation for cold-start ListAll) | Return to 0 after `rs-26` (server-side bounded read) + measured cold-start evidence; live override is the authority until then. |
| D2 Gate mode | target `enforce` | live `log` | Flip to `enforce` only after a clean 24h audit; blocked by `rs-22h` (codex-lane + interactive-shell registration). |
| D3 Server build pinning | pinned builder/runtime digests | `Dockerfile` uses mutable tags `golang:1.25-alpine` + `distroless/…:nonroot`; digests not captured | Capture and pin the resolved image digests on the next deploy; until then C2s/C4 server build inputs are *reconstructed*, not fully evidenced. |
| D4 Per-host client parity | (informational) | M1 `b1911d04…`/go1.27.1 vs M5 `3419d20d…`/go1.26.2 | Expected (per-host toolchains); recorded so a hash mismatch is read as drift, not tampering. |
| D5 Build receipts | full source→binary/image receipts (dirty state, command, output hash) | not captured; binaries carry no `vcs.revision`; C1→C3/C4 lineage is reconstructed | Add a build-receipt step (record commit, clean/dirty, `go build` invocation, output hash per host) to the maintenance procedure; then C3/C4 lineage moves verified. Residual `rs-27`. |
| D6 Current-revision + live receipts | resolvable 00011 evidence doc + retained CI/per-request IDs | Ra-reported this session, not yet a written doc | Write a 00011 rollout evidence doc + transcribe CI run IDs / live-verify request IDs. Residual `rs-27`. |

## Change Log

- 2026-09-13 (deploy) — **C1 `3d535ebc` rolled to C7/C12, C4/C5, and attempted on C3b; C3b REGRESSED and was rolled back** (rs-38). Order run: (1) `apply-schema-job.sh` → exec `sirsi-router-apply-schema-mlccq`: SQL applied in one transaction, `tables=16 triggers=12 partial=5 version=22` — then exit 1 `FAIL-shape` because the script's own pin still read `version = 20` (stale since v21). Schema is v22 (verified by a second read); the pin is corrected to 22 in this change. (2) `deploy.sh` → `00012-pfx`, then `00013-nhn` after re-asserting `--min-instances=1`; image `sha256:bf3fbd30…`; 100 % traffic; `sirsi router status ra` served by the new revision (verified in revision logs). (3) M5 client built from clean worktree `~/.sirsi/worktrees/main-3d535ebc` at C1 → `2ea58a1c…`, atomic `mv`, relay + 9 lanes kickstarted → **all 9 wake loops crash-looped**: `spool: chmod /Users/thekryptodragon/.sirsi/relay/Mac: operation not permitted`. Root cause: `mkdirTrusted` chmod'd every directory on the spool path, but `~/.sirsi/relay` on the M5 is a symlink into `/var/sirsipantheon/relay` whose `Mac/` is owned by `_sirsipantheon`; chmod requires ownership, MkdirAll on an existing dir is a no-op, so the old binary never hit it and the new one (which walks the path) did. **Rollback:** atomic `mv` of candidate `sirsi-pre-3d535ebc` (`59688f20…`) back over `~/.local/bin/sirsi`, kickstart relay + 9 lanes → all `state = running`, last exit 0, relay forwarding 200 — verified >60 s after kickstart. **Fix:** `mkdirTrusted` stats first and only chmods a directory it created (a pre-existing dir keeps its mode; the gid-mismatch refusal is unchanged); regression test `TestMkdirTrustedNeverChmodsAPreExistingDir` with a valid negative control (original body fails at "pre-existing dir was chmod'd: 770"). Row C3b describes the rolled-back state until the re-roll receipt lands.
- 2026-09-13 (merge) — **PR #754 merged as C1 `3d535ebc`** (squash; owner waived the SSA bind in-session; CI green on head `d85f8fe`, all five checks; PG leg green locally at schema **v22**). Lands: body-loss guard (rs-34, fenced-complete), `sirsi router acknowledge <id>` (recipient-only read-ack, `acked_at`, schema v22 SQLite + pg — the rs-35 gap, fenced-complete), ADR-065 as a **Proposed** design (rs-37 still blocked on SHA+SSA verdict; no informer code). **C3a rolled** on the M1 to `341b7ae0…` (go1.27.1, trimpath; atomic `mv`; previous binary retained as candidate `sirsi-pre-3d535ebc`). **Pending in this same rollout** (recipe §4 steps 3–6): M5 client roll + relay kickstart (C3b), `apply-schema-job.sh` for v22 (C7 → 22, C12 exec id), `deploy.sh` for the server code (C4/C5 → new revision; check traffic), live verify; rows C3b/C4/C5/C7 update when each receipt exists — until then those rows describe the PRE-merge state.
- 2026-09-13 (later) — **ADR-065 proposed** (`docs/ADR-065-ROUTER-OWNED-INFORMER-LANES-CARRY-NO-ARMING-LOGIC.md`; ledger `rs-37`, blocked-by `rs-36`). Owner decision after the same-day review cycle exposed one defect shape four ways (seven of nine M5 wake plists wrong, a wrong `.zshenv` fallback, SSA 15 commits stale with the canon'd bundle handoff never operated, a SHA reply invisible to `ra`'s pull beside a live second registry): the logic deciding whether a lane is reachable lives in each lane. Decision: one router-owned watcher/informer per host subscribes for every registered agent over the existing `ListenNotify`/`Wait` edge signal and pushes to lanes; per-lane `ai.sirsi.router.wake.<agent>` LaunchAgents retired; safety gates (#636/#642/ceiling/quarantine) relocated intact per A29/Rule 0; push acknowledged as `read_at` (A2A property 6, the rs-35 gap); arming strategy chosen from the lane's declared agent type in one router-side table; connection is the lane's contract (register truthfully + stay demonstrably connected, else surfaced as the lane's defect). Recipe impact when implemented: C9 (relay) gains the informer as a sibling load-bearing component; the nine wake plists leave the inventory; `AgentConfig` gains a declared `Delivery`. Awaiting SHA + SSA bind — nothing in §1 changes until then. The `A2A_CONTRACT_ASSESSMENT.md` conclusion is re-scoped by its same-day addendum (store sound; delivery not guaranteed).
- 2026-09-13 — SHA (sirsi-hardware-admin) item `20260913-043215` closed in two parts. **rs-34** (PR #754, not yet merged as of this entry): the router body-loss regression — `loadOrLiteral` refused only long inline bodies, not ones a shell substitution collapsed to empty (`--instructions "$(true)"` arrives as `""`, under the length limit, stored with a valid title and a silently blank body). Fixed with a Changed()-gated wrapper so a legitimately omitted flag (idempotent no-op close) stays legal while a passed-but-empty one is refused; `router send`'s `--instructions` made explicitly required. Negative control run (reverted fix, confirmed the 3 new tests fail; restored, confirmed pass). **rs-35**: the A2A contract assessment SHA also requested — see `A2A_CONTRACT_ASSESSMENT.md` (new file, same directory). Five of six named properties (identity, idempotency, correlation, error semantics, and now body integrity) were already implemented and verified against the actual code; the one gap (delivery/read acknowledgement) is recommended as a one-field, one-verb hardening of the existing transport, not a new protocol — sent to SHA, not yet implemented.
- 2026-09-11 (rev 3) — self-audited to the Stack Lab rubric (owner: adopt the rubric as own practice) and closed SSA item 20260911-024401: reframed the promise from "reproduce from this file alone" to an **operating inventory with explicit gaps**; C3a/C3b/C4 lineage labelled *reconstructed* (hash+toolchain verified, C1 lineage not receipted — no `vcs.revision`); §3 receipts corrected — the RS22G doc is HISTORICAL (`00008`/schema 19/unresolved privilege), current 00011 is Ra-reported (retained receipt pending), compiled cites per-PR CI; executable rollback: service `--to-revisions="sirsi-router-00010-s6c=100"` (quoted, named); client = **verify hash → stage → atomic `mv`** with the retained candidate `sirsi-f72a3aaf` + hash (no angle-bracket placeholder; never `cp` over the live inode; installed binary replaced only after the candidate is confirmed — tested on disposable fixtures: correct hash replaces, wrong hash leaves it untouched, no leftover); publication ownership named (contract → codex-inference; Desktop → routine ra sync; Workspace → share dependency); drift D5 (build receipts) + D6 (current/live receipts) added, residuals in `rs-27`.
- 2026-09-11 (rev 2) — corrected per SSA (item 023457): fixed secret names; added DB roles C8r + apply-schema C12 + grant C13; split toolchains; full hashes + image URI; host scope; drift D3/D4.
- 2026-09-11 (rev 1) — recipe created; captured C1–C11 at main `bd5a4614`, rev `00011-x7t`, schema v20, gate `log`.
