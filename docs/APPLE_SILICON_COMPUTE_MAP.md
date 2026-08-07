# Apple Silicon Compute & Memory Map — M5 Max

**Status:** measured, not quoted. Every number below was read off this machine on
2026-08-06 with the command shown. Marketing figures and spec-sheet claims are
excluded unless a local probe confirms them; where a fact is *not* locally
verifiable it is marked **[spec]** and the reason is given.

**Machine of record:** `Mac17,6` — Apple M5 Max, 48 GB unified memory, macOS 25F84.

---

## 1. The five compute stores

Apple Silicon is not "a CPU with a GPU". It is **five independently schedulable
compute stores sharing one memory**. The engineering win is matching work to the
right store — never finding the single "fastest" one.

| # | Store | This machine | Reached via | Verified by |
|---|---|---|---|---|
| 1 | **CPU — Performance cores** | 12 | threads / GCD | `hw.perflevel1.physicalcpu` |
| 2 | **CPU — Super cores** | 6 | threads / GCD | `hw.perflevel0.physicalcpu` |
| 3 | **CPU — SME/SME2 matrix engine** | present | Accelerate, compiler intrinsics | `hw.optional.arm.FEAT_SME=1` |
| 4 | **GPU — shader cores** | 40 | Metal 4 | `SPDisplaysDataType` |
| 5 | **GPU — Neural Accelerators** | per-core **[spec]** | Metal 4 tensor ops | see §4 |
| 6 | **ANE — Neural Engine** | present | **Core ML only** | `ioreg` (39 matches) |
| 7 | **Media engines** | present **[spec]** | VideoToolbox | not exercised by SNE |

> `hw.ncpu = 18` (6 Super + 12 Performance). Apple's M5 naming replaces the older
> efficiency/performance split; both classes are full cores here, which is why a
> naive `GOMAXPROCS=18` fan-out oversubscribes so aggressively (§7).

### Measured CPU feature flags

```
hw.optional.arm.FEAT_SME        = 1     scalable matrix extension
hw.optional.arm.FEAT_SME2       = 1     SME2
hw.optional.arm.FEAT_SME_F64F64 = 1     double-precision matrix
hw.optional.arm.FEAT_BF16       = 1     bfloat16
hw.optional.arm.FEAT_I8MM       = 1     int8 matrix multiply
hw.optional.arm.FEAT_DotProd    = 1
hw.optional.arm.FEAT_FP16       = 1
hw.optional.neon                = 1
```

**This supersedes the previous map's "math → AMX".** AMX was Apple-private and
undocumented. **SME/SME2 is architectural ARM ISA** — documented, compiler-
targetable, and reachable without reverse engineering. `FEAT_I8MM` and `FEAT_BF16`
matter directly: quantized inference math has a first-class CPU path that does not
touch the GPU at all.

---

## 2. The memory store — one pool, several ceilings

Unified memory is a single pool, but it is governed by **several different
ceilings, only one of which anything currently enforces.** This is the root of the
host-cratering class of failure.

| Ceiling | Value | Source | Enforced? |
|---|---|---|---|
| Physical RAM | **48.0 GB** | `hw.memsize` | kernel (Jetsam) |
| GPU recommended working set | **37.4 GB** | `MTLDevice.recommendedMaxWorkingSetSize` | **nothing** |
| Max single Metal buffer | **28.1 GB** | `MTLDevice.maxBufferLength` | Metal |
| MLX "memory limit" | 20.0 GB | `/health` | **NOT A CAP — see below** |
| Page size | 16 KB | `hw.pagesize` | kernel |

### The 20 GB limit is not a limit

The engine says so itself:

```
mlx_memory_limit_bytes     = 21474836480          (20 GB)
mlx_memory_limit_semantics = "scheduler_backpressure_not_allocation_cap"
```

It is **backpressure, not an allocation cap**. MLX allocates straight past it.
Every surface that renders this field as a limit — the Horus ops card, doctor,
menubar — states a guarantee the engine explicitly disclaims (Rule A35).

### What that cost, measured

| | gemma-4-12B-it **8bit** | gemma-4-12B-it **5bit** |
|---|---|---|
| `mlx_active_bytes` | 31.41 GB | **7.63 GB** |
| `mlx_peak_bytes` | **36.33 GB** | **10.36 GB** |
| % of 48 GB RAM | 76% | 22% |
| % of 37.4 GB GPU working set | **97%** | 28% |
| growth across 5 requests | — | **0.00 GB** (flat) |
| host free memory | 41% | **84%** |

