# /goal — Router as a Service (ADR-062), end to end

**Workstream:** `ra` (router agent `ra`, thread `thr-ede440dcf037b029`), reviewer Sirsi
Software Admin (`sirsi-software-admin`). **Owner decisions (2026-09-02):** one ledger shared
by every machine and user — *concurrency*; built once in the Ra shape; hosted in GCP
`sirsi-nexus-live`; deployed directly with `gcloud`, never GitHub Actions; the owner (as Ra) is
the decision authority, GitHub is mechanism. **Governing ADR:** ADR-062 (merged `4b38d111`).

## The goal, in one sentence

An agent on any registered machine, Claude or Codex, claims, works and closes items on the same
ledger at the same time as agents on every other machine, with exactly-once claims proven under
injected failure, and adding a machine or a user is one token and one setting — while every Mac
keeps working alone on its local ledger when the service is unreachable.

## Done means all of these are true, each with evidence

| # | Condition | Evidence |
|---|---|---|
| G1 | No production code opens the router store except through `routerstore.Resolve()` | grep gate in Ma'at pre-push + CI; negative control proves the gate goes red |
| G2 | Same test suites pass on SQLite and Postgres | one suite, two drivers, CI green on both |
| G3 | Two hosts contend for one item 1,000 times; exactly one wins every time, including with the service delayed to 2× p99 and one 30 s database outage | test log in the bind; negative control against two separate SQLite files shows both "winning" |
| G4 | Migration is provably lossless and idempotent | canonical dump hashes (before, after, re-import) equal; full diff empty; dry-run log |
| G5 | Every request is authenticated as a registered session: host token, bound runtime, session id, signed nonce; ownership enforced on every lease and write | one rejection test per claim, each with a passing positive control |
| G6 | Service runs on Cloud Run + Cloud SQL in `sirsi-nexus-live`; deploy has a rollback rehearsal, a revocation rehearsal, TLS pinning, least-privilege roles, and an audit receipt item in the ledger | receipt item id; `gcloud run services describe` digest matches it |
| G7 | M5 and M1 both work the shared ledger; `sirsi router status` agrees on both; Codex lanes and Claude lanes on both hosts claim and close | status output from both hosts in the same minute; **2026-09-10: PARTIAL** — Claude both hosts + one Codex lane (M5, network exception); closes via step 20a |
| G8 | Unset `SIRSI_ROUTER_URL` on a node and it is back on its local file with no data loss | rehearsed, timed, recorded |
| G9 | Adding a third machine is: mint token, set one env var, `sirsi thread register` | rehearsed on a fresh user account |
| G10 | Owner-facing: Horus per node shows fleet-wide board; menubar and `sirsi router board` read the service | screenshot + board output |
| G11 | Docs: user guide + developer README (A8), CHANGELOG (A7), ADR-INDEX, runbook for token mint/revoke/rotate and rollback | files present in the merge |
| G12 | Commercialization gate for **Ra** platform-foundation work passed: product, design, technical, operational, narrative closure recorded | `docs/COMMERCIALIZATION_GATE.md` entry |

## Steps

Each step ends in a PR bound by SSA through the `sirsi-bind` App (ADR-041) with the evidence
named. A step is not done at green CI; it is done when its evidence row above is filled.

### Phase A — Interface and resolver (authorized now by ADR-062)

1. **Extract the `Store` interface** in `internal/routerstore` from the union of methods used by
   `dispatch.Facade`, `routerbreakercmd`, `adrcmd`, `internal/router/threads.go`. Rename the
   struct `SQLiteStore`. No behavior change; all tests green.
2. **Add `routerstore.Resolve()`** implementing the order URL → `SIRSI_ROUTER_DB` → home file.
   Unexport `Open` and `DefaultStorePath`.
3. **Move the five inventoried call sites** onto `Resolve()`; `schemacheck` and `selfupdate` onto a
   read-only `LocalPath()` that errors when `SIRSI_ROUTER_URL` is set.
4. **Install the direct-open gate** in `.githooks/pre-push` and `ci.yml` (grep-based, Rule 0). Prove
   it fails on a planted direct open, then remove the plant. → **Bind #1** (G1).

### Phase B — Postgres backend

5. **Schema** for Postgres with `host`, `user`, `agent`, `session` columns on items and threads;
   migrations under a separate role.
6. **`PostgresStore`** implementing `Store`; claims use `SELECT … FOR UPDATE SKIP LOCKED`.
7. **Run the contract and adversarial suites against both drivers** from one test entry point,
   Postgres in a local container on the self-hosted runner. → **Bind #2** (G2).

### Phase C — Transport, identity, migration tool

8. **`sirsi router serve`**: HTTPS, one route per `Store` method, JSON bodies, bounded contexts,
   in-flight cap, server-issued lease expiry (service clock is the only lease authority).
9. **HTTP client `Store`** with jittered backoff and the in-flight limit; `Resolve()` returns it
   when the URL is set.
10. **Session registration**: `sirsi thread register` obtains a service-minted session bound to
    host token + runtime hash + agent id; every request carries session id and a signed 60 s
    nonce; validation order token → nonce → bound runtime; ownership on every lease/write (G5).
11. **Token lifecycle**: mint, rotate, revoke per host; revocation effective on next request.
12. **`sirsi router migrate --to <url>`**: quiesce (quarantine marker), snapshot-consistent
    canonical dump with hash, dry-run, import, full diff, idempotent re-import; the marker is
    released only by the tool's own exit path, success or failure.
