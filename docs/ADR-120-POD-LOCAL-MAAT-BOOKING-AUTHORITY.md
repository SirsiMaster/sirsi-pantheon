# ADR-120: Pod-Local Ma'at Booking Authority (ADR-062 Exception for One Resource)

## Status
**Proposed.** Owner decision 2026-10-22, given verbatim in Mercury's session ("go with
option 2"), authorizes this design to go ahead. This document is the ADR text promised
back to the owner for review; he has not yet signed off on the text itself.
Design owner: Ra.

### Implementation reconciliation (2026-10-29)

This proposed design is not an activation or acceptance receipt. The current
implementation uses `maat authority-serve` and already-approved mutual TLS
identities, not a pod shared-secret transport. The corrections below describe
the source contract; they do not ratify an authentication change, create keys,
delegate a resource, enable a service, or establish the offline acceptance test.
Service preparation remains the dormant, explicit-consent workflow in
[Ma'at native authority service](user-guides/MAAT_POD_AUTHORITY_SERVICE.md).

The finite grant, durable journal, replay and native routing source exists.
The 2026-10-29 corrective candidate additionally retains a successfully observed
canonical revocation, disappearance or changed grant in that same journal.
It refuses new grants, renewal, live guard reads and historical nonce-liveness
replay after restart with the old cache. Owned release/invalidation and history
remain available. A transport error alone is not a canonical fence observation.
The marker is not a reservation event and does not consume a replay sequence;
older journal readers refuse it rather than silently discarding the fence.
This candidate requires independent review and normal compatible publication;
it is not claimed installed. A failed marker write is an explicit durability
error and blocks the running authority; no crash-durability claim is inferred
from a failed persistence operation.
Automatic 15-minute renewal described as a design goal below is not implemented
by the authority's catch-up loop: `RunRelay` calls `RelayOnce`, not `Delegate`.
An unrenewed grant therefore remains bounded by its original `not_after`.
Runtime/design reconciliation remains Ra's responsibility; SSA owns compatible
service/deployment custody, Mercury the original offline acceptance. A signed
package or dormant recipe is not evidence those obligations are complete.

Related:
- ADR-062: the router as a service, with a single canonical ledger. This ADR amends it.
- ADR-069: the durable ordered outbox.
- ADR-067: credentialed machine identity.
- ADR-040: do no harm to the running host.
- A35: scope the check to the claim.
- A37: a record exists only on origin.

Revision 2 (Ra lead session, 2026-10-22) builds on the first draft at 0c41d330.
That draft settled the requirements; revision 2 settles the mechanism:
- **Authority.** The pod authority holds its resources for as long as its
  delegation is live, not only while the cloud is unreachable. A rule based on
  reachability makes two authorities whenever one Mac can see the cloud and
  another cannot, which is the failure the owner ruled out.
- **Decided, not left open.** Authority placement, renewal, the fence and the
  idempotency key are now decided.

## Context

ADR-062 put the router, and the Ma'at reservation ledger that rides on it
(`internal/maat/schedule`, one state key `maat:reservations`), behind a single
cloud store. Every Ma'at call (`reserve`, `heartbeat`, `release`, `who-is-on`)
is a read-modify-write on that key, so with no internet:
- no guarded run can start;
- the front-door gate `sirsi-maat-run` refuses, as observed on the M1 on
  2026-10-22 when one service call timed out.

The owner's requirement: guarded Mercury and Apollo runs must work offsite with
only Tailscale or a LAN between the Macs. The requirements he was told, and
approved:
- one authority per pod, never two ledgers that reconcile after the fact;
- a finite, fenced delegation;
- reconciliation exactly once when the cloud returns;
- fail closed when neither the cloud nor the pod authority is reachable.

## Decision

Amend ADR-062 for exactly one resource class: **Ma'at reservations on resources
that a delegation names.** Router items, tasks, threads and every other state
key stay on the single canonical service.

### 1. The delegation record (canonical, cloud)

The implemented state key `maat:delegations` holds a JSON document whose
`delegations` array contains one record per pod. The earlier singular
`maat:delegation:<pod>` spelling is historical, not an operator selector:

| Field | Meaning |
|---|---|
| `pod` | Pod id, e.g. `studio-m5` |
| `authority_machine` | Machine-registry id that hosts the authority, e.g. `M5-Studio` |
| `authority_addrs` | The addresses the authority serves on (Tailscale first, then LAN) |
| `resources` | Exact resource ids delegated, e.g. `studio`, `m5`, `rail-a` |
| `epoch` | Monotonic integer; the fence (§3) |
| `issued_at`, `not_after` | `not_after` is at most `issued_at + 12h`; finite (§4) |
| `acked_seq` | Highest pod journal sequence the cloud has reconciled (§5) |
| `pod_secret_hash` | Optional retained schema field; not authentication for the implemented native mTLS transport (§6) |
| `revoked` | Canonical revocation state; does not by itself terminate a partitioned authority or erase the handback fence |

- **Created by** `sirsi maat delegate --pod … --authority … --resources …`.
  Ra or the owner runs it while online.
- **Revoked by** `sirsi maat delegate --revoke`.
- **Overlap check:** a resource can belong to at most one live delegation. A
  create that overlaps another live delegation is refused.

### 2. One authority for delegated resources, always

While a delegation is live, its resources are booked **only** by the pod
authority, whether or not the cloud is reachable.

- **The cloud ledger refuses direct bookings.** `Reserve` on a delegated
  resource returns "delegated to pod `<pod>` (epoch N) at `<addr>`" and grants
  nothing.
- **While online, the authority writes through.** Each grant it makes is copied
  to the canonical ledger immediately, so the cloud record stays current.
- **Pod authority = `sirsi maat authority-serve`** on `authority_machine`.
  - The implemented `PodJournal` uses a retained private event journal and a
    disposable in-memory `StateStore` policy projection with the existing
    `schedule.Ledger`; it does not open a second SQLite reservation database.
  - The listener is explicit. The native client currently requires one exact
    approved HTTPS endpoint from the finite delegation; there is no automatic
    Tailscale/LAN discovery or address failover.
- **One authority, no failover.**
  - If that Mac is off, the pod fails closed.
  - Moving authority means a new delegation, which needs the cloud.
  - The authority never manufactures authority on a second Mac. This is the
    owner's "no stale cached lease" rule, applied to the authority itself.

### 3. The fence

- **The epoch.** Every new delegation, or change of `authority_machine`, bumps
  `epoch`.
- **Stamped grants.** Every grant the pod authority makes carries
  `authority: "pod:<pod>"`, `epoch` and `seq`.
- **The authority checks itself.** When it can reach the cloud (each renewal,
  §4), it compares its epoch with the record. If they differ, or the record is
  revoked, it stops granting at once and returns "fenced: epoch N superseded".
- **The cloud enforces the fence.** It records grants from an old epoch as
  history (they really happened), but never accepts one as current authority
  for a resource.

### 4. Finite delegation and renewal

- **Renewal goal, not current runtime behavior.** The original design calls for
  renewal every 15 min, moving `not_after` to now + 12 h while online. The current
  authority only catches up its journal; it does not renew the delegation.
  Do not describe catch-up, a cached grant, or process liveness as renewal.
- **Expiry.** With no renewal, the delegation expires at most 12 h after the
  cloud was last seen. At `not_after` the authority stops granting, so the pod
  fails closed.
- **Handback.** The cloud treats the resources as its own again only after
  `not_after` + 5 min, a margin for clock skew. So there is never a moment when
  both could grant.
- **Running jobs.** Reservations and heartbeats must fit within the finite
  delegation window. The implemented authority refuses extension beyond that
  bound; the earlier promise of uninterrupted work across renewal timing is
  not qualified. Clients must retain conservative execution/cleanup semantics.

### 5. Reconciliation exactly once

- **The pod journal.** The authority appends every granted lifecycle change,
  including heartbeat renewals needed to prove finite lease continuity, to a local
  journal with a monotonic `seq`. The journal is fsynced before the caller gets
  its answer.
- **Catch-up.** When the cloud is reachable, the authority replays
  `seq > acked_seq` in order.
  - Each entry is an **upsert by reservation ID** into `maat:reservations`, so a
    replay after a crash rewrites the same row; it never adds a second.
  - The canonical transaction adapter commits the replay batch and `acked_seq`
    together, or rolls both back. Repeated acknowledged entries must still bind
    the same reservation/holder/resource/epoch; gaps or substitutions refuse.
- **Queued intent stays queued.** A caller that could not reach the authority
  holds no grant; its request is not in the journal and is never "reconciled
  into" a grant.

### 6. Client resolution, auth and fail closed

- **Cache.** The gate (`sirsi-maat-run`, Mercury's guard, Apollo's runner)
  caches the delegation record locally (0600) whenever the cloud is reachable.
- **Delegated resource:** ask the authority at `authority_addrs`.
  - If it is unreachable, **refuse.** Never fall back to the cloud or to a local
    grant while the cached delegation is unexpired.
  - If the cache says the delegation has expired, use the cloud. If the cloud is
    unreachable too, **refuse.**
- **Any other resource:** the cloud, as today. If unreachable, refuse (unchanged).
- **Implemented native auth.**
  - **Mutual TLS.** Both peers use existing approved certificate/key/CA path
    selectors. The authority requires verified client certificates and an exact
    leaf-certificate SHA-256-to-registered-agent binding. Caller JSON cannot
    choose a principal; no pod shared-secret authentication is implemented.
    TLS policy requires at least TLS 1.3, verified server identity and no redirects.
    These references neither issue nor authorize copying any key.
  - **The cloud token never leaves a Mac.** The ADR-062 host token stays with
    that Mac's relay. The authority never holds or forwards cloud tokens; its
    write-through and catch-up use its own host's relay.
- **The gate's other cloud calls.** Thread register and heartbeat must not
  block a run that has a valid pod grant: they queue through the ADR-069 outbox.
  Ma'at admission is the only call that must succeed.

### 7. Deployment (owner directive 2026-10-22, services run on their own)

- **A signed launchd service.** The pod authority ships as a signed launchd
  service on the pod Macs, installed through the normal signed package path.
- **Never hosted by a lane.** It is never a process that an AI lane (a Claude
  or Codex session, an IDE) starts or keeps alive.
- **Started by the user.** It starts when the user starts it: at login, from
  the app, or by launching the binary.
- **Boot start is opt-in.** Starting at boot or login is something the user
  enables (Login Items or an explicit launchd job), not a silent default.
- **Kept alive by launchd.** Once running, launchd keeps it alive and restarts it.
- **Lanes support it, never host it.** Ra, Mercury and Apollo develop, deploy
  and diagnose it.

### Non-goals

- **No second general-purpose ledger.** Router items, tasks and threads stay
  canonical.
- **No change to the supervised front door.** The pod authority decides which
  grant exists, not whether a command may run unsupervised.
- **No automatic failover or leader election.** A second authority would need a
  consensus protocol. That is out of scope until a measured need exists.

## Data flow

```mermaid
flowchart LR
  C[Cloud Ma'at ledger\nmaat:reservations\nmaat:delegations] -- "finite delegation\n(epoch, not_after ≤ 12h)\nautomatic renewal remains unimplemented" --> A[Pod authority\nmaat authority-serve\nprivate journal + policy projection]
  A -- "bounded catch-up when online\nseq > acked_seq\natomic replay + acknowledgement" --> C
  S[Studio gate / guard] -- "reserve/heartbeat/release\n(approved mutual TLS, exact endpoint)" --> A
  M[M5 gate / guard] -- same --> A
  S -. "authority unreachable\n+ delegation unexpired" .-> X[REFUSE — fail closed]
  M -. same .-> X
  C -. "direct Reserve on a delegated\nresource" .-> R[refused: delegated to pod]
```

## Implementation order

1. **Delegation schema and fence on the cloud side.** The `maat:delegation`
   record, `maat delegate` / `--revoke`, the cloud refusing delegated resources,
   and upsert-by-ID. Unit tests: overlap refusal, epoch fence, upsert idempotency.
2. **`maat authority-serve`.** The existing `schedule.Ledger` policy projection, the
   journal, catch-up, renewal and self-fencing. Tests:
   - crash between upsert and ack: no duplicate row;
   - expiry stops granting;
   - superseded epoch stops granting.
3. **Client resolution in the gate, guard and runner.** Delegation cache,
   refuse-on-unreachable, and outbox-queued thread calls.
4. **Acceptance test** (below), run by Mercury with the owner cutting internet.

Steps 1 and 2 are Ra's. Step 3 is shared: Ra does the gate, Mercury the guard,
Apollo its runner. SSA keeps service and deploy custody.

## Key decisions

| Question | Options | Decision |
|---|---|---|
| When is the pod authority in charge? | (a) only while the cloud is unreachable; (b) whenever its delegation is live | **(b).** (a) splits authority whenever reachability differs between Macs. |
| Failover if the authority's Mac dies? | (a) elect a new leader in the pod; (b) fail closed until a new delegation | **(b).** (a) needs consensus. Fail closed is the owner's rule. |
| Delegation length | short (1 h) / long (12 h) / unbounded | **12 h, renewed every 15 min.** Finite, and covers an evening offsite. |
| Idempotency key | content hash / reservation ID + journal seq | **Reservation ID (upsert) + seq watermark.** IDs already exist and are unique. |
| Authority store | reuse `schedule.Ledger` over a new `StateStore` / a new ledger | **Reuse.** Same grant rules; only the store differs. |
| Native transport as implemented | cloud host tokens / new shared secret / existing approved mTLS identities | **Existing approved mTLS identities**, exact endpoint and fingerprint-to-agent binding. No new credential issuance or cloud-token copying. Original shared-secret design is superseded descriptively, not silently ratified. |

## Verification (Mercury's pre-registered acceptance test, 2026-10-22)

**Setup:** the M1 is offline, and the owner cuts the pod's internet. No agent
changes any radio.

**Pass conditions:**
- A Run started from the Studio's page and from the M5's page each reaches done
  at about 42 GB/s.
- Each has a guard verdict and a **new** reservation granted by the pod
  authority.
- After internet returns, the canonical ledger shows those reservations
  reconciled **exactly once**: one row per reservation ID, and `acked_seq` equal
  to the journal head.

**Negative control:** with the pod authority also down, the run is refused.

**Added by revision 2:** with the delegation revoked from the cloud while the
pod is online, the next pod grant is refused with "fenced".

## Owners

- **Ra:** this ADR, the delegation and fence (steps 1–2), and the gate side of step 3.
- **Sirsi Software Admin:** Ma'at service and deployment custody
  (`maat-local-first-admission-20261022`).
- **Mercury:** maat-run-guard and the run conductor (step 3), and the acceptance test.
- **Apollo:** the same client resolution for its pod runs.

## Dates

- **2026-10-22:** this revision to the owner.
- **2026-10-23:** steps 1–2 schema and interfaces with unit tests, on a branch.
- **2026-10-24:** first working slice: delegation, authority and fence, gate
  client on one pod, tests green. Ready for Mercury's acceptance run when the
  owner can cut internet.
