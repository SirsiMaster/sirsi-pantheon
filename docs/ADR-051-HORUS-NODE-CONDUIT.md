# ADR-051: Horus as Per-Node Conduit — Router + Observability are One Flow

## Status
**Accepted** — 2026-08-03

**Deciders:** Cylton Collymore (owner), claude-nexus (relayed), claude-pantheon (implementer)

## Context

ADR-015 established Horus as "Local Workstation Lord" and Ra as "Fleet Lord." ADR-017 named
Horus the per-desktop runtime node. Neither ADR made the following precise:

1. **Horus is the conduit**, not merely a dashboard. Every router item and every observability
   signal on a node flows _through_ Horus before reaching Ra.
2. **One Horus per physical/virtual node.** Not per process. Not per repo. One.
3. **Anubis = the single-node product**: it bundles SNE (Sirsi Node Engine, profile selected)
   plus a local Horus instance.
4. **Ra aggregates Horus reports.** Ra is the fleet lord _because_ it collects what all
   Horus instances report upward. The internal dev fabric is Ra deployment #1.

Owner directive (verbatim, 2026-08-01/02):
> "The router conduit is horus. horus is a shared node resource that is aggregate across
> pantheon as Ra. Ra is the aggregator of horus reports so to speak which informs the entire
> fabric."

## Decision

### Canonical Hierarchy

```
Ra 𓇶 (Fleet Aggregator — Enterprise SKU)
 └── receives ConduitReports from all Horus instances
 └── internal dev fabric = Ra deployment #1

Horus 𓂀 (ONE per node — the shared node conduit)
 └── router items + observability = one unified flow
 └── per-node singleton: keyed on hostname, persisted to ~/.config/sirsi/horus/conduit.json

Anubis 𓃣 (single-node product)
 └── SNE (Sirsi Node Engine, profile selected)
 └── local Horus instance
```

**One Horus per node is per-node uniqueness, not per-process uniqueness.** Multiple
processes (menubar, TUI, CLI) on the same machine share the same conduit identity. The
identity is a file on disk (`conduit.json`) — whoever writes it first creates it; subsequent
openers load it unchanged.

### Router + Observability = One Flow

Prior to this ADR, router queue depth and workstation health (WorkstationReport) were
separate concerns. This ADR unifies them: `ConduitReport` carries _both_. Ra cannot see
half the picture — it receives `ConduitReport{RouterItems, WorkReport}` and gets a complete
view of each node in one struct.

### Telemetry Opt-In = Horus → Ra

Anubis's telemetry opt-in is precisely the toggle on `NodeConduit.TelemetryOn`. When the
user enables telemetry, the local Horus sends ConduitReports to Sirsi's Ra. When disabled,
all data stays local. No partial telemetry; the conduit is on or off.

### Implementation

- `internal/horus/conduit.go` — `NodeConduit`, `ConduitIdentity`, `ConduitReport`
- `OpenConduit()` — idempotent; creates or loads `~/.config/sirsi/horus/conduit.json`
- `SetTelemetry(bool)` — persists user opt-in; the only gate on Horus→Ra data flow
- `BuildReport(routerPending int, ws *WorkstationReport) ConduitReport` — assembles
  the unified payload for Ra

### What Changes in PANTHEON_RULES.md

The deity table row for Horus changes from "Code Graph | Extracts structural code symbols"
to "Node Conduit | Per-node singleton conduit; unified router + observability flow reporting
to Ra." The code graph capability (`internal/horus` package: SymbolGraph, Parser, Watcher)
remains unchanged — it is one capability of the Horus package, not its identity.

## Alternatives Considered

1. **Separate router-conduit and observability-conduit**: Two structs, two flows to Ra.
   Rejected: owner directive is explicit that they are one. Ra should not need to join
   across two streams to understand a node.

2. **Per-process Horus singleton**: Each OS process has its own conduit.
   Rejected: defeats "shared node resource." The menubar, TUI, and CLI would report
   conflicting router-item counts to Ra.

3. **UUID-based NodeID (not hostname)**: More robust for multi-NIC or container nodes.
   Noted with a `ponytail:` comment in the code. Not implemented now — hostname is
   sufficient for Phase 1 (single-machine, non-containerized). Extend when fleet
   mode encounters multi-NIC node disambiguation.

## Consequences

- **Positive**: Clear canonical answer to "what is Horus?" — it is the conduit. No
  ambiguity between "code graph" and "workstation lord."
- **Positive**: `ConduitReport` gives Ra a single typed payload per node. Future Ra
  aggregation code only needs to iterate `[]ConduitReport`, not fan out across
  multiple sub-queries.
- **Positive**: Telemetry opt-in is one field, one toggle, one code path.
- **Risk**: `conduit.json` is keyed on hostname. If a user changes their hostname, a
  new identity is created. Acceptable at Phase 1 scale; extend `NodeID` to a UUID
  when fleet mode matures (marked with `ponytail:` comment).

## References

- ADR-015: Deity Hierarchy (Horus as Local Lord, Ra as Fleet Lord)
- ADR-017: Ra/Horus CTR Hypervisor (original Horus-as-desktop scope)
- ADR-026: Horus Ops-Dashboard (NodeStatus read model — complements NodeConduit)
- Rule A25: Deity Registry & Attribution
- Owner directive: `.agents/idea-router/items/20260802-014842-claude-nexus-claude-pantheon-addendum-horus-per-node-router-conduit-ra-aggregator-of-horu.md`
