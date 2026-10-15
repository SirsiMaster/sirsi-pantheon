# Ra–Horus Fabric — Diagram index (SL-DIAGRAM-001)

**Wing:** `stacklab.wing.ra-horus-fabric` · **Owner of every row:** Ra unless stated · **Source revision:** main `c1a78077` · **Last review:** 2026-10-05

Coverage is reported against the whole process inventory below, not against what is already drawn.

**Covered (logical + data): 7 of 18. Drafted, awaiting its owner's confirmation: 1. Open: 10.**

Separately, `ra/sl-diagram-001-ra-p06-p07-p16` (PR #1047, open, CI green, awaiting review) draws RA-P06, RA-P07, RA-P16 — not reflected in this branch's count until that PR merges.

State, sequence and recovery views are marked per row. A row is complete only when it has a logical view and a data view; every gap stays OPEN with a next action.

| ID | Process | Trigger | Logical | Data | State / sequence / recovery | Implementation | Next action |
|---|---|---|---|---|---|---|---|
| RA-P01 | Wake loop (reactive inbox consumer) | launchd starts the job | [RA-P01](diagrams/RA-P01-wake-loop.md) | same file | state + recovery | implemented; progress gate and spawn ceiling merged, **undeployed** (#639) | none |
| RA-P02 | Known-failure loop | consumer output of a failed lane | [RA-P02](diagrams/RA-P02-known-failure-loop.md) | same file | recovery | implemented; no self-applying fix | auto-apply fix kind (feature, not a diagram gap) |
| RA-P03 | Origin-pinned registry | `router registry sync`, hourly job | [RA-P03](diagrams/RA-P03-registry-pin.md) | same file | recovery | implemented | decide whether a stale pin fails closed |
| RA-P04 | Release train | `scripts/release-train.sh` | [RA-P04](diagrams/RA-P04-release-train.md) | same file | recovery | implemented, observed (v0.24.69) | scripted rollback is absent |
| RA-P05 | Review and bind (SSA) | PR opened | [RA-P05](diagrams/RA-P05-review-and-bind.md) | same file | recovery | owner: sirsi-software-admin; **Ra's understanding, awaiting SSA confirmation** | SSA confirms or corrects (item sent 2026-10-05) |
| RA-P06 | Item lifecycle: send, pull, acknowledge, claim, close, reopen, reassign, dismiss | any lane | OPEN | OPEN | OPEN | implemented | draw from `internal/routerstore/items.go` and the dispatch facade |
| RA-P07 | Thread registration and heartbeat | session start | OPEN | OPEN | OPEN | implemented | draw |
| RA-P08 | Spool relay forwarding | any lane call | [RA-P08](diagrams/RA-P08-spool-relay-forwarding.md) | same file | recovery | implemented | none |
| RA-P09 | Router service authorization (host token, thread binding, audience log) | every gated call | [RA-P09](diagrams/RA-P09-router-service-authorization.md) | same file | recovery | implemented | none |
| RA-P10 | Quarantine stand-down and lift | owner or Ra | OPEN | OPEN | OPEN | implemented | draw |
| RA-P11 | Horus supervisor duties: dispatch pump, hourly sweep, registry police, thread census | supervisor cadence | OPEN | OPEN | OPEN | implemented | draw |
| RA-P12 | Lane escalation: "lane needs you" alert and auto-resolve | lane unreachable | [RA-P12](diagrams/RA-P12-lane-escalation.md) | same file | recovery | implemented | none |
| RA-P13 | Swap hygiene sampling | `sirsi swap-hygiene` | OPEN | OPEN | OPEN | implemented | draw |
| RA-P14 | Dashboard read path | `sirsi dashboard` | OPEN | OPEN | OPEN | implemented | draw: bounded reads (`ListActive`, `ListSince`, `CountClosed`) |
| RA-P15 | Claim eligibility (`router task why`) | refused claim | OPEN | OPEN | OPEN | implemented | draw |
| RA-P16 | Mailbox alias and reassign | send to a retired lane | OPEN | OPEN | OPEN | implemented | draw (ADR-072 C5) |
| RA-P17 | Router service deploy | release with a new Store method | partly in RA-P04 | OPEN | OPEN | implemented | separate view |
| RA-P18 | Ma'at pre-push gate and CI | `git push`, PR | OPEN | OPEN | OPEN | implemented; owner claude-pantheon | claude-pantheon draws or confirms (item sent 2026-10-05) |

Editable source is the Mermaid in each linked file. Each file renders in any Mermaid viewer.
