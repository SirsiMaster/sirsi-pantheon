# Nexus Ledger Board — Cross-Repo Coordination

**Status:** Active  
**Owner:** claude-pantheon (coordinating both Pantheon + Nexus per owner directive 2026-08-03)  
**Counterpart:** claude-nexus (SirsiNexusApp implementation)  
**Refs:** LEDGER_BOARD_COMPONENT_SPEC.md, ADR-050, ADR-051, A26 (cross-repo mandate)

---

## Mandate

Owner directive 2026-08-03 (router item `20260803-163213`): claude-pantheon coordinates
the Ledger Board build for **both** Pantheon surfaces (menubar/TUI/SwiftUI) and Nexus
(SirsiNexusApp) — one team, one spec, visually identical.

This document is the handshake between the two repos. It is **not** a design document
(that is `LEDGER_BOARD_COMPONENT_SPEC.md`); it is the cross-repo coordination record.

---

## What Pantheon Owns (this repo)

| Artifact | Location | Status |
|----------|----------|--------|
| `internal/ledger` package | `internal/ledger/ledger.go` | ✅ merged (PR #454) |
| `BoardSummary` + `PhaseGroup` types | `internal/ledger/ledger.go` | ✅ this PR |
| `sirsi router ledger [agent] [--json]` | `cmd/sirsi/routerledgercmd.go` | ✅ this PR |
| `sirsi router task add --phase` | `cmd/sirsi/routerledgercmd.go` | ✅ this PR |
| Menubar Ledger section | `cmd/sirsi-menubar/main.go` | ✅ merged (PR #454) |
| Component spec doc | `docs/design/LEDGER_BOARD_COMPONENT_SPEC.md` | ✅ this PR |
| Permanent delete (menubar) | `cmd/sirsi-menubar/actions.go` | ✅ this PR |

## What Nexus Owns (SirsiNexusApp)

Nexus renders the SAME board as a web/app component. The data source is
`sirsi router ledger --json` (subprocess) or a direct call to the Pantheon MCP
`router_ledger` tool if the MCP server is linked.

| Artifact | Notes |
|----------|-------|
| React/web Ledger Board component | Same `BoardSummary` schema, same colors |
| Operator dashboard integration | Nexus's fleet view includes this board |
| Refresh strategy | Poll `sirsi router ledger --json` every 30s, or subscribe via MCP |

---

## Data Contract (shared, do not diverge)

Both sides consume `BoardSummary` from `internal/ledger`. Nexus MUST NOT redefine
this struct — it fetches the JSON via `sirsi router ledger --json` and deserializes it.

If the schema evolves, the change lands in `internal/ledger/ledger.go` here first,
then Nexus updates its deserializer. Pantheon is the canonical source.

```
sirsi router ledger claude-pantheon --json   # Pantheon's own board
sirsi router ledger --json                   # Fleet-wide board
```

---

## Visual Consistency Rule

The Nexus component MUST use the status colors from the spec:

| Status | Color |
|--------|-------|
| done | `#1D9E75` |
| active/in-review | `#EF9F27` |
| queued | neutral gray |
| blocked | `#D85A30` |

Any divergence from these colors is a visual bug that breaks cross-surface consistency.
File a defect against claude-nexus if Nexus renders different colors.

---

## Coordination Task (Ledger Board entry for this workstream)

This cross-repo coordination work is registered as a Pantheon task:

```
sirsi router task add claude-pantheon nexus.ledger.coord \
  --subject "Coordinate Ledger Board in Nexus (SirsiNexusApp)" \
  --phase "Cross-Repo" \
  --status "in-progress" \
  --responsible-party "self"
```

Track progress via `sirsi router ledger claude-pantheon`.

---

## Handoff to claude-nexus

When Pantheon's implementation is complete (this PR merged), route a review item to
claude-nexus with:

1. The merged PR number and `BoardSummary` JSON schema
2. The spec doc path (`docs/design/LEDGER_BOARD_COMPONENT_SPEC.md`)
3. Status colors and visual language requirements
4. Data fetch command (`sirsi router ledger --json`)

claude-nexus then owns the Nexus-side implementation as a separate PR in SirsiNexusApp,
keeping the same visual language. Pantheon does NOT push code into SirsiNexusApp (A26).
