# PRD — SNE Heterogeneous Compute & Allocation Governance

**Owner:** claude-home · **Status:** draft for review · **Date:** 2026-08-06
**Source of truth:** [`docs/APPLE_SILICON_COMPUTE_MAP.md`](../APPLE_SILICON_COMPUTE_MAP.md)

---

## 1. Problem

SNE's name promises a *Sirsi Neural Engine* that spreads work across Apple
Silicon. Measured, it is a **Metal-only engine**: `CoreML` is not linked, `BNNS`
and `AMX` symbol counts are zero, and `mlx_` accounts for 1063 symbols. The ANE
cannot be used at all by construction, and SME2 — present and enabled on this
chip — is untouched.

The consequence is not academic. On 2026-08-06 a single SNE process reached
**36.33 GB peak, 97% of the GPU's 37.4 GB recommended working set**, while a
nominal "20 GB limit" was in force that the engine itself documents as
`scheduler_backpressure_not_allocation_cap`. Host free memory fell to 41%, load
average hit 50, and the machine became unusable.

**One store saturated, three idle, one memory pool, no arbiter.**

### Why this is a product problem, not a tuning problem

A local model that makes the machine unusable while running is not a feature. The
value proposition of on-device inference is *sovereignty without penalty*. Today
the penalty is the whole machine. Until compute is distributed and allocation is
governed, every downstream Sirsi promise built on Tier-0 local inference (A30) is
resting on something that cannot be run continuously.

---

## 2. Goals

| # | Goal | Measure of success |
|---|---|---|
| G1 | Inference never exceeds a governed share of the GPU working set | peak ≤ 60% of `recommendedMaxWorkingSetSize`, enforced externally |
| G2 | Embeddings and short-text classification leave the GPU | ANE-resident, GPU contention from these workloads → 0 |
| G3 | Ranking / similarity math leaves the GPU | SME2 path via Accelerate |
| G4 | Prefill exploits M5's in-GPU Neural Accelerators | measurable prefill improvement vs shader-only baseline |
| G5 | Memory guards read truth | every guard reads `mlx_active_bytes`, zero read RSS |
| G6 | Agent workload bounded | load average stays < core count under normal fabric operation |
| G7 | Host stays usable | interactive latency unaffected while inference runs |

### Non-goals

- **Moving token generation to the ANE.** Decode is bandwidth-bound; the ANE
  offers no win and dequantizes INT weights to fp16. Generation stays on the GPU.
  This PRD explicitly rejects "route everything to the ANE."
- Replacing MLX. MLX is the correct GPU path; this extends around it.
- Windows/Linux parity (ADR-032 — Mac-first).
- Fixing Electron's single main thread. Out of our control (§6).

---

## 3. Users and jobs

| User | Job | Today's failure |
|---|---|---|
| Owner on one Mac | run agents + IDE + local model at once | machine locks up |
| Tier-0 triage (A30) | screen router items cheaply | competes with generation for GPU |
| gemma-the-builder | draft code headlessly | starves the interactive session |
| Horus / doctor | report honest resource state | reads RSS, blind to 27 GB |

---

## 4. Requirements

### R1 — External allocation cap *(G1)*

The cap must live **outside the process it governs**; a limit inside the allocator
it bounds is not a limit (Rule A35).

- Budget authority is `MTLDevice.recommendedMaxWorkingSetSize` (37.4 GB here),
  never a hand-set constant.
- Default inference budget: **60% of recommended working set**, configurable.
- Admission control refuses a request that would exceed budget, with a typed
  error — never a silent OOM or a Jetsam.
- Surfaces must render the field's real semantics. `mlx_memory_limit_bytes` is
  displayed as **"backpressure threshold"**, never "limit".

**Acceptance:** a deliberate over-budget request is refused and named; verified in
both directions (passes under budget, fails over).

### R2 — KV sized to the qualified envelope *(G1)*

`context_window = 262144` against `qualified_prompt_tokens = 1024` is a 256× gap.
KV allocation must derive from the qualified/sliding envelope, with headroom
growth on demand — not from the theoretical maximum.

**Acceptance:** idle-to-steady-state active memory for a 12B model stays under
15 GB; per-request growth remains 0.00 GB across ≥ 50 requests.

### R3 — Embeddings on the ANE via Core ML *(G2)* — **first build**

Link Core ML; convert the embedding model; serve embeddings from the ANE.

