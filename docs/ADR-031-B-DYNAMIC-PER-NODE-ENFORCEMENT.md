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

## Pressure source — RESOLVED (claude-home binding + codex-pantheon SME, 2026-06-19)

The pressure-source question is **decided**: macOS pressure is observed through **`DISPATCH_SOURCE_TYPE_MEMORYPRESSURE`** (the kernel/libdispatch source) as Hapi's **primary** signal. `memory_pressure -Q` / `sysctl` snapshots are **diagnostic / bootstrap fallback only**, never the governance contract.

**Why this, not the no-cgo poll** (the open question is closed by codex's *tested* evidence): on this host `sysctl kern.memorystatus_vm_pressure_level` returns **`Operation not permitted`** to an unprivileged daemon, and `memory_pressure -Q` returns only a point-in-time *free percentage* (a hardcoded-percent equivalent — the very thing #4 removes), not the kernel's NORMAL/WARN/CRITICAL *level*. The dispatch source is the only unprivileged source of the actual node-relative level. claude-home's binding review deferred the final API call to this SME verdict.

**Signal vs. policy (reconciles claude-home's architectural point):** the dispatch handler does **only** a tiny state update — it stores the last-observed level in `NodeCapacity` and enqueues. **All policy runs in Hapi's existing bounded loop**, combining `(kernel level) + (governed RSS / measured high-water) + (current concurrency) + (NodeCapacity)`. So there are no sub-tick policy wakes; the **hard MLX cap (`set_wired_limit`) remains the instant fast backstop** for a fast balloon between ticks, and Hapi stays the slower governance layer — exactly claude-home's framing.

**Implementation contract:**
- Tiny interface: `type PressureLevel int { Normal, Warn, Critical, Unknown }`; `Subscribe(ctx, chan<- PressureEvent) error`.
- Darwin impl behind `//go:build darwin && cgo`: serial dispatch queue + `DISPATCH_SOURCE_TYPE_MEMORYPRESSURE`, mask `NORMAL|WARN|CRITICAL`, `dispatch_resume` after handler registration; handler reads `dispatch_source_get_data` (a **bitmask** — pick the most severe flag) and posts a compact event. **No Go pointers into retained C** — the C shim updates an atomic primitive Go reads, or calls an exported Go callback with primitives only. Handler must **not** purge/traverse caches (aggravates VM pressure).
- Fallback for `darwin && !cgo` and non-Darwin: a bootstrap snapshot then `Unknown`/no-op — **never fabricate thresholds**. (The cross-platform agent binary stays `CGO_ENABLED=0`, Rule A3, and uses the fallback; cgo is acceptable for the Mac CLI/menubar/daemon surface, which already links objc.)
- **Action policy with hysteresis:** `NORMAL` → normal scheduling (still honor dynamic cap + measured budget); `WARN` → stop admitting warm starts, lower concurrency, drain (debounce ~1–3 s); `CRITICAL` → refuse new inference, pause admission, shed governed jobs **within the consent invariant** (act immediately). Require sustained `NORMAL` (~10–30 s / N clean samples) before raising caps again. Seed from a snapshot — **do not assume an initial event**; store last level in `NodeCapacity`.

This supersedes the earlier "lean no-cgo" note; the dispatch source is canon for #4.