The 8-bit configuration ran at **97% of the GPU's own recommended working set**
while a 20 GB "limit" was nominally in force. That is the mechanical cause of the
2026-08-06 lockup, not agent concurrency.

### RSS is blind here — the measurement trap

```
sne-server RSS        =  4.2 GB
mlx_active_bytes      = 31.4 GB
```

**A 27 GB blind spot.** MLX/Metal unified allocations do not appear in RSS. Every
RSS-based guard on this machine — `FindRunaway`, hapi, doctor memory findings, the
menubar hog card — is blind to the single largest consumer. During this very
investigation a "top RSS" reading of 4.9 GB was used to declare the host healthy
*while the broker held 31 GB*.

**Rule: memory guards read `mlx_active_bytes` from `/health`, never RSS.**

---

## 3. Where SNE actually runs today — GPU-only

Measured against the shipped binary (`sne-server-macos-arm64`):

```
otool -L   →  Metal, Accelerate, Foundation      ← CoreML NOT linked
nm -U      →  mlx_ = 1063   CoreML = 0   BNNS = 0   AMX = 0   ane_ = 3 (strings)
```

| Store | Used by SNE | Consequence |
|---|---|---|
| GPU shader cores | ✅ everything | saturated |
| GPU Neural Accelerators | ❌ | M5's headline matmul units idle |
| ANE | ❌ **unreachable — CoreML not linked** | ~entire unit idle |
| SME/SME2 | ❌ (Accelerate linked, 0 BNNS symbols) | documented matrix engine idle |
| CPU cores | control plane only | fine |

**SNE's founding premise — distribute across the chip — is unimplemented.** It is
a Metal-only engine. On Apple Silicon the ANE is reachable in practice only through
Core ML; with no CoreML linkage the Neural Engine cannot be used at all, by
construction, regardless of configuration.

This is the mechanical reason inference craters the host: **one store saturated,
three idle, one memory pool.**

---

## 4. Correcting the record on the ANE

The previous map (2026-07-21) was right and must not be over-corrected by
enthusiasm:

- **Token generation / decode belongs on the GPU.** Decode is memory-bandwidth-
  bound. The ANE gives no compute win for autoregressive decode of quantized
  weights (it dequantizes INT weights to fp16). Generation stays on the GPU.
- **The M5 leap is *in-GPU*.** M5's Neural Accelerators are matmul units inside
  each GPU core — *not* the ANE, and *not* the shader ALUs. They are reached
  through Metal 4 tensor operations, so they are the one new unit that benefits the
  existing MLX/Metal path rather than requiring a separate runtime. **[spec]** —
  Metal exposes `MTLGPUFamily` 1009/1008/5001 locally but no per-core neural
  accelerator counter is queryable, so core count is not locally verifiable.
- **The ANE's real win is fixed-shape encoder work** — embeddings and short-text
  classification. Apple's own DistilBERT case: ~10× faster, ~14× less peak memory,
  ~2 W vs ~20 W on GPU.

So "route everything to the ANE" is wrong. **"Route embeddings to the ANE, keep
decode on the GPU, and claim the GPU's neural accelerators via Metal 4"** is right.

---

## 5. The routing map — workload → store

| Workload | Store | Path | Why |
|---|---|---|---|
| LLM decode / generation | **GPU shaders + Neural Accelerators** | MLX → Metal 4 | bandwidth-bound; already correct |
| Prefill / prompt matmul | **GPU Neural Accelerators** | Metal 4 tensor ops | dense matmul, the exact shape they exist for |
| **Embeddings** | **ANE** | **Core ML** | fixed-shape encoder; 10× faster, 14× less memory, frees GPU |
| Short-text classification / triage screen | **ANE** | Core ML | same shape; Tier-0 screening (A30) |
| Similarity / ranking / dedup / vector math | **SME2** | Accelerate BLAS | int8/bf16 matrix on CPU; zero GPU contention |
| Tokenization, sampling, JSON, HTTP | **CPU P-cores** | goroutines | scalar, latency-sensitive |
| File scan / hashing / indexing | **CPU cores** | goroutines | I/O-bound; Metal dispatch tax (~350 µs) is a trap |
| Video/image transcode | Media engines | VideoToolbox | not currently a Sirsi workload |

**Contention rule:** embeddings and ranking are the workloads that today steal GPU
time from generation. Moving *those two* off the GPU is worth more than any
further MLX tuning, because it removes contention rather than redistributing it.

