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
    - 20a.1 **Discovery — DONE 2026-09-10, answer NO.** A `codex exec --sandbox workspace-write` process
      cannot connect to a unix socket either (`curl: (7)`), with `network_access=true` it can.
      Evidence: `docs/evidence/ADR-062-RS22A-CODEX-SANDBOX-SOCKET-DISCOVERY-20260910.md`. The
      socket relay is therefore superseded by 20a.1b; sub-steps 20a.2–20a.4 below are the spool
      shape, replacing the socket shape they had before this amendment.
    - 20a.1b **Design: filesystem spool — location proven.** Second discovery
      (`docs/evidence/ADR-062-RS22-SPOOL-LOCATION-DISCOVERY-20260910.md`): a repo-rooted
      workspace-write lane cannot write `~/.sirsi/relay` (EPERM) unless the directory is declared
      with `-c sandbox_workspace_write.writable_roots=["$HOME/.sirsi/relay"]`; a HOME-rooted lane
      can by default. So the spool is one fixed per-host directory `~/.sirsi/relay/` and every
      Codex lane's consumer command declares it as a writable root (a file-scope grant, never
      network). Layout: `~/.sirsi/relay/<agent>/req/` and `…/<agent>/res/`, each lane directory
      0700 to the lane's uid (same uid as the relay: file modes separate lanes from accidents, not
      from each other). Protocol: the client writes `req/<id>.json.tmp` then `rename(2)`s it to
      `req/<id>.json` (atomic publication; the relay never sees a partial file); one request =
      `{method, headers:{session,nonce,runtime,signature}, body}` forwarded byte-for-byte with the
      relay adding only `Authorization`; the relay writes `res/<id>.json.tmp` → rename; the client
      waits on the response with kqueue/poll and a bounded timeout (default 30 s, per-call
      override), then deletes both files. **Correlation and uncertain writes:** the request id is
      `<unixms>-<per-process sequence>-<8 random hex>` scoped to the lane directory, so two lanes,
      two processes, or a relay restart cannot collide; the relay CONSUMES a request atomically BEFORE forwarding it —
      `rename(req/<id>.json → inflight/<id>.json)`; a failed rename means another relay owns it and
      it is not forwarded — then performs the HTTP call, publishes the response, and deletes the
      in-flight file last. A restarted relay never re-forwards anything it finds under `inflight/`:
      each such file gets a response of **outcome-unknown** ("relay restarted after consuming
      <method> <id>") and is removed. So the failure trace "consume → forward → service commits →
      relay crashes → restart" yields exactly one forward and an outcome-unknown to the caller,
      never a duplicate mutation. A response lost after the service committed (relay crash between
      forward and publish, or a client timeout) is reported to the caller as **outcome-unknown**
      naming the method and id; the client NEVER retries a mutating method (`Send`, `Claim`,
      `Complete`, `Respond`, task verbs) automatically — the caller re-queries (`Get`, `Inbox`,
      `ledger`) and decides. Read-only methods may be retried freely. A durable service-side
      idempotency key for mutations is not part of this step; if the outcome-unknown rate is ever
      non-zero in practice, it becomes a step of its own with its own evidence. Bounds: request bodies ≤ 4 MiB decoded (the service limit; the file
      envelope allows base64 overhead), response bodies ≤ 64 MiB decoded (what `RemoteStore`
      accepts over HTTPS — a full-ledger `ListAll` exceeds 4 MiB), oversize is an error never a
      truncation; at most 64 in-flight requests per lane enforced with exclusive slot files; stale
      files older than 10 min are swept by the relay with a log line. The relay refuses `MintHostToken`, `RevokeHostToken`,
      `ListHostTokens` by name; the spool carries no token; the relay log carries agent + method +
      id only. Least-privilege claim, exactly: the host token is held by one process instead of
      every lane's environment; same-uid processes are not isolated from each other by file modes.
    - 20a.2 `sirsi router relay serve --spool ~/.sirsi/relay`: the forwarder above. Evidence: unit
      tests (forward, refusal list, timeout, secret-free log, consume-before-forward with a failed
      consume not forwarded, restart with an in-flight file → outcome-unknown and zero forwards,
      response lost after forward → caller sees outcome-unknown) + a spooled `status` receipt.
    - 20a.3 Client: `RemoteStore` accepts `SIRSI_ROUTER_URL=spool://~/.sirsi/relay` and `Resolve()`
      requires no token for it. Evidence: tests + `sirsi router status` through the spool from
      inside a `workspace-write` codex sandbox WITHOUT network_access (the raw output).
    - 20a.4 LaunchAgent `ai.sirsi.router.relay` per host, token only in its 0600 plist; installed
      by `sirsi router relay install`; wake plists stop carrying the token once the relay is up.
      Evidence: `launchctl list`, plist mode, `ps eww` of a wake loop (secrets redacted) showing no
      token.
    - 20a.5 Registry: every Codex lane's consumer command carries
      `-c sandbox_workspace_write.writable_roots=["$HOME/.sirsi/relay"]` and the lane env
      `SIRSI_ROUTER_URL=spool://$HOME/.sirsi/relay`; the SSA lane's `network_access=true` is
      removed. Evidence: the SSA lane claims and closes a router item with network_access absent
      (wake log + item result). G7 then needs only 20a.6.
    Ledger (D3): rows `rs-22a-relay-discovery` (done) … `rs-22f-g7-closure` registered on `ra`
    with this dependency chain, `rs-22f` owner-responsible; `rs-22b`, `rs-22c`, `rs-22e` subjects
    carry the spool shape (updated 2026-09-10T03:25Z); `rs-20-cutover-bind4` narrowed to its proven
    subset and `rs-20b-bind4-full` holds the unfinished Bind #4 obligation, blocked by `rs-22f`.
    - 20a.6 **G7 closure (owner gate).** The original condition is Codex AND Claude on BOTH hosts,
      and only an M1 Codex lane claiming and closing satisfies it. The alternate path is an owner
      decision that AMENDS G7's condition in this document (row G7 rewritten to name the amended
      scope and the decision's date/item), with the M1 Codex proof recorded in the evidence file
      as EXCLUDED — never as passed. G7 stays PARTIAL until one of those two exists.
20b. **The Rule of Ra — registration gate (owner 2026-09-10: "every thread launched needs to
    register with the router, new old or indifferent … everyone must register with you to receive
    an audience").** Enforced in the service, not in prose:
    - 20b.1 Service: `Send`, `Claim`, `Complete`/`Close`, `Respond` and task verbs are refused
      unless the CALLING SESSION is bound to its OWN active registered thread: the session's
      thread id must name a `threads` row whose agent and host equal the session's agent and host
      and whose heartbeat is inside the staleness window. One registered thread never covers
      another session under the same agent id. Bootstrap stays reachable: `MintSession`,
      `RegisterThread`, `Heartbeat` and the read-only verbs (`status`, `show`, `ledger`, `list`)
      are exempt; owner-surface `dismiss` is exempt. The refusal names
      `sirsi thread register --agent <id>` and the session's thread id. Evidence: server tests —
      unregistered session refused; registered allowed; stale heartbeat refused; TWO sessions under
      one agent, one registered and one not: the registered one allowed, the other refused; read
      and bootstrap verbs unaffected — plus one live refusal receipt from the service.
    - 20b.2 Launchers register first: wake loops (already), horus (already), interactive sessions
      (`sirsi thread register` in the session-start hook), GUI-launched codex (the consumer prompt's
      first line). Evidence — registration freshness AT MUTATION TIME, not "active now": for every
      item mutation of the last 24 h (`opened`, `closed`, claim/lease changes), the mutating
      session's thread row was registered and inside its heartbeat window at that timestamp. A
      session that registered, mutated, and then finished legitimately PASSES (its thread is
      idle/closed now, but was live at the mutation). A mutation whose session had no registration
      at that time FAILS. Current live-session coverage (`sirsi thread list`) is reported
      separately and is not the pass criterion.
    - 20b.3 Registry audit verb `sirsi router audience [--since 24h]`: two tables — (a) mutations
      whose session was unregistered at mutation time (the failures), (b) live sessions without an
      active thread right now (the coverage gap); both on the board and in `router doctor`.
      Evidence: audit output with zero rows in (a) over 24 h, including at least one finished
      session that passes and one synthetic unregistered mutation that appears in (a) (test).
    - 20b.4 Canon: PANTHEON_RULES gains the Rule of Ra (A-number assigned there); ADR-062
      amendment names the service check. Evidence: the merged rule text + ADR revision line.
    Ledger (D3): rows `rs-22g-rule-of-ra-service`, `rs-22h-rule-of-ra-launchers`,
    `rs-22i-rule-of-ra-audit`, `rs-22j-rule-of-ra-canon` are registered on `ra` (pending,
    review-dependent, chained after `rs-22e`) as of 2026-09-10T03:25Z; no implementation is
    claimed before this amendment merges.
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
