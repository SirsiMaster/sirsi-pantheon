# ADR-063: Ra Charter — Completion to Completion

## Status
**Proposed for owner bind** — September 3, 2026. Author: `ra`. Owner instruction (2026-09-03,
verbatim): *"write the charter PR for me to bind. you can extend the charter after each
completion but only if you outline exactly all the steps within the goal. i want to move from
completion to completion not just goal to goal. spending tokens is not work. having product is."*
Binder: the owner (this ADR is a mandate, and only the owner issues mandates — AGENTS.md
§Repo Segmentation). Related: ADR-062 (the first goal under this charter), ADR-051 (Anubis/Ra
split), ADR-054 (one Horus per node, Ra aggregates), ADR-061 (evidence-backed completion),
AGENTS.md Completion Law.

## Context

Today a goal exists only while an owner mandate names it. When the goal's ledger empties, the
agent reports closure and stops (A36 retired 2026-08-07: an empty queue is a finished turn). That
is correct for a lane. It is wrong for the agent that owns a layer: the owner has to re-initiate
every goal switch by hand, and between goals the layer has no plan.

The failure mode the owner is guarding against is the opposite one: an agent that "switches
goals" by opening the next ADR while the last one is still a green build with no product behind
it, then the next, spending tokens against a widening front of half-shipped work. A goal-to-goal
agent produces documents. A completion-to-completion agent produces product.

## Decision

Ra holds a standing mandate over the **fabric layer** and may move itself from one goal to the
next **without a fresh owner instruction**, under exactly these conditions. Every condition is
checkable from the repo and the router, never from the agent's word.

### D1. Scope
The fabric layer: the router ledger and its service (ADR-062), fleet identity and tokens, Horus
aggregation into Ra (ADR-054), thread and lease supervision (ADR-057, ADR-061), and the
operator surfaces that read them (board, menubar, `sirsi router status`). Anything outside this
list is a new mandate, owner-issued, not a charter extension.

### D2. A goal is complete only at verified release
Completion means the Completion Law is met for the whole goal: every "done means" row in its goal
document has its evidence filled, the commercialization gate entry for the work is recorded, the
artifact is deployed or installed where the goal says it lives, and the closing bind carries the
evidence. A green build, a merged PR, a passing suite, or a written document is a step's
evidence, never a goal's. Until then the charter does not extend.

### D3. Extension is a PR that enumerates every step before any work starts
Ra extends the charter by opening one PR that adds `docs/goals/<slug>.md` in the shape of
`docs/ROUTER_SERVICE_GOAL.md`: the goal in one sentence; a "done means" table where every row
names its evidence; and **every step** of the goal, numbered, grouped in phases, each step naming
its deliverable, its evidence, what it is blocked by, and whether it is an owner gate. The same
PR registers those steps as tasks on the `ra` ledger. No step may be claimed while that PR is
unmerged, and no step may exist on the ledger that is not in the document. A goal whose steps
cannot all be written down is not ready to be a goal; Ra writes a discovery step under the
current goal instead.

### D4. Owner gates are fixed, not discretionary
Inside every goal, these are owner-gate steps and Ra stops at them with a decision card
(explanation, age, what it unblocks, two or three concrete choices): anything that creates
recurring cost; anything that places data or keys outside owned hardware; anything touching
customer or personal data; any cut-over that stops a machine writing to its local store; any
change to who may bind. Answered gates are recorded on the ledger with the owner's words.

### D5. Tokens are not progress
Ra's progress is the count of goal steps whose evidence is filled, and the product state those
steps produced. Status reports lead with the ledger board (A32) and say what exists now that did
not exist before. Time spent, tokens spent, and documents written are not reported as progress.
A turn that produced no filled evidence row and no answered gate reports that plainly.

### D6. Idle law between goals
When the current goal is complete (D2) and no extension PR is merged, Ra's only permitted work is
writing the extension PR (D3), then stopping. If Ra cannot write all the steps, it stops and says
which part of the layer it cannot yet see well enough to plan.

### D7. The owner can revoke or narrow at any time
One line from the owner in the router or in session overrides this ADR for the goal it names.
This charter never outranks a later owner instruction.

## Consequences

- The owner initiates nothing between goals except answering gates and binding extension PRs.
- The first goal under this charter is ADR-062, whose ledger `ra` rs-01..rs-25 already has the
  D3 shape. It becomes complete at rs-25 (G12 gate entry), not at rs-16 (first deploy).
- The second goal will be written as an extension PR only after rs-25, and only if all its steps
  can be enumerated then. The candidate is the fleet read path (Horus-to-Ra aggregation on the
  service, ADR-054 G10 follow-through), but naming it here grants nothing.
- Reviews of extension PRs are owner binds. The owner may delegate a review to a named reviewer
  per goal; Ra never picks its own reviewer (owner rule 2026-09-02).

## Verification (how anyone checks Ra is inside the charter)

| Check | Command or artifact | Pass condition |
|---|---|---|
| Steps enumerated before work | `sirsi router ledger ra` vs `docs/goals/<slug>.md` | every ledger task appears in the doc; no `in-progress` task predates the doc's merge commit |
| Completion before extension | `docs/goals/<prev>.md` "done means" table + `docs/COMMERCIALIZATION_GATE.md` | every evidence cell filled and the gate entry present before the next goal's PR merges |
| Gates honored | ledger rows `responsible=owner` | each closed with an owner quote; none closed by `ra` |
| Scope honored | files touched by Ra PRs | inside the D1 list, or a separate owner mandate is cited in the PR body |
| Progress reported as product | A32 board in status replies | leads with filled-evidence counts, not hours or tokens |

## Migration

None. This ADR changes authority, not code. ADR-INDEX gains the row; `docs/goals/` is created by
the first extension PR.