---

## 5a. Why decode *does* get faster — the queueing correction

An earlier draft of this map stated flatly that **"decode will not get faster."**
That claim was wrong as written, and it is wrong in the way this repo has a rule
about (A35): it was scoped to *peak decode on an unloaded, non-paging machine* but
stated as a universal.

**The corrected thesis.** Decode throughput on a real machine is not set by peak
memory bandwidth. It is set by **how much of the time the compute unit is actually
computing** versus stalled. The stall budget is real and measurable:

| Stall source | Present on this machine | Evidence |
|---|---|---|
| Paging / swap writes | yes | swap grew 628 MB → 3.04 GB under load |
| GPU working-set pressure | yes | 36.33 GB peak vs 37.4 GB recommended = 97% |
| Contention from co-resident work | yes | embeddings + ranking + decode all on GPU |
| Queueing behind unrelated batches | yes | `admission_queue: 32`, `continuous_batch_max: 4` |
| Host oversubscription | yes | load average 36 on 18 cores |

A unit at 97% of its working set with swap growing is **not bandwidth-limited — it
is stall-limited.** Bandwidth is the ceiling you hit *after* you stop stalling; we
are nowhere near it. So the correct statement is:

> **Peak** decode is bandwidth-bound. **Observed** decode is stall-bound.
> Eliminating stalls raises observed throughput toward the bandwidth ceiling — and
> that headroom is the win.

This reframes every routing decision in §5. Moving embeddings to the ANE is not
merely "polite to the GPU" — it removes a queueing source, and **a completed
operation delivered on time is worth more than a faster operation delivered
late.** Saturating each unit with the work it is good at, continuously, is how
utilization converts into delivered throughput.

**What is still true:** none of this raises the *bandwidth ceiling*. Anyone
claiming "N× faster decode" must state which regime they measured — a stall-bound
machine or a clean one — or the number means nothing (A14).

**How to measure it, before claiming it.** The stall fraction is the number that
settles this, and we do not have it yet:

```bash
# occupancy + stall accounting during a sustained decode run
xcrun xctrace record --template 'Metal System Trace' --launch -- <decode-bench>
# swap + page-in pressure across the same window
vm_stat 1 | awk '{print $1,$2}'     # pageins / pageouts / swapins / swapouts
sysctl -n vm.swapusage
```

Baseline first, then the routing changes, then the same run again. The delta is
the claim.

---

## 5b. Multi-node fabric — RDMA over Thunderbolt 5

The same stall logic extends across machines, but **only if the transport is used
for the right thing.** The bandwidth arithmetic is unforgiving:

| Link | Throughput | Ratio to local unified memory |
|---|---|---|
| Local unified memory | ~546 GB/s **[spec]** | 1× |
| Thunderbolt 5 | 80 Gb/s = **10 GB/s** (120 Gb/s boost = 15 GB/s) **[spec]** | **~2%** |

**Therefore a TB5 fabric cannot extend memory bandwidth, and any design that moves
weights across it will be slower than a single node.** That is the trap to name
explicitly before anyone builds it.

**What it *can* do is remove the stalls, which is the actual goal.** The design
rule follows directly from the ratio above:

> **Transport activations, never weights.**
> Weights are GB and static. Activations, logits, embeddings, and KV deltas are
> KB–MB and per-request. The first is catastrophic over a 10 GB/s link; the second
> is free.

Under that rule the fabric wins by **residency**, not by bandwidth:

1. **Every node's working set fits its own memory** → paging and SSD swap writes go
   to zero, which is the largest stall source measured above.
2. **Each unit stays saturated with work it is good at** — one node's ANE serving
   embeddings, another's GPU serving decode — rather than three workloads
   time-slicing one GPU.
3. **Results are published once and consumed immediately** by any node that needs
   them, instead of being recomputed behind a queue.

Effective throughput can therefore rise substantially *without any increase in
bandwidth* — the gain comes from reclaiming the stall budget. Whether that
approaches "double" is an empirical question, and it is answerable: measure the
stall fraction (§5a) on one node, then again with the workload split. **Publish
the ratio only alongside the absolute numbers and the regime** — a ratio without
the baseline is exactly the claim A14 exists to prevent.

**Open risk:** RDMA semantics over Thunderbolt/USB4 networking on macOS are not
verified here. `[spec]` throughout this section. Before any fabric work, the
first deliverable is a measured point-to-point latency and throughput test
between two Macs — not a design document.

