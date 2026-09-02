# ADR-062: Router as a Service — Fleet Concurrency for Anubis and Ra

## Status
**Accepted for step 1 (merged 2026-09-02: #682, #683, #684); revision 4** — September 2, 2026. Author: claude-home. Owner
decision recorded 2026-09-02 (verbatim: "put it in GCP", "build a solution
only once", "skip github actions"). Reviewer: Sirsi Software Admin
(`sirsi-software-admin`), verdict CONDITIONAL on revision 1, **ACCEPT for Migration step 1 design
authority** on revision 2 (2026-09-02T20:24Z); revision 3 folds in the
remaining evidence conditions for steps 2–4. **This ADR authorizes design and the
behavior-preserving refactor (Migration step 1) only.** Steps 2–4 each require
their own bind carrying the evidence named in §Verification.
Related: ADR-024 (one inbox), ADR-042 (self-hosted CI), ADR-051 (Anubis/Ra
split), ADR-054 (one Horus), ADR-057 (thread store).

## Context

The router ledger is a single SQLite file, `~/.sirsi/router.db`. Most
`sirsi router` verbs reach it through `dispatch.Facade` →
`*routerstore.Store` (`internal/dispatch/facade.go:59`), but **not all**.
Reviewer inventory (verified against `origin/main` at 92ad9808) of production
code that opens the store or resolves its path without the Facade:

| Call site | What it does | Classification |
|---|---|---|
| `cmd/sirsi/routerbreakercmd.go:33-37` | opens store, **mutates** breakers | router state — must go through the resolver |
| `cmd/sirsi/adrcmd.go:280` | opens store, **writes** ADR records | router state — must go through the resolver |
| `internal/router/threads.go:442-451` | opens store for thread authority (ADR-057) | router state — must go through the resolver |
| `cmd/sirsi/schemacheckcmd.go:37` | resolves path, reads schema | local-only diagnostic; read-only; may not mutate |
| `cmd/sirsi/selfupdate.go:28` | resolves path for backup | local-only; may not mutate |

`internal/notify`, `internal/vault` and `internal/seshat` open their own
SQLite files; they are not router state and are out of scope. The path comes
from `SIRSI_ROUTER_DB` or the home directory. There is no network transport:
no listener, no client that accepts a remote address. `internal/routerstore`
already has lease and write-retry layers, so many agents on **one** Mac
contend correctly through kernel file locks.

The owner now runs two Macs (M5 primary, M1 `sirsimasterdev`) and will add
machines and users. Agents on every host must work **the same ledger at the
same time** — the owner's word is *concurrency*. Codex lanes participate on
equal terms with Claude lanes; nothing that Codex needs may move.

Options that do not work were tested against the store as written:

| Option | Why it fails |
|---|---|
| rsync `router.db` between hosts | two writers, two files; lost updates, no atomic claim |
| SQLite on SMB/NFS | file locks unreliable across the network; corruption risk |
| git as the ledger | no locking; a double claim is a merge conflict, not a winner |
| Firestore | not self-hostable; Ra customers run their own hardware (ADR-046) |
| Turso / libSQL | works, but bets an enterprise SKU on a small vendor |

The canon hierarchy (owner, 2026-08-01) already describes the target: **Horus
is one per node; Ra aggregates every Horus into one fabric.** The ledger is the
thing Ra aggregates. Building the transport is building Ra's spine.

## Decision

**The router becomes a service.** Nodes never open the database; they call an
authenticated HTTPS API. One codebase serves both products:

| Product | Store backend | Transport | Hosting |
|---|---|---|---|
| **Anubis** | SQLite, in-process (today's file) | none — same binary, same Mac | the user's Mac |
| **Ra** | Postgres | `sirsi router serve` over HTTPS | Cloud Run + Cloud SQL, GCP `sirsi-nexus-live` |

Anubis users see no server and no new dependency. The owner's own fabric runs
the Ra configuration from day one, because the owner is a fleet.

### 1. Store interface and the single resolver

`internal/routerstore` gains a `Store` **interface** covering the union of
methods called by `dispatch.Facade`, `routerbreakercmd`, `adrcmd` and
`internal/router/threads`. The existing struct is renamed `SQLiteStore` and
satisfies it unchanged. A `PostgresStore` implements the same interface with
`SELECT … FOR UPDATE SKIP LOCKED` for claims, replacing the file-lock retry
loop.

**One resolver.** `routerstore.Open` and `routerstore.DefaultStorePath` become
unexported. The only public constructor is `routerstore.Resolve()`, which
applies the order in §2 and returns a `Store`. Every production call site in
the inventory above is moved onto `Resolve()` in Migration step 1. A
`go vet` analyzer (or a `grep`-based Ma'at gate rule, whichever is smaller)
fails the build on any new direct open outside `internal/routerstore`.
Diagnostics classified local-only (`schemacheck`, `selfupdate`) call
`routerstore.LocalPath()` which is read-only by construction and refuses to
return a path when `SIRSI_ROUTER_URL` is set — a node on the service has no
local ledger to check or back up. This closes the split-brain path the
reviewer identified: nothing can write local state while the node is
pointed at the service.

### 2. Transport

`sirsi router serve --listen :8080 --store postgres://…` exposes the `Store`
interface over HTTPS, one route per method, JSON bodies. `Resolve()` picks:

1. `SIRSI_ROUTER_URL` set → HTTP client implementing `Store`.
2. `SIRSI_ROUTER_DB` set → SQLite path (unchanged, tests and sandboxes).
3. neither → `~/.sirsi/router.db` (unchanged, Anubis default).

Every remote call carries a bounded `context.Context` (default 5 s, lease
operations 2 s), exponential backoff with jitter capped at 30 s, and a
per-node in-flight limit of 4 so a slow service backpressures the wake loop
instead of piling up claims. Lease TTLs are set to no less than 10× the p99
request latency measured in §Verification, so a lease cannot expire inside a
single retry window. Horus, wake lanes, Codex lanes and the menubar keep
their existing call paths; only the constructor beneath them changes.

### 3. Identity

Items and threads gain first-class `host`, `user`, `agent` and `session`
columns. **cwd is metadata, never identity.** It is caller-controlled and two
agents can share one directory (a known failure class on this workstation),
so it cannot authenticate anything.

Identity is a registered **agent session**, bound at `sirsi thread register`
(ADR-057) and carried on every request:

| Claim | Proven by | Checked on |
|---|---|---|
| host | per-host bearer token from Secret Manager, rotated, revocable | every request |
| runtime | executable identity: the `sirsi` binary's code-signature hash and version, **bound into the session at register time** and matched against the release manifest server-side; a caller-provided hash is never trusted on its own — the service validates token, then nonce, then checks the claimed runtime equals the session's bound runtime; any mismatch invalidates the session | every request, in that order |
| agent + session | session id minted by the service at register time, bound to (host, runtime, agent id); stored server-side | every request |
| freshness | expiring nonce (60 s) signed with the session key; replay rejected | every mutating request |
| ownership | lease and write verbs succeed only when the session owns the item or lease | every lease/write |

A Codex lane and a Claude lane in the same cwd are therefore two sessions
with two keys; neither can act as the other. Token revocation and session
expiry are service-side and take effect on the next request. The existing
registry (`.agents/idea-router/agents.json`) remains the source of *which*
agent ids exist; the service is the source of *which sessions are live*.

### 4. Deployment — direct, never GitHub Actions

Owner rule (2026-09-02): "github actions is costly for no helpful reasons."
Deploy is `gcloud run deploy sirsi-router --source . --project sirsi-nexus-live`
run from the owner's Mac as `sirsimaster@gmail.com`, with the Cloud SQL
instance attached. No `.github/workflows/*deploy*.yml` is added; the existing
docs-site workflow is untouched.

**This section is a proposal, not deployment authority.** Deployment authority
is a separate bind (Migration step 4) that must carry:

- **Rollback:** the previous Cloud Run revision pinned by digest; `gcloud run
  services update-traffic` to it is rehearsed and timed before cutover.
- **Revocation:** per-host tokens revocable individually in Secret Manager;
  a revoked host's sessions are rejected on the next request; rehearsed.
- **TLS and server identity:** Cloud Run managed TLS; clients pin the
  service's expected SPKI hash shipped in the release manifest, so a
  redirected `SIRSI_ROUTER_URL` cannot impersonate the service.
- **Least privilege:** the service runs as its own service account with a
  Postgres role limited to the router schema; migrations run under a
  separate role; no owner credentials in the container.
- **Audit receipt:** every deploy writes a router item (`type: decision`,
  from the deploying agent, to `claude-home`) carrying image digest, git
  SHA, revision name and the rollback target digest. The receipt is the
  audit trail; a deploy without one is treated as unrecorded and rolled back.

### 5. What does not move

- **SNE** stays Apple-silicon only; the cloud holds the ledger, never
  inference (ADR-046, ADR-051).
- **Wake lanes, horus supervise, gemma-broker** stay on a Mac. A cloud ledger
  removes the need for the M5 to be *reachable*, not the need for a Mac to be
  *awake*.
- **`~/.codex`, `~/.sirsi`** are per-host and never synced. Codex lanes reach
  the service through the same `sirsi` binary and `SIRSI_ROUTER_URL`.
- **Repos and Thoth** sync through git as today. Claude memory
  (`~/.claude/projects/*/memory`) is out of scope here.

## Consequences

**Positive**
- Adding a machine = minting a token and setting one env var.
- Adding a user = the same; identity is in the ledger, not the filesystem.
- Ra ships the same binary a customer can run against their own Postgres.
- Cloud Run scales to zero; an idle fabric costs cents.

**Negative / accepted**
- One more managed service (Cloud SQL) to own. Mitigation: smallest tier,
  automated backups, `SIRSI_ROUTER_DB` fallback keeps a node useful offline
  for local-only work (ADR-046 T0 floor).
- Latency per verb rises from ~1 ms (file) to an HTTPS round trip. The
  reviewer confirmed `internal/router/wake.go` is ticker and backoff based
  with no 1 ms assumption, but the figure is **unmeasured until step 3**:
  p50/p95/p99 from each Mac to Cloud Run are recorded in that bind, and
  lease TTLs, request contexts and backoff (§2) are set from those numbers,
  not guessed. The progress gate in PR #639 bounds spawn rate independently.
- Two store backends to test. Mitigation: the existing
  `enforcement_adversarial_test.go` and `dispatch_contract_test.go` run
  against both via the interface — one suite, two drivers.

## Migration

1. **Interface + resolver** — `Store` interface, `Resolve()`, all five
   inventoried call sites moved, direct-open gate installed, behavior
   preserving, all existing tests green. **Authorized by this ADR.** Bind #1.
2. **`PostgresStore`** + the existing contract and adversarial suites run
   against both drivers in a local Postgres container. Bind #2.
3. **`serve`, HTTP client store, migration tool, measured evidence.**
   `sirsi router migrate --to $SIRSI_ROUTER_URL` is proven by, in order:
   (a) quiesce — fabric quarantine marker set, no live sessions, local DB
   opened read-only; (b) snapshot-consistent export of schema, indexes and
   every row of every table including leases, tombstones and ordering
   metadata, via a deterministic canonical dump whose hash is recorded;
   (c) dry-run import that reports what it would write and changes nothing;
   (d) real import; (e) canonical dump of the Postgres side, full diff
   against (b), must be empty; (f) second import, must be a no-op with the
   same hash — idempotence. Writes stay frozen for the **entire** interval
   (a) through (f); the quarantine marker is released only by the migration
   tool's own exit path, success or failure, so a failed migration cannot
   leave the fabric falsely quarantined (the tool records which path
   released it). **Clock and lease model:** the service's clock is the only
   authority for lease expiry; clients send no timestamps, and every lease
   response carries the server-issued expiry, which the client treats as
   opaque. The bind also carries the latency and
   concurrency measurements in §Verification. Bind #3.
4. **Provision and cut over** in `sirsi-nexus-live`, under the deployment
   authority requirements in §4. `router.db` on the M5 stays read-only until
   rollback to it has been rehearsed and verified, then is retained 30 days
   and pruned under the retention policy. Bind #4.

Steps 1–3 are revertible by unsetting `SIRSI_ROUTER_URL`. Step 4 is
revertible only by the rehearsed rollback, which is why it is gated on the
rehearsal.

## Verification (owner standard: verify the artifact, not the command)

Bind #1
- `grep -rn 'routerstore\.\(Open\|DefaultStorePath\)(' cmd internal` returns
  only `internal/routerstore/`; the gate fails a deliberately added direct
  open (negative control).
- Existing suites green with `SQLiteStore` behind the interface.

Bind #2
- `dispatch_contract_test.go` and `enforcement_adversarial_test.go` pass
  against both drivers from one suite.

Bind #3
- Two hosts claim the same item within one second, 1,000 trials; exactly
  one wins every trial. Negative control: the same trials against two
  separate SQLite files show both "winning", proving the test detects the
  defect.
- No duplicate claims and no overlapping wake consumers across 1,000 wake
  cycles from two Macs with the service artificially delayed to 2× measured
  p99 and with one 30 s Cloud SQL outage injected; leases never expire
  mid-retry.
- Latency table, per Mac: p50, p95, p99 for `pull`, `send`, lease claim and
  `respond`, over 1,000 calls each. Lease TTL ≥ 10× p99 recorded beside it.
- Migration artifact: canonical dump hashes for (b), (e) and (f) equal; the
  (e) diff is empty; the dry-run log shows the row counts it would write.
- A session presenting the wrong runtime hash, an expired nonce, or another
  session's lease is rejected — one test per claim, each with a passing
  positive control.

Bind #4
- Deploy receipt item present in the ledger with the digest that
  `gcloud run services describe` reports, and the rollback target digest.
- Rollback rehearsal: traffic moved to the previous revision and back, timed.
- Revocation rehearsal: one host token revoked; its next request fails; the
  other host is unaffected.
- `sirsi router status` on M1 and M5 report identical open counts and the
  same item ids after each writes one item.


## Amendments (revision 4, 2026-09-02, from implementation and SSA review)

Each amendment records what the code does where it differs from the text
above, with the reviewer's disposition.

1. **Constructor naming (SSA Bind #1, accepted).** `Open`/`DefaultStorePath`
   did not become unexported; they became `OpenPath` (tests and `Resolve()`
   only) and `LocalPath()` (read-only diagnostics, refuses under
   `SIRSI_ROUTER_URL`). Seven test files across five packages need a
   path-open, so the boundary is enforced by the direct-open gate
   (`scripts/check-router-store-open.sh`, in CI and pre-push) rather than by
   export scope. The gate's allowlist is exactly `cmd/sirsi/routerservecmd.go`,
   the service entry point.
2. **Identity binding lives in a side table (SSA rs-10, accepted).** §3 said
   items and tasks gain `host`/`user`/`session` columns. `items` mirrors
   `internal/work.Item` field-for-field and round-trips through markdown
   (`TestFieldFidelityWithWorkItem` forbids invented columns), and identity is
   service-side truth that must never round-trip through a file. The binding
   is `lease_sessions(kind, key, session)`; `threads` does gain
   `host`, `user_id`, `session`, `runtime_hash`. Sessions live in `sessions`.
3. **Enrollment (SSA rs-10 condition; rs-11).** `MintSession` is reachable
   with a bearer token only, by design — it is how a node obtains a session.
   Under a **per-host token** (`sirsi router token mint <host>`) it may mint
   only for that host (`ErrHostMismatch`); the shared bootstrap token is the
   single unconstrained path and is operator-held. Agent id stays
   caller-declared: ownership is by session, an agent label is a workstream
   claim, not a permission. Runtime attestation beyond the self-reported
   executable hash (release-manifest match) is Phase D, rs-16, as §3 states.
4. **Store failures are 503, never 401 (rs-13 finding).** During a 30 s
   database outage every call had read "invalid bearer token" because the
   token/session lookups failed on the store and the handler treated that as
   an auth verdict; clients re-minted sessions. A store failure in the auth
   chain is now `503` + `ErrServiceUnavailable`; the client keeps its
   session and retries (`TestOutageIs503NotUnauthorized`).
5. **Lease-expiry reclaim is not a duplicate claim.** Under an outage a holder
   whose completion fails keeps its lease until TTL; the item then reopens and
   another host may claim it. The evidence report distinguishes this designed
   recovery from a concurrent duplicate (two hosts both completing). Measured
   2026-09-02: 3,000 items, two hosts, 0 concurrent duplicates
   (`docs/evidence/ADR-062-RS13-TWO-MAC-EVIDENCE-20260902.md`).
6. **One implementation, two dialects.** There is no separate `PostgresStore`
   type; `SQLiteStore` carries a dialect (`internal/routerstore/dialect.go`)
   and `OpenPostgres` returns it. "PostgresStore implements the same
   interface" is satisfied by one implementation behind one rewrite seam.
7. **Migration also removes trigger-minted wake events and scrubs NUL on
   request.** `migrate-store` deletes destination wake events the source
   never had (minted by the destination's own insert triggers for pre-v10
   rows) and, with `--scrub-nul`, strips 0x00 from text cells on both sides;
   a source with NUL is otherwise refused with the cells listed.
8. **Evidence-run knob.** `SIRSI_ROUTER_SERVE_TEST_DELAY` sleeps before every
   call on the service; off unless set, logged at startup when on.
