# Ra–Horus Fabric — Diagram index (SL-DIAGRAM-001)

**Wing:** `stacklab.wing.ra-horus-fabric` · **Owner of every row:** Ra unless stated · **Source revision:** main `0ed15ac9` · **Last review:** 2026-10-10

Coverage is reported against the whole process inventory below, not against what is already drawn.

**Covered (logical + data): 10 of 18 (6 drawn this pass: P10, P11, P13, P14, P15, P17; P06/P07/P16 and P08/P09/P12 are drawn on open, unmerged PRs #1047/#1048 — not yet landed on this branch). Drafted, awaiting its owner's confirmation: 1. Open: 7 (P06, P07, P08, P09, P12, P16, P18).

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
| RA-P10 | Quarantine stand-down and lift | owner or Ra | [RA-P10](diagrams/RA-P10-quarantine-stand-down-and-lift.md) | same file | yes | implemented | none |
| RA-P11 | Horus supervisor duties: dispatch pump, hourly sweep, registry police, thread census | supervisor cadence | [RA-P11](diagrams/RA-P11-horus-supervisor-duties.md) | same file | — | implemented | none |
| RA-P12 | Lane escalation: "lane needs you" alert and auto-resolve | lane unreachable | OPEN | OPEN | OPEN | implemented | draw from `internal/router/laneescalation.go` |
| RA-P13 | Swap hygiene sampling | `sirsi swap-hygiene` | [RA-P13](diagrams/RA-P13-swap-hygiene-sampling.md) | same file | — | implemented | none |
| RA-P14 | Dashboard read path | `sirsi dashboard` | [RA-P14](diagrams/RA-P14-dashboard-read-path.md) | same file | — | implemented | none |
| RA-P15 | Claim eligibility (`router task why`) | refused claim | [RA-P15](diagrams/RA-P15-claim-eligibility.md) | same file | — | implemented | none |
| RA-P16 | Mailbox alias and reassign | send to a retired lane | OPEN | OPEN | OPEN | implemented | draw (ADR-072 C5) |
| RA-P17 | Router service deploy | release with a new Store method | [RA-P17](diagrams/RA-P17-router-service-deploy.md) | same file | — | implemented | none |
| RA-P18 | Ma'at pre-push gate and CI | `git push`, PR | OPEN | OPEN | OPEN | implemented; owner claude-pantheon | claude-pantheon draws or confirms (item sent 2026-10-05) |

Editable source is the Mermaid in each linked file. Each file renders in any Mermaid viewer.
