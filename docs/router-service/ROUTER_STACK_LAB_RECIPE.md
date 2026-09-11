<!-- agent: ra | workstream: router-service (ADR-062) | authority: this file is the SINGLE authority for the router runtime recipe; screenshots, dashboards and summaries are projections of it. -->

# Router Service — Stack Lab Recipe

**Status:** Active operating recipe
**Owner:** `ra` (router / worker-plane; ADR-062, ADR-063)
**Classification:** Core platform foundation — the router is critical infrastructure; if it fails, many threads' business goes undone. It is therefore built and maintained under Stack Lab methodology: a reproducible recipe assembled from *identified* components, not an untraceable build.

## Why this recipe exists

The router is a runtime (a Cloud Run service + a Cloud SQL ledger + a per-host client binary + per-host relays). Before this recipe it was operated by ad-hoc `gcloud`/`go build` steps whose exact component identities lived only in shell scrollback. This file makes every component a named, hash-identified part with a distinct evidence state, so the running system can be reproduced, audited, and recovered from this document alone.

## Stack Lab rules applied here

- **One recipe, one current build state.** This file names the *current* identity of every component. A superseded identity moves to the Change Log, not a parallel doc.
- **One authority per fact.** Source commit, binary hash, image digest, revision, schema version, host, and evidence live here. The live cloud state must match this file or the mismatch is recorded in §7 Reconciliation.
- **Distinct evidence states.** Each component is proven at three separate states — **compiled**, **deployed/applied**, **verified** — each with a concrete receipt. A green build is evidence of the *compiled* state only.
- **No sprawl.** No parallel build trees; a retired revision is a Change-Log line + its Cloud Run revision (retained by the platform), not a duplicate.
- **Durable receipts.** Every material state change (deploy, schema apply, gate-mode flip, scale change) is a router item or an evidence doc under `docs/evidence/`.
- **Identified payloads, not copies.** Secrets are referenced by Secret Manager name, never value. Images and schema are referenced by digest/version, never re-pasted.

## 1. Components (current identities — captured 2026-09-11)

