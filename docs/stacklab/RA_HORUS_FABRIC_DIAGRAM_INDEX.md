# Ra–Horus Fabric — Diagram index (SL-DIAGRAM-001)

**Wing:** `stacklab.wing.ra-horus-fabric` · **Owner of every row:** Ra unless stated · **Source revision:** main `c1a78077` · **Last review:** 2026-10-05

Coverage is reported against the whole process inventory below, not against what is already drawn.

**Covered (logical + data): 5 of 18. Drafted, awaiting its owner's confirmation: 1. Open: 12.**

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
| RA-P08 | Spool relay forwarding | any lane call | OPEN | OPEN | OPEN | implemented | draw; include the 30 s timeout class |
| RA-P09 | Router service authorization (host token, thread binding, audience log) | every gated call | OPEN | OPEN | OPEN | implemented | draw; confirm what authenticates a lane |
| RA-P10 | Quarantine stand-down and lift | owner or Ra | OPEN | OPEN | OPEN | implemented | draw |
| RA-P11 | Horus supervisor duties: dispatch pump, hourly sweep, registry police, thread census | supervisor cadence | OPEN | OPEN | OPEN | implemented | draw |
| RA-P12 | Lane escalation: "lane needs you" alert and auto-resolve | lane unreachable | OPEN | OPEN | OPEN | implemented | draw from `internal/router/laneescalation.go` |
| RA-P13 | Swap hygiene sampling | `sirsi swap-hygiene` | OPEN | OPEN | OPEN | implemented | draw |
| RA-P14 | Dashboard read path | `sirsi dashboard` | OPEN | OPEN | OPEN | implemented | draw: bounded reads (`ListActive`, `ListSince`, `CountClosed`) |
| RA-P15 | Claim eligibility (`router task why`) | refused claim | OPEN | OPEN | OPEN | implemented | draw |
| RA-P16 | Mailbox alias and reassign | send to a retired lane | OPEN | OPEN | OPEN | implemented | draw (ADR-072 C5) |
| RA-P17 | Router service deploy | release with a new Store method | partly in RA-P04 | OPEN | OPEN | implemented | separate view |
| RA-P18 | Ma'at pre-push gate and CI | `git push`, PR | [RA-P18](diagrams/RA-P18-maat-gate.md) | same file | state + recovery | implemented; owner claude-pantheon | none — confirmed by claude-pantheon 2026-10-06 |

Editable source is the Mermaid in each linked file. Each file renders in any Mermaid viewer.
