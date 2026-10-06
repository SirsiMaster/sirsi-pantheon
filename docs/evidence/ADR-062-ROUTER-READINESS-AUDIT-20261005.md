# Router readiness audit — G1 to G12 (2026-10-05)

Auditor: Ra. Revision: origin/main `c1a78077` plus the changes in this PR. Each row names what was run or read today. A row is **Met** only when its evidence is in hand; older receipts are cited with their date and are not re-run claims.

**Met 7 of 12 (G11 once this PR merges), Partial 3 (G3 in process only, G6, G7), Open 2 (G9, G12).**

| # | Condition | State | Evidence |
|---|---|---|---|
| G1 | Only `routerstore.Resolve()` opens the store | **Met** | `bash scripts/check-router-store-open.sh` exits 0; `--self-test` plants an `OpenPath()` call and reports it detected. Gate runs in `.githooks/pre-push` and CI lint. |
| G2 | Same suites on SQLite and Postgres | **Met** | CI job Test runs `scripts/ci-postgres.sh` (whole `internal/routerstore` suite on PostgreSQL 16, fail-closed guard `ci-postgres-fail-closed.test.sh`). Green on PRs #980 and #982. |
| G3 | 1,000 contended claims, one winner each; service delay and outage | **Met in process; live two-Mac run still owed** | `TestTwoNodesContendThroughTheServiceExactlyOnceUnderDelayAndOutage` (`internal/routerstore/remote_contention_test.go`): two separate RemoteStore clients (own sessions, own retry loops) race for one task through real HTTP to one service, 1,000 rounds, latency injected at twice the measured baseline worst case, and a 30-second service outage in the middle (scaled to 300 rounds and 1.5 s under `-short` in CI); exactly one winner every round; race-clean; runs on SQLite and Postgres. Negative control `TestContentionCounterDetectsSplitBrain`: two separate services let both nodes win. Not proven: two physical machines. |
| G4 | Lossless, idempotent migration | **Met** | `internal/routerstore/migratestore.go` hashes source and destination dumps, diffs and re-imports; the 2026-09-10 cut-over recorded equal hashes `e69c7de3…` and a transactional rollback of a bad first import (`ADR-062-RS20-CUTOVER-EVIDENCE-20260910.md`). |
| G5 | Every request authenticated as a registered session | **Met** | Rejection tests in `internal/routerstore`: host token (`TestHostTokenAuthorizesOnlyItsOwnHost`, `TestHostTokenCannotUseAnotherHostsSession`, revocation), runtime (`TestIdentityWrongRuntimeIsRejectedAndSessionRevoked`), nonce (`TestIdentityStaleNonceIsRejected`, `TestIdentityReplayedNonceIsRejected`), ownership (`TestIdentityLeaseOwnershipIsPerSession`). Positive controls not individually re-verified today. |
| G6 | Cloud Run + Cloud SQL, rollback and revocation rehearsed, TLS pin, least privilege, audit receipt | **Partial** | Live: Cloud Run revision `sirsi-router-00021-4rj`, image digest `sha256:782d9624…`. Least privilege: `pg/` roles checked by `scripts/check-pg-schema.sh`. Revocation: unit-tested. **Open:** Cloud Run traffic rollback not rehearsed; SPKI pin is computed and published (`deploy.sh`, recipe C11) but no client enforces it (no code references it); the release train wrote no per-deploy audit receipt (added in this PR). |
| G7 | Both Macs, Codex and Claude lanes, same ledger | **Partial** | `sirsi router status` today: M1 and M5 both report 83 open, 14816 closed. Claude lanes work on both hosts and Codex consumers dispatch on the M5 today. Codex on the M1 and Claude consumers on the M5 remain unproven (per 2026-09-10 receipt). |
| G8 | Unset `SIRSI_ROUTER_URL` returns to the local file | **Met** | Rehearsed and timed 2026-09-10 on the M5: rollback 0.030 s, identical data dump hash (`ADR-062-RS20-CUTOVER-EVIDENCE-20260910.md`). Not re-run today. |
| G9 | Third machine: mint token, one env var, register | **Open** | Not rehearsed (rs-21). A rehearsal needs a fresh account and a minted token. Agents do not touch tokens, so this is owner-run. |
| G10 | Fleet-wide board in Horus, menubar, `router board` | **Met** | Redesigned Horus dashboard rendered against the live service today (Overview, Lanes, Releases) and shipped in v0.24.69; `sirsi router fleet` agrees. |
| G11 | User guide, developer README, runbook | **Met on merge** | `docs/router-service/{USER_GUIDE,README,RUNBOOK}.md`. Runbook marks each step rehearsed or documented only. Before this change the only runbook was `docs/stacklab/ra/canon/RUNBOOK.md`, a three-line stub. |
| G12 | Commercialization gate entry | **Open** | Entry recorded in `docs/COMMERCIALIZATION_GATE.md`; classification `pilot`, with product, narrative open and design, technical, operational partial. The gate is recorded, not passed. |

## What closes the rest

1. G3: run the two-host, delayed-service variant against a test service.
2. G6: rehearse a Cloud Run rollback in a scheduled window; enforce the SPKI pin in the client.
3. G7: one Codex lane on the M1 and one Claude consumer on the M5, same-minute status from both.
4. G9: owner mints a token and rehearses on a fresh account.
5. G12: close product (a "run your own" install rehearsed by an outsider) and narrative (README and launch copy).


## Ledger rows that read done without evidence

`sirsi router ledger ra` shows `rs-17-rehearsals`, `rs-21-third-machine`, `rs-23-docs` and `rs-25-gate-closure` as done, yet each sits at stage `spec` with no links or evidence attached, and the repo held none of the artifacts they name (no third-machine rehearsal, a stub runbook, no commercialization gate file). A done row with no evidence is the A35 shape: the claim outran the check. This audit is now linked from those rows; rs-21 and rs-17 stay open in substance until a rehearsal is recorded.
