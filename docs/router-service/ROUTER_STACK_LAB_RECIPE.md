<!-- agent: ra | workstream: router-service (ADR-062) | recipe revision: 2 | authority: this file is the SINGLE authority for the router runtime recipe; screenshots, dashboards and summaries are projections of it. -->

# Router Service — Stack Lab Recipe

**Status:** Active operating recipe (revision 2)
**Owner:** `ra` (router / worker-plane; ADR-062, ADR-063)
**Classification:** Core platform foundation — the router is critical infrastructure; if it fails, many threads' business goes undone. It is therefore built and maintained under Stack Lab methodology: a reproducible recipe assembled from *identified* components, not an untraceable build.
**Observation:** all live identities below captured 2026-09-11 ~02:30–02:40 UTC from the M1 (`sirsimasterdev`) and M5 (`thekryptodragon`) hosts and from `gcloud` (project `sirsi-nexus-live`, region `us-central1`), unless a row is marked *reconstructed* or *unverified*.

## Why this recipe exists

The router is a runtime: a Cloud Run service, a Cloud SQL ledger, per-host client binaries, and per-host relays. This file makes every component a named, hash-identified part with a distinct evidence state, so the running system can be reproduced, audited, and recovered from this document alone. A reader must be able to *reproduce or recover from this file* — so every identity is full, every receipt is a resolvable path/ID, and anything not directly observed is labelled.

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
| C3a | M1 `~/.local/bin/sirsi` | `b1911d04eef3a01f89121f963e940d9b2ca71550a049e174340046bb4c166884` | C1 + C2a, `go build -trimpath ./cmd/sirsi` | deployed + verified (M1 `sirsi router status ra` reads C5) |
| C3b | M5 `~/.local/bin/sirsi` | `3419d20d2548e52eb646da1e1c4351b68de84d0447abfecd2bec85690c003f62` | C1 + C2b | deployed + verified (M5 spool round-trip forwards; relay runs this binary) |

### Service
| # | Component | Identity | Provenance | Evidence state |
|---|-----------|----------|-----------|----------------|
| C4 | Service image | `us-central1-docker.pkg.dev/sirsi-nexus-live/cloud-run-source-deploy/sirsi-router@sha256:39eac505abc4e83b1d0aab500679c3865011ebf233b67824c0944cd0ac7f71a6` | `deploy.sh --source .` (Cloud Build) at server-code commit **`397eb638`** — the last commit that changed server code (`#732` idle-gate); C1's later commits (`#734` thoth, `#735` client per-call) are client-only, so C4 is current for server behaviour | deployed. *Source edge is `397eb638`→C4, not today's `bd5a4614`; the full server build inputs are not otherwise captured* |
| C5 | Service revision | `sirsi-router-00011-x7t`, 100% traffic (latestRevision) | C4 image + `run services update --min-instances=1` on rev `00010-s6c` | verified — serving; audit query and `status` return |
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

## 3. Evidence-state receipts (resolvable)

- **compiled:** hosted CI on the merge commit (per PR) + local `go test -race -short ./internal/routerstore ./internal/router ./cmd/sirsi` + `golangci-lint`.
- **applied (schema):** job execution **`sirsi-router-apply-schema-ndlkn`** (2026-09-10T19:04:21Z), assertions in its logs.
- **deployed (service):** revision `sirsi-router-00011-x7t`; rollout evidence `docs/evidence/ADR-062-RS22G-RULE-OF-RA-SERVICE-ROLLOUT-20260910.md` (+ retained would-refuse log under its sibling directory).
- **verified (live, host-scoped):** M1 `sirsi router status ra` and `sirsi router audience --since 30m` return (2026-09-11 ~02:30Z); M5 spool round-trip for a lane forwards HTTP 200.

## 4. Build & maintenance procedure (recipe → build → deploy → verify → receipt)

