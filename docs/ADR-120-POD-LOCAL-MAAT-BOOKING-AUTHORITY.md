# ADR-120: Pod-Local Ma'at Booking Authority (ADR-062 Exception for One Resource)

## Status
**Proposed.** Owner decision 2026-10-22 (verbatim, in Mercury's session: "go with
option 2") authorizes this design to proceed; this document is the ADR text
promised back to the owner for review, not yet his sign-off on the text itself.
Author/design owner: Ra. Related: ADR-062 (router as a service — single
canonical ledger), ADR-040 (do no harm to the running host / load-bearing
recognition), ADR-031-A/B (resource broker), A32/A33 (owner reporting,
universal thread census).

## Context

ADR-062 made the router — and the Ma'at reservation ledger that rides on it —
a single cloud-hosted authority so that every Mac books the same resources
through one atomic store. That holds everywhere the pod has internet.

It does not hold for the specific case Mercury's guarded runs need: a pod (the
Studio + M5, and sometimes M1) with **no internet**, where a "Run" button or a
measured window still has to book `m5`/`studio`/`m1` exclusively against each
other without two local ledgers silently drifting and reconciling after the
fact — the exact failure mode ADR-062 was written to prevent, just scoped to
LAN/Tailscale-only reachability instead of cloud-unreachability on a single
host.

The owner was told, and approved, that any such authority must give up none
of ADR-062's guarantees while the cloud is reachable, and must never let a
disconnected node manufacture authority it doesn't have.

## Decision

Amend ADR-062 for exactly one resource class — Ma'at pod reservations
(`m5`, `studio`, `m1` build/run grants) — with a **pod-local booking
authority** that activates only when the canonical cloud ledger is
unreachable, and that is itself the sole writer for the pod while active.

### Properties (binding, per the owner's requirements)

1. **One authority per pod, never two ledgers that reconcile after the fact.**
   The pod authority is a single process (or singly-elected leader) reachable
   by every Mac in the pod over LAN/Tailscale. Both Macs book through it;
   there is no per-host local fallback ledger that later merges.
2. **Finite, fenced delegation from the canonical ledger.** The pod authority
   holds a delegation lease (resource id, pod id, expiry) minted by the
   canonical Ma'at/router service the last time it was reachable. The
   delegation names exactly the resources it may grant (`m5`, `studio`, `m1`)
   and expires; it does not renew itself without the canonical service.
3. **Fail closed.** If neither the canonical ledger nor the pod authority is
   reachable, no reservation is granted — this is a refusal, not a fallback
   to an uncoordinated local grant.
4. **No stale-lease authority manufacture.** A disconnected node may *queue*
   an intent (e.g. "I want `m5` for 90 min") but a queued intent is never
   treated as a grant. A grant exists only from the pod authority (while
   delegated) or the canonical ledger (while reachable).
5. **Exactly-once reconciliation.** Every grant made by the pod authority
   while offline is replayed to the canonical ledger when connectivity
   returns, idempotently (grant id dedup), so the canonical history shows it
   once — not duplicated, not dropped.

### Non-goals

- This does not create a second general-purpose router ledger. Router items,
  tasks, and threads stay on the single canonical service (ADR-062,
  ADR-057). Only Ma'at pod *reservations* get the offline exception.
- This does not change `enforce_admins`/owner-override semantics (A28), and
  does not grant the pod authority any write path to anything outside the
  three named resources.
- No bypass of Ma'at's existing supervised-execution gate (the front-door
  `sirsi-maat-run` requirement) — the pod authority arbitrates *which grant
  exists*, not *whether a command may run without supervision*.

## Design sketch

```
Canonical Ma'at/router (cloud)
   │  delegation lease: {pod: "m5-studio", resources: [m5,studio,m1], exp}
   ▼
Pod authority (one process, LAN-reachable from every pod Mac)
   │  grants/releases m5|studio|m1 to callers on the pod while delegated
   │  queues intents when it itself can't reach a grantable state
   ▼
maat-run-guard / run conductor (Mercury, Apollo) on each Mac
   -- calls pod authority first when canonical is unreachable,
      canonical ledger first when it is reachable (no dual-check race)
```

Open implementation questions for the first build pass (not blocking this
ADR's scope, but tracked so they aren't silently decided by code):

- **Leader/singleton placement**: which Mac in the pod hosts the authority
  process, and what happens if that Mac is the one that goes offline from the
  *other* pod member (not just from the cloud)? Default answer: the
  authority runs on whichever Mac the owner names in the delegation record;
  losing that Mac fails the pod closed until re-delegated — this is the
  "never manufacture authority from a stale cached lease" rule applied to
  the authority's own host, not just to callers.
- **Delegation renewal cadence**: how far ahead of the lease's expiry does
  the pod authority need the canonical ledger reachable to renew, so a short
  cloud blip doesn't fail a run mid-window.
- **Reconciliation idempotency key**: grant id format that survives a pod
  authority restart without creating a duplicate on replay.

## Verification (Mercury's pre-registered acceptance test, 2026-10-22)

With the M1 offline and the pod's internet cut by the owner (no radio change
by any agent): a Run started from the Studio's page and from the M5's page
each reaches done at ~42 GB/s with a guard verdict and a **new** reservation
granted by the pod authority. After internet returns, the canonical ledger
shows those reservations reconciled **exactly once**. Negative control: with
the pod authority also down, the run is refused (fail closed).

## Owners

- **Ra** — this ADR, the pod authority design and its delegation-lease fence
  path.
- **Sirsi Software Admin** — Ma'at service and deployment custody
  (`maat-local-first-admission-20261022`).
- **Mercury** — `maat-run-guard`/run conductor integration (use the pod
  authority when cloud is unreachable) and the acceptance test above.
- **Apollo** — consumer of the same gate for its own pod runs.

## First deliverable date

2026-10-23: delegation-lease schema + pod-authority interface sketch (no
production wiring yet) for SSA/Mercury review, so the acceptance test's
negative control can be reasoned about before any code lands.