---

## 6. Allocation governance — the missing layer

Today each consumer allocates against the shared pool with no arbiter. Required:

1. **A real cap, enforced outside the process it governs.** A limit inside the
   allocator it bounds is not a limit (§2, Rule A35). The budget authority is
   `recommendedMaxWorkingSetSize` (37.4 GB), not a hand-set 20 GB.
2. **Per-store budgets, not one global number.** GPU working set, ANE residency,
   and CPU heap are different stores with different ceilings; one number cannot
   govern three.
3. **KV sized to the qualified envelope.** `context_window = 262144` vs
   `qualified_prompt_tokens = 1024` — a **256× gap**. If KV is sized from the
   planned context rather than the tested envelope, that alone explains the 31 GB.
4. **Admission control keyed on measured headroom**, reading `mlx_active_bytes`
   and `recommendedMaxWorkingSetSize` — never RSS.

---

## 7. Application-level distribution (the other half)

The same principle applies above the inference layer: **work pinned to one master
thread cannot use 18 cores.**

Measured 2026-08-06 with only three router lanes live: **load average 36 on 18
cores** — 2× oversubscription — from 10 concurrent `go build`/`.test` processes.
Cause: Go defaults `GOMAXPROCS` and `go test -p` to `NCPU`, so *each* of two
workers fanned out to 18, giving ~36 runnable procs for 18 cores.

| Layer | Distributable | Mechanism |
|---|---|---|
| Go workers (sirsi, builds, tests) | ✅ | `GOMAXPROCS`, `go test -p/-parallel`, worker pool caps |
| Router dispatch | ✅ | max-concurrent-lanes + load-average backpressure |
| Native macOS surfaces (SwiftUI menubar) | ✅ | GCD QoS classes, `.utility` for background |
| **Electron apps (Claude, Codex, VS Code)** | ❌ | main thread is single-threaded by construction |

**The Electron limit is real and must be designed around, not fought.** Renderer
work cannot be spread across cores from outside the app. What *can* be controlled
is everything those apps spawn: agent workers, builds, test suites, indexers. Those
are ours, and they are where the oversubscription actually comes from.

**Spotlight belongs in this section.** `mds` was measured at 18.4% CPU indexing
directories that agents rewrite continuously — the router store (mutated every few
seconds), session transcripts, build caches, 22.8 GB of model blobs. The system
indexes machine state that no human will ever search, competing with the work that
produced the writes. After excluding `~/.sirsi`, `~/.claude`, `~/go`, `~/.cache`,
and `/private/tmp/claude-501`: **mds 18.4% → 0.0–0.8%**, load 33 → 8.46.

---

## 8. Verification commands

Every claim above is reproducible:

```bash
# compute stores
sysctl -n hw.ncpu hw.perflevel0.physicalcpu hw.perflevel1.physicalcpu
sysctl -a | grep -E "FEAT_SME|FEAT_I8MM|FEAT_BF16"
system_profiler SPDisplaysDataType | grep -E "Chipset|Cores|Metal"

# memory ceilings (Swift)
MTLCreateSystemDefaultDevice()!.recommendedMaxWorkingSetSize
MTLCreateSystemDefaultDevice()!.maxBufferLength

# what SNE actually links
otool -L ~/.sirsi/sne/current/sne-server-macos-arm64
nm -U    ~/.sirsi/sne/current/sne-server-macos-arm64 | grep -c CoreML

# live memory truth (never RSS)
curl -s http://127.0.0.1:8477/health | jq '.mlx_active_bytes, .mlx_peak_bytes, .mlx_memory_limit_semantics'
```

---

## 9. Open items this map creates

| Ledger task | What it blocks |
|---|---|
| `sne-is-gpu-only-ane-unused` | the entire premise of SNE |
| `mlx-limit-is-not-a-cap` | any credible memory guarantee |
| `rss-blind-to-metal-memory` | every existing memory guard |
| `context-window-262k-vs-1k-qualified` | likely the 31 GB directly |
| `capabilities-reports-wrong-model` | benchmark attribution correctness |
| `worker-build-parallelism-unbounded` | host usability under agent load |
| `wake-fabric-throttle-not-durable` | ability to stop the fabric at all |

**Refs:** PANTHEON_RULES A30 (Model Tiering), A32 (Load-Bearing Recognition),
A35 (Scope The Check To The Claim). Supersedes the 2026-07-21 acceleration map on
the AMX→SME point; upholds it on ANE-vs-GPU decode.
