# ADR-031-B — Dynamic Per-Node Enforcement (the numbers become functions of the measured node)

**Status:** Proposed (design — owner Priority-0 correction 2026-06-19; binding review claude-home; SME codex-pantheon on the kernel memory-pressure source) · *amends ADR-031-A*
**Custodian:** 𓁢 Pantheon (runtime) · 🌊 Hapi (resource governance) · 🪨 Seba (hardware self-model)
**Refs:** ADR-031-A (never exhaust the host — the invariant + 4 layers); PANTHEON_RULES A1/A5/A23; the 2026-06-19 owner directive *"dynamically understand EACH node's constraints and balance within-node + across-node."*

---

## Context

ADR-031-A is **correct in its invariant** (Pantheon must never spawn a process that can exhaust the host) but **wrong in its implementation**: it hardcodes constants tuned for one machine — a 48 GB M5 Max. That contradicts the entire thesis of Pantheon / SirsiNexus, which is to *dynamically understand each node's constraints and balance load within and across nodes*. A governor that only works because someone hand-tuned `8 GB` for one box is not a governor — it is a configuration that happens to be safe here.

**This is NOT a fire.** The 48 GB constants are *conservative* on this box — nothing is broken, nothing is at risk right now. This is the **generalization the product requires**, to be done before polishing surfaces (per owner: "before more menubar").

The hardcoded literals, all to be replaced:

| # | Hardcoded today (ADR-031-A) | Where |
|---|---|---|
| 1 | `concurrency` default = **1** | `gemmaServeConcurrency`, `gemmaSafeConcurrency` |
| 2 | headroom = **8 GB** fixed | `capHeadroom`, `gemmaSafeConcurrency` |
| 3 | cap = `free − 8 GB` | `gemmaServerStart` cap computation |
| 4 | suspend at `free < 8%`, kill at `free < 4%` | `MemGovernor` thresholds (`hapi.go`) |
| 5 | serial budget = **2× model** | `gemmaSafeConcurrency` |

## Decision

> **The invariant is a constant; every number that enforces it becomes a function of the measured node.** All thresholds, budgets, and concurrency derive from a live `NodeCapacity` self-model — never from literals. Where the OS already computes a node-relative signal (memory pressure), subscribe to it rather than re-deriving it from a hardcoded percent.

### The foundation — `NodeCapacity` (Seba/Hapi)

A live struct that models *this* node, refreshed on a bounded tick and exposed by `sirsi hapi status`:

```
type NodeCapacity struct {
    TotalRAM        int64          // seba.DetectHardware
    FreeRAM         int64          // vm_stat (free+inactive+speculative)
    GPUCores        int            // seba — VRAM ceiling for model residency
    VRAMBudget      int64          // GPU/unified-memory budget for MLX
    CPUCoresP, CPUCoresE int       // P/E split — QoS, not hard pinning
    PressureLevel   MemPressure    // from the kernel source (below), NOT a percent
    PerProcessRSS   map[int]int64  // who holds what (agents, foreground, governed)
    GovernedPIDs    map[int]string // consented compute
    OSBaselineRSS   int64          // measured idle OS footprint
}
```

Everything reads from `NodeCapacity`. On 48 GB/12 GB-model it yields concurrency ~1–2; on 256 GB it yields 15+ — the warm broker's real value (multi-concurrent on the GPU) appears automatically on machines that can hold it.

### The five mappings (hardcoded → dynamic)

1. **Concurrency** → `maxSlots = floor((FreeRAM − dynamicReserve) / measuredPerModelBytes)`, **also capped by VRAM/GPU** (`min` of the RAM-derived and VRAM-derived slot counts), floor 1. No fixed default.
2. **Headroom** → `dynamicReserve = OSBaselineRSS + live(Claude+Codex+foreground)RSS + margin`, where `margin = max(8 GB, k · TotalRAM)` — a *floor* of ~8 GB but **proportional above it**, never a flat constant.
3. **Cap** → `cap = FreeRAM − dynamicReserve` (the per-node wired/mem limit fed to the MLX cap wrapper).
4. **Suspend/kill thresholds** → **observe, don't threshold.** Subscribe to the kernel's `DISPATCH_SOURCE_MEMORYPRESSURE` (NORMAL / WARN / CRITICAL) — macOS computes this *relative to the machine*, so it is node-proportional for free. Hapi acts on `(kernel pressure level) + (its own RSS accounting) + (trend)`, not a literal percent. The `MemTier` enum maps onto the kernel levels; the `free%` path remains only as a fallback when the dispatch source is unavailable. *(codex-pantheon SME on the dispatch-source wiring.)*
5. **Serial budget** → **measure** peak resident once (the cap wrapper reports the actual high-water mark per model), cache per-model; fall back to `2×` only until a real measurement exists.

### Cross-node trajectory (design now, build later — Ra / SirsiNexus)

Each node's `NodeCapacity` + current load is the **unit a fleet layer balances across**. A node near pressure sheds or refuses inference; a node with headroom accepts more. **The per-node dynamic governor is the prerequisite** — there is nothing to balance until each node models itself. So `NodeCapacity` must be expressed in a form Ra/Nexus can consume (serializable, versioned), even though the fleet balancer is future work.

## Consequences

- **The invariant is unchanged and still enforced** — the hard cap, the cold-path single-flight, and consent-based Hapi governance all stay. Only the *numbers* feeding them become measured.
- ADR-031-A is amended: *invariant = constant; enforcement = dynamic, and (future) fleet-aware.*
- `sirsi hapi status` gains the `NodeCapacity` view (what this machine is, what's governed, current kernel pressure).
- Honest scaling story replaces the apologetic one: "safe concurrency is 1 on 48 GB" becomes "concurrency is whatever this node can hold," true on a laptop and a 512 GB server alike.
- **Sequencing:** kernel memory-pressure source (#4) + `NodeCapacity` struct first (the foundation), then re-point the five call sites, then expose `NodeCapacity` for Ra. Route the refactor PR(s) → claude-home binding review; codex-pantheon SME on `DISPATCH_SOURCE_MEMORYPRESSURE`. Broker stays disabled until the dynamic stack is reviewed + the owner re-enables.

## Open SME question (codex-pantheon)
`DISPATCH_SOURCE_MEMORYPRESSURE` is a C/dispatch API. Go access options: (a) a tiny cgo shim subscribing to the dispatch source and emitting level changes on a channel; (b) shell `memory_pressure`/`sysctl kern.memorystatus_vm_pressure_level` polled on the existing tick; (c) read the pressure level via `host_statistics64`/`vm_pressure`. Recommend the lowest-complexity option that gives reliable NORMAL/WARN/CRITICAL transitions without cgo if possible (A23 advanced-simplicity). SME verdict requested before implementation.