1. **Recipe update** — branch off C1; name which components change.
2. **Compile** — full `-race -short` on the three packages + lint; land via PR + SSA bind + CI (never `--no-verify`).
3. **Apply (schema changed)** — from a clean `origin/main` checkout on the M5: `bash scripts/router-service/apply-schema-job.sh` (transactional, re-runnable; asserts shape + closed privilege audit); capture the exec id. Recovery prerequisite: C8r `router_migrator` must hold CREATEROLE+CREATEDB+ADMIN OPTION on `router_service` (the C13 grant job establishes ADMIN OPTION once, as the Cloud SQL `postgres` superuser).
4. **Deploy (server code changed)** — from the M5 `origin/main` checkout: `bash scripts/router-service/deploy.sh`; **then immediately check traffic** (`gcloud --project=sirsi-nexus-live run services describe sirsi-router --region=us-central1 --format='value(status.traffic)'`) — a named-revision pin from a rollback rehearsal silently keeps the OLD revision serving (this bit us 00007→00008).
5. **Roll the client (CLI changed)** — rebuild per host; **`rm` then `cp`** into `~/.local/bin/sirsi` (never `cp` over the live binary → SIGKILL/exit 137); restart relay + wake loops so they run the new binary. Record the new per-host hash in C3.
6. **Verify** — live audit + a positive control; confirm no new would-refuse class.
7. **Receipt** — update §1 + Change Log; add a `docs/evidence/` doc for a material change; update the ledger task (§6).
8. **Rollback** — service: `gcloud --project=sirsi-nexus-live run services update-traffic sirsi-router --region=us-central1 --to-revisions <prev-rev>=100`; client: reinstall the previous candidate from `~/.sirsi/candidates/`; schema is forward-only (re-runnable, never destructive).

## 5. Observability (read the live state)

- Service traffic: `gcloud --project=sirsi-nexus-live run services describe sirsi-router --region=us-central1 --format='value(status.traffic)'`
- Gate signal: `SIRSI_AGENT_ID=ra SIRSI_THREAD_ID=<thread> sirsi router audience --since 24h`
- Ledger: `sirsi router status ra` / `sirsi router board`
- Relay: `~/.sirsi/logs/router-relay.log`

## 6. Ledger binding & publication

- Tracked as ra task `rs-27-router-stack-lab-recipe` (recipe = task; provenance = links).
- **Build Recipe Contract:** this recipe adopts the intent of `SIRSI_BUILD_RECIPE_CONTRACT_V1.md` (owner-ratified, owned by `codex-inference`). That contract file is **not yet mirrored into this repo**; pending owner: `codex-inference` to publish it to the canonical repo, after which this recipe links it directly.
- **Publication state:** canonical repo = this file (on merge). Owner Reading Room (Desktop) and Google Workspace copies = **pending** (owner action; recorded here as not-yet-published rather than claimed).

## 7. Reconciliation — recipe-vs-reality drift (open)

| Drift | Recipe/target | Reality | Action |
|-------|---------------|---------|--------|
| D1 Service scale | `deploy.sh --min-instances=0` | live `min-instances=1` (warm mitigation for cold-start ListAll) | Return to 0 after `rs-26` (server-side bounded read) + measured cold-start evidence; live override is the authority until then. |
| D2 Gate mode | target `enforce` | live `log` | Flip to `enforce` only after a clean 24h audit; blocked by `rs-22h` (codex-lane + interactive-shell registration). |
| D3 Server build pinning | pinned builder/runtime digests | `Dockerfile` uses mutable tags `golang:1.25-alpine` + `distroless/…:nonroot`; digests not captured | Capture and pin the resolved image digests on the next deploy; until then C2s/C4 server build inputs are *reconstructed*, not fully evidenced. |
| D4 Per-host client parity | (informational) | M1 `b1911d04…`/go1.27.1 vs M5 `3419d20d…`/go1.26.2 | Expected (per-host toolchains); recorded so a hash mismatch is read as drift, not tampering. |

## Change Log

- 2026-09-11 (rev 2) — corrected per SSA review (item 20260911-023457): fixed secret names (`sirsi-router-router-{migrator,service}-password`); added DB roles C8r + apply-schema C12 + grant C13 jobs with SA identities and role authority; split client (per-host) vs server toolchains (C2a/C2b/C2s); full binary hashes + full image URI + resolvable receipt paths + observation timestamps + host scope; labelled service-boot-unverified secrets; executable rollback; publication state and Build Recipe Contract pending owner recorded; drift D3/D4 added.
- 2026-09-11 (rev 1) — recipe created; captured C1–C11 at main `bd5a4614`, rev `00011-x7t`, schema v20, gate `log`.
