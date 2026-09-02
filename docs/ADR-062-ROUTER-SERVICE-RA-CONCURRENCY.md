# ADR-062: Router as a Service — Fleet Concurrency for Anubis and Ra

## Status
**Proposed** — September 2, 2026. Author: claude-home. Owner decision recorded
2026-09-02 (verbatim: "put it in GCP", "build a solution only once", "skip
github actions"). Review: codex-home (Sirsi Software Admin).
Related: ADR-024 (one inbox), ADR-042 (self-hosted CI), ADR-051 (Anubis/Ra
split), ADR-054 (one Horus), ADR-057 (thread store).

## Context

The router ledger is a single SQLite file, `~/.sirsi/router.db`, opened
directly by every `sirsi router` verb through `dispatch.Facade` →
`*routerstore.Store` (`internal/dispatch/facade.go:59`). The path comes from
`SIRSI_ROUTER_DB` or the home directory. There is no network transport: no
listener, no client that accepts a remote address. `internal/routerstore`
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

### 1. Store interface

`internal/routerstore` gains a `Store` **interface** covering exactly the
methods `dispatch.Facade` already calls. The existing struct is renamed
`SQLiteStore` and satisfies it unchanged. A `PostgresStore` implements the
same interface with `SELECT … FOR UPDATE SKIP LOCKED` for claims, replacing
the file-lock retry loop. `Facade` holds the interface. **No verb in
`cmd/sirsi` changes its call site.**

### 2. Transport

`sirsi router serve --listen :8080 --store postgres://…` exposes the Facade
over HTTPS, one route per Facade method, JSON bodies. `sirsi router <verb>`
verbs resolve their store in this order:

1. `SIRSI_ROUTER_URL` set → HTTP client implementing the `Store` interface.
2. `SIRSI_ROUTER_DB` set → SQLite path (unchanged, tests and sandboxes).
3. neither → `~/.sirsi/router.db` (unchanged, Anubis default).

The HTTP client is itself a `Store` implementation, so `Facade` cannot tell
whether it is local or remote. Horus, wake lanes, Codex lanes and the menubar
all keep their existing code paths.

### 3. Identity

Items and threads gain first-class `host`, `user` and `agent` columns. The
`host` label that already exists on process and thread records
(`internal/router/processes.go:47`) becomes authoritative. Each node
authenticates with a bearer token minted per host and stored in GCP Secret
Manager; the service rejects an item whose `host` does not match the token.
Agent ids stay cwd-resolved on each node (PANTHEON_RULES identity rule) —
the token proves the host, the cwd proves the agent.

### 4. Deployment — direct, never GitHub Actions

Owner rule (2026-09-02): "github actions is costly for no helpful reasons."
Deploy is `gcloud run deploy sirsi-router --source . --project sirsi-nexus-live`
run from the owner's Mac as `sirsimaster@gmail.com`, with the Cloud SQL
instance attached. **Every deploy writes a router item** (`type: decision`,
from the deploying agent, to `claude-home`) carrying the image digest and
git SHA — that item is the audit trail. No `.github/workflows/*deploy*.yml`
is added; the existing docs-site workflow is untouched.

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
- Latency per verb rises from ~1 ms (file) to ~30–80 ms (HTTPS). Wake lanes
  poll; the progress gate in PR #639 already bounds spawn rate, so this does
  not change the cost class. Measured before cutover, recorded in the bind.
- Two store backends to test. Mitigation: the existing
  `enforcement_adversarial_test.go` and `dispatch_contract_test.go` run
  against both via the interface — one suite, two drivers.

## Migration

1. Interface extraction — pure refactor, behavior-preserving, all existing
   tests green. Bind #1.
2. `PostgresStore` + contract suite against a local Postgres container. Bind #2.
3. `serve` + HTTP client store; `sirsi router migrate --to $SIRSI_ROUTER_URL`
   copies `router.db` rows into Postgres, idempotent and verified by row
   count and per-item hash. Bind #3.
4. Cloud SQL + Cloud Run provisioned in `sirsi-nexus-live`; first deploy
   recorded as a router item. M5 nodes cut over by setting
   `SIRSI_ROUTER_URL`; M1 joins with its own token. `router.db` on M5 is
   retained read-only for 30 days, then pruned under the retention policy.

Each step is independently revertible: unset `SIRSI_ROUTER_URL` and the node
is back on its local file.

## Verification (owner standard: verify the artifact, not the command)

- Two hosts claim the same item within one second; exactly one wins, the
  other receives the existing "already claimed" error. Negative control: run
  the same test against two separate SQLite files and confirm both "win",
  proving the test detects the defect.
- `sirsi router status` on M1 and M5 report identical open counts and the
  same item ids after each writes one item.
- Deploy audit item present in the ledger with the digest that
  `gcloud run services describe` reports.