- Separable from generation: no change to the MLX token path.
- Expected profile (Apple's DistilBERT reference): ~10× faster, ~14× less peak
  memory, ~2 W vs ~20 W.
- Go reaches Core ML through a Swift/ObjC bridge — the known cost (week+, per the
  2026-07-21 research pass). This is accepted deliberately.

**Acceptance:** embeddings served with GPU `mlx_active_bytes` unchanged during
embedding load; power and latency measured against the GPU baseline; a repro
script ships with the numbers (A14/A33 — every public number reproducible).

### R4 — SME2 path for similarity / ranking *(G3)*

`FEAT_SME`, `FEAT_SME2`, `FEAT_I8MM`, `FEAT_BF16` are all enabled. Route vector
similarity, ranking, and dedup through Accelerate BLAS.

- Lowest cost / highest ratio of the four: a linker flag and cgo BLAS, hours not
  weeks. **Adopt first among the compute changes.**
- Supersedes the older "AMX" framing — SME2 is documented ARM ISA, not a private
  Apple extension.

**Acceptance:** ranking throughput vs the current path, measured; zero GPU
allocation attributable to ranking.

### R5 — Metal 4 tensor ops for prefill *(G4)*

M5's Neural Accelerators are matmul units **inside** GPU cores, distinct from
shader ALUs, reached via Metal 4 tensor operations. Prefill is dense matmul — the
exact shape they exist for. This is the one new unit that improves the existing
path rather than requiring a second runtime.

**Acceptance:** prefill latency vs shader-only baseline on identical prompts,
repro script included. **Risk:** per-core accelerator counts are not locally
queryable — benefit must be demonstrated by benchmark, not asserted from spec.

### R6 — Guards read truth, not RSS *(G5)*

Every memory guard reads `mlx_active_bytes` from `/health`. RSS understates the
broker by **27 GB** and has already produced a false "host healthy" verdict.

**Acceptance:** grep shows zero RSS-based memory decisions in `guard`, `hapi`,
`doctor`, and the menubar; a regression test asserts a 30 GB Metal allocation is
*seen*.

### R7 — Bounded agent workload *(G6)*

- Worker builds capped: `GOMAXPROCS=4`, `go test -p 2 -parallel 2`.
- Router dispatch capped: max concurrent lanes and max concurrent workers.
- Load-average backpressure: dispatch defers above a threshold.
- An operator off-switch the supervisors **honour rather than race** — three
  teardown attempts reverted on 2026-08-06 (`bootout` re-bootstrapped in 40 s;
  `launchctl disable` cleared outright; `horus.agent-router` restarted via
  KeepAlive and reinstalled all 24 lanes).

**Acceptance:** with the fabric at full width, load average stays below core
count; a single documented command stops dispatch and it *stays* stopped.

### R8 — Index hygiene *(G7)*

Spotlight must not index machine state agents rewrite continuously. Measured
`mds` 18.4% → 0.0–0.8% after excluding `~/.sirsi`, `~/.claude`, `~/go`,
`~/.cache`, `/private/tmp/claude-501`.

**Acceptance:** `sirsi setup` applies exclusions idempotently; doctor reports an
un-excluded high-churn path as a finding.

---

## 5. Sequencing

Ordered by **value ÷ cost**, not by architectural tidiness.

| Phase | Work | Cost | Why here |
|---|---|---|---|
| **0** | R6 guards read truth · R8 index hygiene · R7 caps | hours | stops active harm; R8 already measured |
| **1** | R1 external cap · R2 KV envelope | days | removes the cratering class outright |
| **2** | R4 SME2 ranking | hours–days | cheapest real distribution; linker flag |
| **3** | R3 embeddings → ANE | week+ | the headline win; unblocks the premise |
| **4** | R5 Metal 4 prefill | days–weeks | benefit must be benchmarked first |

Phase 0 and 1 are host-stability work and should not wait on the compute work.
**A 5-bit model swap already took peak 36.33 → 10.36 GB** (measured, live) — that
is a Phase-0-grade mitigation available immediately and already applied.

---

## 6. Constraints and honest limits

- **Electron cannot be distributed.** Claude, Codex, and VS Code pin their main
  thread by construction. We do not control renderer scheduling. What we control
  is everything they *spawn* — agent workers, builds, test suites — which is where
  the measured oversubscription actually originates.
- **Core ML is the only ANE door.** MLX exposes exactly `MLX_CPU` and `MLX_GPU`
  and has no Neural Engine path. Any ANE work is a Core ML/Swift-bridge project,
  never an MLX flag.
- **Per-core Neural Accelerator counts are not locally queryable.** R5's benefit
  is a hypothesis until benchmarked. It ships with numbers or it does not ship.
- **Decode will not get faster from any of this.** It is bandwidth-bound. The win
  is *contention removal* and *host usability*, not tokens/sec. Claiming otherwise
  would repeat the inflated-benchmark pattern A14 exists to prevent.

---

## 7. Risks

| Risk | Mitigation |
|---|---|
| Core ML bridge cost overruns (week+) | R4 (SME2) lands real distribution first, independently |
| Metal 4 tensor benefit fails to materialise | R5 gated on benchmark; abandonable without affecting R1–R4 |
| A cap that is again not a cap | R1 enforced externally; falsified in both directions before merge |
| "We use the whole chip" outruns the code | no public claim ships ahead of a repro script (A14, A33) |

---

## 8. Traceability

Every requirement traces to a measured defect on the ledger:

| Req | Ledger task |
|---|---|
| R1 | `mlx-limit-is-not-a-cap` |
| R2 | `context-window-262k-vs-1k-qualified` |
| R3 | `sne-is-gpu-only-ane-unused` |
| R4 | `sne-is-gpu-only-ane-unused` |
| R5 | `sne-is-gpu-only-ane-unused` |
| R6 | `rss-blind-to-metal-memory` |
| R7 | `worker-build-parallelism-unbounded`, `wake-fabric-throttle-not-durable` |
| R8 | (new — spotlight exclusions applied 2026-08-06) |
| — | `capabilities-reports-wrong-model` (correctness of any benchmark above) |

**Refs:** A14 (Statistics Integrity), A30 (Model Tiering), A32 (Load-Bearing
Recognition), A35 (Scope The Check To The Claim), ADR-031-A/B (resource
governance), ADR-032 (Mac-first).