| # | Component | Identity | Provenance (built/derived from) | Evidence state |
|---|-----------|----------|----------------------------------|----------------|
| C1 | Source | `origin/main` @ `bd5a461471e548e127cbe2f0b2f87f12243f6bb0` | GitHub SirsiMaster/sirsi-pantheon | verified (CI green on merge) |
| C2 | Toolchain | Go `go1.27.1` | host `go version` | verified |
| C3 | Client binary `sirsi` | sha256 `b1911d04eef3a01f…` (M1); built `go build -trimpath ./cmd/sirsi` from C1 | C1 + C2 | deployed (installed `~/.local/bin/sirsi` on M1 + M5); verified (talks to C5) |
| C4 | Service image | `…/cloud-run-source-deploy/sirsi-router@sha256:39eac505abc4e83b1d0aab500679c3865011ebf233b67824c0944cd0ac7f71a6` | built by `scripts/router-service/deploy.sh --source .` from commit `397eb638` (last server-code change: the idle-gate fix #732; #734/#735 are client-only, so the image is current) | deployed |
| C5 | Service revision | `sirsi-router-00011-x7t`, 100% traffic | C4 image + `run services update --min-instances=1` on rev `00010-s6c` | verified (serving; audit query returns) |
| C6 | Gate mode | `SIRSI_ROUTER_RULE_OF_RA` **unset ⇒ default `log`** (observe, never refuse) | serve.go default | verified (audit shows `mode: log`) |
| C7 | Schema | Cloud SQL `router` @ version **20** (`audience_log` present) | `scripts/router-service/apply-schema-job.sh`, exec `sirsi-router-apply-schema-ndlkn` (2026-09-10T19:04Z) | applied + verified (job audit: tables=16, version=20, router_service DML-only) |
| C8 | Ledger instance | Cloud SQL `sirsi-router`, POSTGRES_16, private IP `10.95.0.3` | `scripts/router-service/provision.sh` | verified |
| C9 | Relay (per host) | LaunchAgent `ai.sirsi.router.relay`, 0600 plist, running on M5 (and M1) | `sirsi router relay install` (running-binary only) | deployed + verified (forwarding) |
| C10 | Secrets (identities only) | `sirsi-router-bootstrap-token`, `sirsi-router-service-dsn`, `sirsi-router-{migrator,service}-password`, `sirsi-router-schema-sql`, `sirsi-router-postgres-password`, `sirsi-router-grant-sql` | Secret Manager (sirsi-nexus-live) | verified (mounted; C5 boots) |
| C11 | TLS pin | SPKI `78rPvnhm1Lb3jziI2hDDogyku5XoaVABHemUnWwOd7M=` | `deploy.sh` release manifest | verified |

## 2. Provenance chain

```
C1 source (bd5a4614)
  ├─ go build -trimpath ──▶ C3 client binary (b1911d04…) ──install──▶ M1, M5  ──speaks to──▶ C5
  └─ deploy.sh --source . (server code @397eb638) ──▶ C4 image (sha256:39eac5…) ──▶ C5 revision (00011-x7t) ──serves──▶ ledger
apply-schema-job.sh (C1 pg/schema.sql) ──▶ C7 schema v20 on C8 instance
provision.sh ──▶ C8 instance, C10 secrets, service accounts
```

## 3. Evidence states — how each is proven

- **compiled:** `go test -race -short ./internal/routerstore ./internal/router ./cmd/sirsi` + `golangci-lint` + hosted CI on the merge commit.
- **applied (schema):** `apply-schema-job.sh` runs transactionally, asserts shape (16 tables / 12 triggers / version 20) and the closed `router_service` privilege audit; receipt = job execution id.
- **deployed (service):** `deploy.sh` prints revision + image digest + SPKI pin; receipt = the revision name and `docs/evidence/ADR-062-RS22G-…`.
- **verified (live):** `SIRSI_AGENT_ID=ra SIRSI_THREAD_ID=<thread> sirsi router audience --since 30m` returns and `sirsi router status ra` reads; the gate's own audit log is the running receipt.

## 4. Build & maintenance procedure (recipe → build → deploy → verify → receipt)

Every change to the router runtime follows this, and updates §1 + the Change Log:

1. **Recipe update** — edit source (C1) on a branch; state which component(s) change.
2. **Compile** — full `-race -short` on the three packages + lint; land via PR + SSA bind + CI (never `--no-verify`).
3. **Apply (if schema changed)** — run `apply-schema-job.sh` from a clean `origin/main` checkout on the M5 (gcloud auth lives there); it is transactional and re-runnable; capture the exec id.
4. **Deploy (if server code changed)** — `deploy.sh` from the M5 `origin/main` checkout; **immediately check `spec.traffic`** (a named-revision pin from a rollback rehearsal will silently keep the old revision serving — this bit us on rev 00007→00008).
5. **Roll the client (if CLI/client changed)** — rebuild + `rm`-then-`cp` into `~/.local/bin/sirsi` on both Macs (never `cp` over the live binary — SIGKILL); restart the relay/wake loops so they run the new binary.
6. **Verify** — run the live audit + a positive control; confirm zero *new* would-refuse classes.
7. **Receipt** — update this file's §1 and Change Log; add a `docs/evidence/` doc for a material change; register/update the ledger task (§6).
8. **Rollback** — `run services update-traffic --to-revision <prev>` (service) or reinstall the previous candidate binary (client); the schema is forward-only (re-runnable, never destructive).

## 5. Observability (read the live state — no dev shell required)

- Service: `gcloud run services describe sirsi-router --region=us-central1 --format='value(status.traffic)'`
- Gate signal: `sirsi router audience --since 24h` (two tables: mutations unregistered-at-mutation-time; live threadless sessions).
- Ledger: `sirsi router status ra` / `sirsi router board`.
- Relay: the LaunchAgent's log (`~/.sirsi/logs/router-relay.log`).

## 6. Ledger binding

This recipe is tracked as ra task `rs-27-router-stack-lab-recipe` (lifecycle + provenance), per the same task-registry convention offered to `codex-inference` for Stack Lab: recipe = task, provenance = links, material receipts = items.

## 7. Reconciliation — current recipe-vs-reality drift (open)

| Drift | Recipe says | Reality | Action |
|-------|-------------|---------|--------|
| Service scale | `deploy.sh --min-instances=0` | live `min-instances=1` (warm mitigation for cold-start ListAll) | Return to 0 after `rs-26` (server-side bounded read) + measured cold-start evidence; until then the live override is the authority and is recorded here. |
| Gate mode | design target `enforce` | live `log` | Flip to `enforce` (set `SIRSI_ROUTER_RULE_OF_RA=enforce`) only after a clean 24h audit; blocked by `rs-22h` (codex-lane + interactive-shell registration). |

## Change Log

- 2026-09-11 — recipe created; captured C1–C11 at main `bd5a4614`, rev `00011-x7t`, schema v20, gate `log`. Drift recorded: min-instances (live 1 vs recipe 0), gate mode (log vs target enforce).