13. **Concurrency and latency evidence** on two Macs against a local `serve`: the 1,000-trial
    claim test with delay and outage injection; p50/p95/p99 per verb per host; lease TTL set to
    ≥ 10× p99 and recorded. → **Bind #3** (G3, G4, G5).

### Phase D — Cloud (needs a separate owner authority card before step 14 runs)

14. **Provision** in `sirsi-nexus-live` as `sirsimaster@gmail.com`: Cloud SQL Postgres smallest
    tier with backups; a service account with a router-schema-only role; Secret Manager entries
    for per-host tokens; Cloud Run service with managed TLS.
15. **First deploy** with `gcloud run deploy --source .`; pin SPKI hash into the release manifest;
    write the audit receipt item (digest, git SHA, revision, rollback target).
16. **Rehearse rollback** (traffic to previous revision and back, timed) and **revocation** (one host
    token revoked, that host fails, the other unaffected).
17. **Migrate** the M5 ledger with step 12 against the real service; M5 `router.db` set read-only.
18. **Cut over M5**: set `SIRSI_ROUTER_URL` for horus, wake lanes, Codex lanes, menubar; confirm
    each surface claims and closes. **Join M1**: its own token, `sirsi thread register`, same
    checks. → **Bind #4** (G6, G7, G8).

### Phase E — Fleet proof and product closure

19. **Third-machine rehearsal** on a fresh macOS user account: token, env var, register, claim,
    close (G9).
20. **Horus and menubar read the service**; `sirsi router board`/`fleet` show all hosts (G10).
20a. **Codex-lane relay — least privilege (owner 2026-09-10: "power invested in Ra").** Codex sandboxes
    have no DNS, so after the cut-over a Codex lane cannot reach the service; the interim is a
    network exception on the SSA lane only. The relay retires it. Steps, each with evidence:
    - 20a.1 **Discovery**: prove whether a `codex exec --sandbox workspace-write` process can connect
      to a unix-domain socket under `$HOME/.sirsi/` (seatbelt profile). Evidence: the raw codex
      output of a socket round-trip, both with and without `network_access`. If it cannot, the
      fallback design (per-lane loopback port or a filesystem queue) is written as 20a.1b before
      any code.
    - 20a.2 `sirsi router relay serve --socket $HOME/.sirsi/router.sock`: an HTTP listener on a
      unix socket (0600) that forwards `/v1/*` to `SIRSI_ROUTER_URL` with the host token from its
      OWN environment. It refuses the token methods by name (`/v1/call/MintHostToken`,
      `RevokeHostToken`, `ListHostTokens`), forwards everything else byte-for-byte so the
      service's session, signature, runtime-hash and ownership validation is unchanged, and logs
      agent id + method only — never a token, session secret or nonce. Least-privilege claim, stated
      exactly: the token is held by one process instead of every lane's environment; a 0600
      socket does NOT isolate the relay from other processes under the same uid. Evidence: unit
      tests (forwarding, refusal list, secret-free log) + `curl --unix-socket` receipt.
    - 20a.3 Client: `RemoteStore` accepts `SIRSI_ROUTER_URL=unix:///path` (http over a unix
      dialer) and `Resolve()` does not require `SIRSI_ROUTER_TOKEN` for it. Evidence: tests + a
      `sirsi router status` through the socket.
    - 20a.4 LaunchAgent `ai.sirsi.router.relay` per host, token only in its 0600 plist; installed
      by `sirsi router relay install`; wake plists stop carrying the token once the relay is up.
      Evidence: `launchctl list`, plist mode, `ps eww` of a wake loop showing no token.
    - 20a.5 Registry: every Codex lane's env points at the socket; the SSA lane's
      `network_access=true` is removed. Evidence: the SSA lane claims and closes a router item with
      network_access absent (wake log + item result). G7 then needs only 20a.6.
    Ledger (D3): rows `rs-22a-relay-discovery` … `rs-22f-g7-closure` registered on `ra` with this
    dependency chain, `rs-22f` owner-responsible; `rs-20-cutover-bind4` narrowed to its proven subset
    and `rs-20b-bind4-full` holds the unfinished Bind #4 obligation, blocked by `rs-22f`.
    - 20a.6 **G7 closure (owner gate).** The original condition is Codex AND Claude on BOTH hosts,
      and only an M1 Codex lane claiming and closing satisfies it. The alternate path is an owner
      decision that AMENDS G7's condition in this document (row G7 rewritten to name the amended
      scope and the decision's date/item), with the M1 Codex proof recorded in the evidence file
      as EXCLUDED — never as passed. G7 stays PARTIAL until one of those two exists.
21. **Docs**: `docs/user-guides/router-service.md`, `internal/routerstore/README.md`, runbook
    `docs/runbooks/router-service-tokens-and-rollback.md`, CHANGELOG, ADR-INDEX (G11).
22. **Retention**: M5 local `router.db` retained 30 days read-only, then pruned; retention policy
    updated for the service store.
23. **Commercialization gate** entry for Ra platform-foundation; Thoth memory and journal updated;
    continuation written to `docs/continuations/ra-router-service-<date>-<session8>.md` (G12).

## Owner gates (the only two)

- Before step 14: **security/privacy card** — cloud placement of the ledger, token custody, and
  the service account's scope. Everything before it is local and reversible.
- Before step 18: **cut-over card** — the moment the M5 stops writing its local file.

## Not in scope

SNE stays on Apple silicon. Wake lanes, horus supervise and gemma-broker stay on a Mac. `~/.codex`
and `~/.sirsi` stay per host. Claude memory sync is a separate workstream. GitHub Actions is not
used for any deploy.
