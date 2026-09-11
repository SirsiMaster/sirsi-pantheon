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
| C1 | Source | `origin/main` @ `bd5a461471e548e127cbe2f0b2f87f12243f6bb0` | GitHub SirsiMaster/sirsi-pantheon | verified — CI green on merge |
| C2a | Client toolchain, M1 | Go `go1.27.1`, `GOARCH=arm64` (from `go version -m ~/.local/bin/sirsi`) | host M1 | verified |
| C2b | Client toolchain, M5 | Go `go1.26.2` (from `go version -m ~/.local/bin/sirsi`) | host M5 | verified |
| C2s | Server toolchain | build `golang:1.25-alpine`, runtime `gcr.io/distroless/static-debian12:nonroot` (repo `Dockerfile`) | Cloud Run source build | *reconstructed from Dockerfile* — both tags are **mutable**; the exact builder/runtime image **digests** are not pinned and not captured (see Reconciliation D3) |

The three toolchains differ on purpose (per-host client builds + a container server build); the binaries are therefore **not** byte-identical across hosts even from the same C1. Recorded, not a defect.

### Client binaries (per host — distinct identities)
| # | Host | sha256 | Built from | Evidence state |
|---|------|--------|-----------|----------------|
| C3a | M1 `~/.local/bin/sirsi` | sha256 `b1911d04eef3a01f89121f963e940d9b2ca71550a049e174340046bb4c166884`; `go version -m`: go1.27.1, arm64, trimpath=true | *reconstructed* C1→C3a (built this session `go build -trimpath ./cmd/sirsi`; the binary carries no `vcs.revision`, so the hash+toolchain are verified but C1 lineage is Ra-reported, not receipted) | hash+toolchain verified; **live-verified** (M1 `sirsi router status ra` reads C5, 2026-09-11 ~02:30Z) |
| C3b | M5 `~/.local/bin/sirsi` | sha256 `3419d20d2548e52eb646da1e1c4351b68de84d0447abfecd2bec85690c003f62`; go1.26.2, arm64, trimpath=true, CGO_ENABLED=1 (independently re-observed by SSA) | *reconstructed* C1→C3b (as C3a, M5 toolchain) | hash+toolchain verified; **live-verified** (M5 spool round-trip forwards HTTP 200; relay runs this binary) |

### Service
| # | Component | Identity | Provenance | Evidence state |
|---|-----------|----------|-----------|----------------|
| C4 | Service image | `us-central1-docker.pkg.dev/sirsi-nexus-live/cloud-run-source-deploy/sirsi-router@sha256:39eac505abc4e83b1d0aab500679c3865011ebf233b67824c0944cd0ac7f71a6` | *reconstructed*: built by `deploy.sh --source .` (Cloud Build) whose last server-code input was commit **`397eb638`** (`#732` idle-gate); C1's later commits (`#734` thoth, `#735` client per-call) are client-only. The *complete* server build inputs (full source tree, resolved builder digest) are NOT captured — the `397eb638`→C4 edge is a last-code-change marker, not a source-build receipt (D3). | deployed (image digest is verified; source lineage reconstructed) |
| C5 | Service revision | `sirsi-router-00011-x7t`, 100% traffic (latestRevision) | C4 image + `gcloud run services update --min-instances=1` (my action, 2026-09-11 ~01:20Z) on rev `00010-s6c`; **00010-s6c** was the `#732` deploy of C4 | verified — serving; `status`/`audience` return (2026-09-11 ~02:30Z, M1) |
| C6 | Gate mode | `SIRSI_ROUTER_RULE_OF_RA` **unset ⇒ default `log`** (observe, never refuse) | serve.go default; not in the service env | verified — audit reports `mode: log` |
| C11 | TLS pin | SPKI `78rPvnhm1Lb3jziI2hDDogyku5XoaVABHemUnWwOd7M=` | `deploy.sh` release manifest | verified (openssl from M5) |

### Ledger, roles & jobs (recovery-critical)
| # | Component | Identity | Authority / provenance | Evidence state |
|---|-----------|----------|------------------------|----------------|
| C7 | Schema | Cloud SQL db `router` @ version **20** (`audience_log` present) | `apply-schema-job.sh` (transactional, re-runnable) `pg/schema.sql` from C1 | applied + verified — exec `sirsi-router-apply-schema-ndlkn` (2026-09-10T19:04Z) asserted 16 tables / 12 triggers / version 20 / `router_service` DML-only |
| C8 | Ledger instance | Cloud SQL `sirsi-router`, `POSTGRES_16`, private IP `10.95.0.3` | `provision.sh` | verified |
| C8r | DB roles | `router_migrator` (owns DDL; needs CREATEROLE + CREATEDB + **ADMIN OPTION on `router_service`**), `router_service` (DML-only, no CREATEROLE/CREATEDB) | `pg/roles.sql`; ADMIN OPTION granted once via the grant job below | verified — closed privilege audit in exec `…-ndlkn` |
| C12 | apply-schema job | Cloud Run job `sirsi-router-apply-schema`, image `postgres:16-alpine`, SA **`sirsi-router-schema@sirsi-nexus-live.iam.gserviceaccount.com`** (roles/cloudsql.client + secretAccessor on its three secrets), mounts `sirsi-router-schema-sql`, `sirsi-router-router-migrator-password`, `sirsi-router-router-service-password` | `apply-schema-job.sh` | applied — last exec `…-ndlkn` |
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

- 2026-09-11 (rev 3) — self-audited to the Stack Lab rubric (owner: adopt the rubric as own practice) and closed SSA item 20260911-024401: reframed the promise from "reproduce from this file alone" to an **operating inventory with explicit gaps**; C3a/C3b/C4 lineage labelled *reconstructed* (hash+toolchain verified, C1 lineage not receipted — no `vcs.revision`); §3 receipts corrected — the RS22G doc is HISTORICAL (`00008`/schema 19/unresolved privilege), current 00011 is Ra-reported (retained receipt pending), compiled cites per-PR CI; executable rollback: service `--to-revisions="sirsi-router-00010-s6c=100"` (quoted, named); client = **verify hash → stage → atomic `mv`** with the retained candidate `sirsi-f72a3aaf` + hash (no angle-bracket placeholder; never `cp` over the live inode; installed binary replaced only after the candidate is confirmed — tested on disposable fixtures: correct hash replaces, wrong hash leaves it untouched, no leftover); publication ownership named (contract → codex-inference; Desktop → routine ra sync; Workspace → share dependency); drift D5 (build receipts) + D6 (current/live receipts) added, residuals in `rs-27`.
- 2026-09-11 (rev 2) — corrected per SSA (item 023457): fixed secret names; added DB roles C8r + apply-schema C12 + grant C13; split toolchains; full hashes + image URI; host scope; drift D3/D4.
- 2026-09-11 (rev 1) — recipe created; captured C1–C11 at main `bd5a4614`, rev `00011-x7t`, schema v20, gate `log`.
