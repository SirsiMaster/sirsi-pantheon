# ADR-052: A2A/Router-Conduit Operating Rules — Pantheon Adoption

## Status

**Accepted** — 2026-08-03. Owner: claude-pantheon (sirsi-pantheon).

**Numbering note:** The owner/claude-nexus system designates the cross-portfolio A2A/conduit
standard as "ADR-051". Pantheon ADR-051 is already occupied by the SNE Supervisor
Configuration split (PR #449, merged 2026-08-03). This ADR, numbered 052, is Pantheon's
formal adoption of those operating rules in its own ADR space. Cross-references from other
repos to "ADR-051 A2A" resolve here.

---

## Context

On 2026-08-03, claude-nexus issued two decisions establishing the A2A/router-conduit layer
as the single undergirding coordination primitive across the Sirsi portfolio:

- `20260803-164603` — Live fleet dashboard at localhost:8734; subscribe if useful
- `20260803-162146` — Owner directive: confirm ADR-051 A2A undergirds all router coordination

These decisions were forwarded to claude-pantheon via the owner dashboard directive item
`20260803-173116-codex-home-claude-pantheon-owner-dashboard-directive-ledger-your-real-in-flight-pantheo`.

The directive identified five operating rules that Pantheon must adopt. Critically, several
in-flight workstreams were producing conflicting patterns: browser polling (rejected), recursive
scheduling (rejected), cwd-derived identity (rejected), and a second dispatch authority (rejected).
At the same time, the Universal Task Ledger (ADR-050) shipped its `sirsi router task add` verb
but no in-flight tasks had been registered — the fleet dashboard had no truthful projection.

There was also a three-way ADR-051 numbering collision (inbox items `20260803-192909`,
`20260803-193557`) arising from concurrent agent threads each claiming 051 for different
purposes. This ADR acknowledges and resolves the Pantheon side of that collision.

---

## Decision

Pantheon formally adopts the following five operating rules, effective immediately:

### Rule 1 — ADR-051 A2A Layer is the single conduit authority

Router work in Pantheon composes the durable item authority defined in ADR-049/050 and the
A2A contract established at the portfolio level. All inter-agent handoffs, task-transition
events, and work routing go through the router primitives (`sirsi router send/close/respond`,
`sirsi router task add/update`). No second dispatch authority is introduced.

The seven A2A properties this layer provides:

| Property | Pantheon implementation |
|----------|------------------------|
| Durable item authority | Router SQLite store (ADR-036/037); `items/*.md` are the item record |
| Transactional task-transition events | `sirsi router task update --status` (ADR-050 migration v3) |
| Edge-triggered marker | Router wait + SSE events; `items/` file monitor for pull-model threads |
| Fenced spawn lease | Thread register + current_item pickup (ADR-022/024/025) |
| Adapter-bound identity | ADR-041 identity-enforced bind; per-agent `responsible-party` in task registry |
| Bounded consumer | `router pull --build-filter` (type gate); `router wait` (event-wake, <250ms) |
| Evidence-bearing disposition | `router respond` (atomic close + route back); `router close --result` |

### Rule 2 — PR #430 durable host anchoring is liveness input, not an alternate dispatcher

PR #430 (thread record becomes session-id lease, merged) proved that the durable-host anchor
is correct liveness evidence for the thread registry. It is not a parallel dispatch path.
`sirsi thread register` consumes that anchor; the router dispatch fabric remains the sole
task-routing authority.

### Rule 3 — Prohibited patterns

The following patterns are prohibited in Pantheon and must be reconciled with ADR-051/052
before any bind or release:

- **Browser polling** — the local dashboard at `http://localhost:8734` is a read-only SSE
  projection; no Pantheon code polls it or uses it as an authority.
- **Recursive scheduling** — no Pantheon module schedules work by calling the scheduler from
  within a scheduled handler. All recurrence is owned by the supervisor (launchd / Ra).
- **cwd-derived identity** — no Pantheon module infers its agent-id from the working directory.
  Identity comes from the thread registry and is established at register time (ADR-041).
- **Second dispatch authority** — all task routing goes through the router primitives listed
  in Rule 1. No module bypasses the router to route directly to another agent or surface.

### Rule 4 — Task ledger is authoritative

Every real in-flight task in Pantheon is registered with `sirsi router task add` and its
status is kept truthful. The task ledger (ADR-050) is the authoritative projection for:
- The live fleet dashboard at `http://localhost:8734`
- The future native Ledger Board in the menubar and Nexus
- CTR per-agent staleness and blocked-count summaries

Minimum registered task fields: `agent`, `task-id`, `--subject`, `--status`, `--responsible-party`.

### Rule 5 — Local dashboard is read-only

`http://localhost:8734` is the optional read-only SSE projection of the router ledger.
It is an observer (ADR-049), not an authority. `sirsi ctr` is not used in refresh loops
because it performs wake sweeps; use `sirsi router ledger --json` for programmatic reads.

---

## Conflicting In-Flight Work (Reconciliation Register)

The following active Pantheon workstreams were identified as requiring reconciliation:

| Item | Conflict | Resolution |
|------|----------|------------|
| PR #450 (Horus per-node conduit, ADR-051 number) | ADR-051 number occupied by PR #449 | PR #450 must renumber its ADR to ADR-053 before bind |
| PR #451 (SNE canon artifacts, ADR-051 number, no CI) | ADR-051 number occupied; owner-gated claims canon with no CI gate | PR #451 must renumber AND pass CI before bind; owner must approve canon claims |
| Any module polling `localhost:8734` | Violates Rule 3 (no browser polling of the dashboard) | Audit and remove any such polling before merge |

---

## Consequences

**Positive:**
- Single source of truth for inter-agent dispatch — router primitives, not parallel authorities.
- Fleet dashboard and Ledger Board now have truthful, registered task data to project.
- The numbering collision is documented and resolution is explicit (050→051→052 chain).

**Negative:**
- PR #450 and #451 authors must renumber their ADRs — cost: one commit each.
- `ctr` may not be used in refresh loops, which constrains some polling patterns.

---

## References

- `docs/ADR-049-ROUTER-OBSERVER-BOUNDARY.md` — transport vs observer split
- `docs/ADR-050-UNIVERSAL-TASK-LEDGER.md` — task registry schema and CLI
- `docs/ADR-051-ANUBIS-RA-SNE-SUPERVISOR-SPLIT.md` — occupies ADR-051 (Pantheon numbering)
- `docs/ADR-041-IDENTITY-ENFORCED-BIND.md` — adapter-bound identity
- `docs/ADR-036/037` — store cutover authority
- claude-nexus decisions `20260803-164603`, `20260803-162146`
- Router item `20260803-173116-codex-home-claude-pantheon-owner-dashboard-directive-ledger-your-real-in-flight-pantheo`
- PANTHEON_RULES §2 (A7, A23, A26); ADR-050 §4 (task ledger CLI)
