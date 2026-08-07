# PRD — SNE Heterogeneous Compute & Allocation Governance (Unified)

**Owner:** claude-nexus (unified 2026-08-07, superseding the claude-home draft — see
§0) · **Status:** approved plan, execution in progress · **Date:** 2026-08-06 /
unified 2026-08-07
**Source of truth:** [`docs/APPLE_SILICON_COMPUTE_MAP.md`](../APPLE_SILICON_COMPUTE_MAP.md)
**Sibling repo:** `~/Development/sirsi-inference` (the SNE engine itself — owned
for implementation by codex-inference; this PRD governs both repos' work on
this one problem)

---

## 0. Why this document changed today

claude-home wrote the original version of this PRD on 2026-08-06 after
measuring the crash from the **sirsi-pantheon** side (the host, the guards,
the dispatcher). Independently, **codex-inference** had spent the same week
building against the identical problem from the **sirsi-inference** side (the
engine itself) — a 10-phase plan covering memory, KV persistence, multi-Mac
sharding, and a CoreML/ANE benchmark harness. Neither plan knew about the
other's *evidence*, only about each other's *status reports*. The owner's
read, on being shown both: **"in reality the two of you are working the same
job from different angles."** That's correct, and this document is the fix —
one plan, one sequence, one place requirements are declared done.

**Nothing here throws away either team's work.** Every codex-inference
artifact found in the audit (§4a) is either (a) already the requirement's
solution and just needs wiring/verification, or (b) a distinct, still-needed
piece of work that this plan sequences rather than duplicates. Where this PRD
says "do X," it says so *because* the audit confirmed X does not already
exist — never as a guess.

---

## 1. Problem

SNE's name promises a *Sirsi Neural Engine* that spreads work across Apple
Silicon. Measured, it is a **Metal-only engine**: `CoreML` is not linked into
the running server, `BNNS` and `AMX` symbol counts are zero, and `mlx_`
accounts for 1063 symbols. The ANE cannot be used at all by construction, and
SME2 — present and enabled on this chip — is untouched.

The consequence is not academic. On 2026-08-06 a single SNE process reached
**36.33 GB peak, 97% of the GPU's 37.4 GB recommended working set**, while a
nominal "20 GB limit" was in force that the engine itself documents as
`scheduler_backpressure_not_allocation_cap`. Host free memory fell to 41%,
load average hit 50, and the machine became unusable. **The local-model
broker is disabled right now, as of this writing, because of this exact
problem** — which means the fix for it currently cannot use the fix's own
intended builder (Gemma, Tier-0 local execution) until enough of it lands to
safely turn the broker back on. See §5 (Builder Assignment) for how this
bootstrapping constraint shapes the plan.

**One store saturated, three idle, one memory pool, no arbiter.**

### Why this is a product problem, not a tuning problem

A local model that makes the machine unusable while running is not a
feature. The value proposition of on-device inference is *sovereignty
without penalty*. Today the penalty is the whole machine — on a **48 GB M5
Max with the best unified-memory architecture Apple has shipped**, which
should make this the *easiest* machine in the world to run a 12B model
comfortably on. It is currently the opposite.

### The credibility stake

Sirsi publishes blog posts and investor materials claiming to have solved
local-model deployment (A33 — Humble Claims, Reproducible; the local-LLM
sovereignty thesis, `feedback_local_llm_sovereignty.md`). Right now, the
system those claims describe **locks up the machine it runs on**, and the
most recent investor-facing demo (`qubits-demo-repetition-incident`, §4a)
repeated one word until it hit the token limit. Every day this stays broken
is a day the public claim and the private reality diverge — and per A33,
that gap is the thing that costs trust when it surfaces, not the bug itself.

---

## 2. Impact — what changes, and for whom

| Before | After |
|---|---|
| Running the local model risks locking up the owner's only dev machine | The model runs inside a governed memory budget the OS itself agrees to, with a real off-switch |
| Health checks report "healthy" while 27 GB of GPU memory is invisible to them | Every guard reads the engine's own truth (`mlx_active_bytes`) |
| The ANE (16 cores) and SME2 (matrix-extension ISA) sit idle while the GPU saturates | Embeddings and ranking route off the GPU entirely; generation stays on GPU where it belongs |
| Two AI agent lanes (Claude/Codex) do the actual engineering at frontier-model token prices | Gemma (local, free) does the bulk of the well-specified implementation once the host is safe enough to run it (§5) |
| "We solved local inference" is a claim outrunning the evidence | Every throughput/memory claim ships with a repro script and an absolute-number baseline (A14) |

## 3. Consumers — who actually uses this

| Consumer | What they need from this work |
|---|---|
| **The owner**, on this one M5 Max Mac, today | Can run agents + IDE + the local model simultaneously without the machine locking |
| **Tier-0 triage** (A30 Model Tiering Doctrine) | The local model has to actually be *usable* for the whole tiering doctrine — cheap local screening — to mean anything |
| **gemma-the-builder** | Needs the broker to be safely re-enableable so it can do the Tier-0 build work this very plan assigns it |
| **Horus / doctor / menubar** | Need to report the *true* resource state, not a false "healthy" |
| **Investors and blog readers** | Are being told a claim this system needs to actually be true |
| **Future Sirsi products riding Tier-0 local inference** (SirsiNexusApp, FinalWishes, Assiduous — anything invoking `sirsi gemma`) | All inherit whatever reliability this plan delivers or fails to deliver |

## 4. How this serves Sirsi's actual goals

- **Local LLM Sovereignty** (`feedback_local_llm_sovereignty.md`) is a standing
  company thesis: when the cloud API is unavailable, local takes over. That
  thesis is void if local also takes down the machine.
- **A30 Model Tiering Doctrine** assumes Tier-0 (local, free) absorbs the bulk
  of low-stakes work. Right now Tier-0 is the least reliable tier on the
  machine — backwards from the doctrine's design.
- **A14/A33 (Statistics Integrity, Humble Claims)** require every public
  number to be reproducible and honestly scoped. This plan is partly *in
  service of* those rules — it can't ship a claim it can't reproduce.
- **Token economy** (the owner's explicit, urgent constraint below) — Sirsi
  cannot afford to have Claude or Codex hand-write the entirety of a
  multi-week systems-engineering effort at frontier-model prices. This plan's
  §5 exists specifically to make that unnecessary.

---

## 4a. What codex-inference has already built (audit, 2026-08-07)

Read-only source audit of `~/Development/sirsi-inference` at
`feat/inference-engine @ 9a9a0f9d`. This is the ground truth this plan is
built against — not codex-inference's own status report, but independent
verification of the actual code.

| Codex item | What it really is | State | Relationship to this PRD |
|---|---|---|---|
| `sne-27` Apple Stack Lab | GPU/CPU compute-lane benchmark harness + a **standalone Swift/CoreML binary** (`tools/coreml-stacklab/Runner.swift`) that builds an `.mlpackage` and inspects its compute plan | **MERGED**, but the CoreML path has **never executed a real prediction** (`executed: false` every run; compute plan prefers CPU, not ANE) | **This is most of R3's "week+" CoreML bridge cost, already paid.** R3 is now "wire this into live embedding serving and get it to actually predict," not "build a CoreML bridge from zero." |
| `sne-28` Speculative decode | n-gram drafter + verification, opt-in via `SIRSI_SPEC_K` env var, CLI-only | **MERGED**, not wired into the HTTP server, no production default | Not one of R1–R10. A real throughput lever, sequenced *after* the host is stable (§6, new Phase 5). |
| `sne-30` Publication-boundary policy | Governance doc: what's safe to publish externally about validated components | **MERGED, docs-only** | Not engineering work. No PRD overlap. |
| `sne-31` Multi-Mac sharding (JACCL / Thunderbolt-RDMA) | Design + partial code for 2-node pipeline-parallel inference over Apple's JACCL collective library | **Design merged; fleet-admission state machine merged (off by default); actual tensor transport "remains open"; JACCL itself never initialized/qualified — needs an operator-attended two-Mac session that has never happened.** Open bug: SNE's LM head is tied to the embedding matrix, conflicting with the no-replication rank split (unresolved). | **This IS R10.** Do not build a second multi-node design. R10's "measured link first, activations-never-weights" rule becomes the missing prerequisite gate on `sne-31-jaccl-qualification`. |
| `sne-43` Durable KV persistence | Save/restore immutable prefix-cache blocks across process restart | **Core save/restore code merged and tested; explicitly NOT wired into the server** (`server.go` has zero references to it) | Distinct from R2. R2 = size the KV cache correctly; sne-43 = persist it across restarts. **Sequence R2 before finishing sne-43** — no point persisting an oversized snapshot. |
| `sne-55` Stock comparator benchmark | Head-to-head decode tok/s vs stock `mlx_lm`, fail-closed, pinned versions | **MERGED and live**, real measured tables in `docs/BASELINE-COMPARISON.md` | **Reuse this harness for R9's stall baseline** rather than building new benchmark infrastructure. |
| `sne-broker-memory-balloon` | P0: MLX active memory climbed to ~3.2× the configured limit while **idle** | **Root-caused and source-repaired** (frees sampler/KV handles at idle; adds `TestServerIdleMemoryPlateau` gate) — **the repaired build has never been run; the broker is disabled and stays disabled until this is verified live** | **This is the literal unlock for re-enabling the broker.** It is a different layer from R1: this fixes a *leak*; R1 adds an *external cap* on top of a broker that no longer leaks. Verify this first. |
| `qubits-demo-repetition-incident` | Investor demo repeated one word to the token limit — wrong turn-token IDs used for termination | **Root-caused and source-repaired**, unverified live | Not one of R1–R10, but a credibility-critical fix that should be verified in the same live session as the memory-balloon fix (§6 Phase 0). |

**Net finding: two teams were closer to the same solution than either status
report showed.** The multi-Mac fabric (R10) is a live, partially-built
codex-inference project, not a future exploration. The ANE bridge (R3) is
80%-built and simply never turned on. The memory crash has *two* root causes
that were being treated as one: an idle-time **leak** (codex-inference,
source-repaired, unverified live) and a missing **external cap** on
legitimate-but-oversized allocation (claude-nexus, R1, not yet built). Both
are required; neither substitutes for the other.

---

## 5. Builder assignment — who builds what, and why (the owner's binding constraint)

> *"I cannot afford for either codex or claude to actually [build] this
> project because it is billions of tokens of cost which neither of you
> have."*

This is not a preference, it's a budget constraint this plan is designed
around, using the tiering already codified in **A30 (Model Tiering
Doctrine)**:

- **Tier 2 (Claude/Codex, frontier, expensive) — architecture judgment and
  safety-critical review ONLY.** Used for: the R1 external-cap design (memory
  safety), the sne-31 tied-embedding/LM-head resolution and JACCL go/no-go
  call (architecture decisions with no do-over), binding PR review/merge
  (A34 — a bind is always frontier), and this document itself.
- **Tier 1 (Claude/Codex, standard) — orchestration, not authorship.**
  Routing, ledger management, PR review of Tier-0 drafts, running the R6/R7
  host-safety fixes that had to land *before* Gemma could be trusted to run
  at all (already done — PRs #624, #625, #2, see §7).
- **Tier 0 (Gemma, local, free) — the actual implementation grunt work**,
  once §6 Phase 0 makes it safe to run the broker continuously:
  - R2 (KV sizing to the qualified envelope) — well-specified, mechanical.
  - R4 (SME2 linker flag + cgo BLAS) — well-specified, the PRD's own
    "cheapest, hours not days" item.
  - Wiring R3's *already-built* CoreML pipeline into live embedding serving
    — integration work against an existing, working standalone binary.
  - `sne-43-real-prefix-proof` / `sne-43-three-home-sync` — evidence-gathering
    against already-merged save/restore code.
  - `sne-31-pipeline`/`sne-31-contract` MLX tensor transport, **once** the
    tied-embedding architecture call is made by Tier 2.
  - Every regression test and repro script this plan requires (A14) —
    mechanical, well-specified, exactly gemma-the-builder's job per
    `feedback_gemma_builds_to_reduce_tokens.md`.

**The bootstrapping problem, named honestly:** Gemma cannot do any of this
until the broker can run continuously without threatening the host — and the
fix for that is itself part of this plan. That's why §6 Phase 0 is Tier-1/2
work (a small, one-time cost) whose entire purpose is to unlock Tier-0 for
everything after it. This is not a violation of the token-economy constraint
— it's the minimum frontier spend required to make the constraint
achievable at all.

---

## 6. Goals

| # | Goal | Measure of success |
|---|---|---|
| G1 | Inference never exceeds a governed share of the GPU working set | peak ≤ 60% of `recommendedMaxWorkingSetSize`, enforced externally |
| G2 | Embeddings and short-text classification leave the GPU | ANE-resident, GPU contention from these workloads → 0 |
| G3 | Ranking / similarity math leaves the GPU | SME2 path via Accelerate |
| G4 | Prefill exploits M5's in-GPU Neural Accelerators | measurable prefill improvement vs shader-only baseline |
| G5 | Memory guards read truth | every guard reads `mlx_active_bytes`, zero read RSS |
| G6 | Agent workload bounded | load average stays < core count under normal fabric operation |
| G7 | Host stays usable | interactive latency unaffected while inference runs |
| G8 | The broker can run continuously without owner intervention | zero manual quarantine/restart cycles across a 7-day window |
| G9 | Implementation cost stays inside the token budget | ≥ 70% of non-architectural changes land as Gemma-authored, Tier-1/2-reviewed diffs |

### Non-goals

- **Moving token generation to the ANE.** Decode is bandwidth-bound; the ANE
  offers no win and dequantizes INT weights to fp16. Generation stays on the
  GPU. This PRD explicitly rejects "route everything to the ANE."
- Replacing MLX. MLX is the correct GPU path; this extends around it.
- Windows/Linux parity (ADR-032 — Mac-first).
- Fixing Electron's single main thread. Out of our control (§9).
- **Re-designing sne-31's multi-Mac fabric from scratch.** It exists; this
  plan sequences and gates it, never replaces it.

---

## 7. Requirements

Each requirement below is marked with its **builder tier** (§5) and, where
relevant, its **codex-inference relationship** (§4a).

### R1 — External allocation cap *(G1, Tier 2 design → Tier 0 implementation)*

The cap must live **outside the process it governs**; a limit inside the
allocator it bounds is not a limit (Rule A35). This is layered *on top of*
codex-inference's already-repaired idle-leak fix (§4a
`sne-broker-memory-balloon`) — the leak fix stops the broker from wasting
memory it doesn't need; R1 stops it from being granted memory it does
"legitimately" request past budget.

- Budget authority is `MTLDevice.recommendedMaxWorkingSetSize` (37.4 GB
  here), never a hand-set constant.
- Default inference budget: **60% of recommended working set**, configurable.
- Admission control refuses a request that would exceed budget, with a typed
  error — never a silent OOM or a Jetsam.
- Surfaces must render the field's real semantics. `mlx_memory_limit_bytes`
  is displayed as **"backpressure threshold"**, never "limit".

**Acceptance:** a deliberate over-budget request is refused and named;
verified in both directions (passes under budget, fails over).

### R2 — KV sized to the qualified envelope *(G1, Tier 0)*

`context_window = 262144` against `qualified_prompt_tokens = 1024` is a
256× gap. Confirmed by the 2026-08-07 audit: KV allocation currently derives
from nothing but the theoretical max — `qualified_prompt_tokens`/
`sliding_window` are used for admission-gating only, never for sizing the
actual buffer. Must derive from the qualified/sliding envelope, with headroom
growth on demand.

**Sequencing note:** do this *before* finishing `sne-43`'s wiring — no point
persisting an oversized snapshot to disk.

**Acceptance:** idle-to-steady-state active memory for a 12B model stays
under 15 GB; per-request growth remains 0.00 GB across ≥ 50 requests.

### R3 — Embeddings on the ANE via Core ML *(G2, Tier 0 — cost already paid by sne-27)*

Wire the already-built, already-merged CoreML/Swift bridge
(`tools/coreml-stacklab/Runner.swift`) into live embedding serving. The
"week+ Swift bridge" cost this PRD originally budgeted is **mostly already
spent** — the remaining work is getting a real prediction to execute (today
it reports `executed: false` on every run) and routing live embedding
requests through it, separable from the MLX token path.

- Expected profile (Apple's DistilBERT reference): ~10× faster, ~14× less
  peak memory, ~2 W vs ~20 W.

**Acceptance:** embeddings served with GPU `mlx_active_bytes` unchanged
during embedding load; power and latency measured against the GPU baseline;
a repro script ships with the numbers (A14/A33).

### R4 — SME2 path for similarity / ranking *(G3, Tier 0)*

`FEAT_SME`, `FEAT_SME2`, `FEAT_I8MM`, `FEAT_BF16` are all enabled. Route
vector similarity, ranking, and dedup through Accelerate BLAS.

- Lowest cost / highest ratio of the four: a linker flag and cgo BLAS, hours
  not weeks. **First Gemma-buildable item once Phase 0 clears.**
- Supersedes the older "AMX" framing — SME2 is documented ARM ISA, not a
  private Apple extension.

**Acceptance:** ranking throughput vs the current path, measured; zero GPU
allocation attributable to ranking.

### R5 — Metal 4 tensor ops for prefill *(G4, Tier 0 draft → Tier 2 benchmark gate)*

M5's Neural Accelerators are matmul units **inside** GPU cores, reached via
Metal 4 tensor operations. Prefill is dense matmul — the exact shape they
exist for.

**Acceptance:** prefill latency vs shader-only baseline on identical
prompts, repro script included. **Risk:** per-core accelerator counts are
not locally queryable — benefit must be demonstrated by benchmark, not
asserted from spec.

### R6 — Guards read truth, not RSS *(G5, Tier 1)* — ✅ DONE

Every memory guard reads `mlx_active_bytes` from `/health`. RSS understated
the broker by **27 GB** and had already produced a false "host healthy"
verdict.

**Shipped:** [PR #625](https://github.com/SirsiMaster/sirsi-pantheon/pull/625)
— `internal/guard/brokerhealth.go` + call sites in `audit.go`, `hapi.go`,
`doctor.go`, `vitalscmd.go`, `internal/liveness/livenesswatch.go`.
Independently verified (build/vet/test, both-direction regression tests).

### R7 — Bounded agent workload *(G6, Tier 1)* — ✅ DONE

Worker builds capped (`GOMAXPROCS=4`), load-average backpressure gate, and a
fabric-wide quarantine marker the supervisors actually honour (generalizing
the gemma-only quarantine pattern that already existed).

**Shipped:** [PR #624](https://github.com/SirsiMaster/sirsi-pantheon/pull/624)
— `internal/router/{consumer.go,backpressure.go,fabricquarantine.go}`,
`sirsi router quarantine`/`unquarantine`. Independently verified.

### R8 — Index hygiene *(G7)* — ✅ DONE (2026-08-06, already applied)

Spotlight exclusions for `~/.sirsi`, `~/.claude`, `~/go`, `~/.cache`,
`/private/tmp/claude-501`. `mds` 18.4% → 0.0–0.8%.

### R9 — Stall baseline *(gates every throughput claim, Tier 0 using an existing harness)*

The thesis in the compute map's §5a — *observed decode is stall-bound, not
bandwidth-bound* — is stated but not yet measured. **Reuse `sne-55`'s
already-merged, fail-closed comparator harness** rather than building new
benchmark infrastructure; extend it to capture GPU occupancy/stall
accounting, `pageins`/`pageouts`, and `mlx_active_bytes` trajectory.

**Acceptance:** a committed baseline with a repro script. Every later claim
reports **absolute before/after plus the regime**, never a bare ratio (A14).

### R10 — Multi-node fabric *(exploratory → THIS IS `sne-31`, Tier 2 gate + Tier 0 execution)*

**This requirement and codex-inference's `sne-31` are the same project.**
The original framing ("link test only, exploratory") undersold how far
codex-inference had already gone: fleet-admission is merged, a CPU-only
pipeline/protocol proof is merged, and the actual blocker is that **JACCL
has never been qualified** — it needs an operator-attended two-Mac session
that has never happened, gated behind an unresolved architecture question
(the tied embedding/LM-head conflict with the no-replication rank split).

**Design rule, non-negotiable, unchanged: transport activations, never
weights.**

| Link | Throughput | vs local unified memory |
|---|---|---|
| Local unified memory | ~546 GB/s **[spec]** | 1× |
| Thunderbolt 5 | 10 GB/s (15 boost) **[spec]** | **~2%** |

**Unified next step:** (1) Tier 2 resolves the tied-embedding architecture
question — this is a real, irreversible design call and stays frontier work;
(2) the measured point-to-point TB5 link test (this PRD's original R10
deliverable) runs as the prerequisite check *before* the operator-attended
JACCL qualification session, so the qualification session isn't spent
discovering the link doesn't perform; (3) if the link test disappoints, JACCL
qualification is deferred without touching R1–R6.

**Acceptance:** measured link characteristics committed; the
activations-only rule encoded as a review check; the tied-embedding
architecture decision recorded with its rationale before any qualification
session is scheduled.

---

## 8. Sequencing (unified, with builder assignment)

Ordered by **value ÷ cost**, and by **what has to be true before Gemma can
build the rest** (§5's bootstrapping constraint).

| Phase | Work | Builder | Cost | Status |
|---|---|---|---|---|
| **0 — Unlock** | Verify `sne-broker-memory-balloon` fix live (safely, briefly) · verify `qubits-demo-repetition-incident` fix live · R6 guards read truth · R7 fabric caps · R8 index hygiene · R9 stall baseline (via sne-55 harness) | Tier 1 (verification), Tier 0 (baseline capture) | hours | **R6/R7/R8 done ([#625](https://github.com/SirsiMaster/sirsi-pantheon/pull/625), [#624](https://github.com/SirsiMaster/sirsi-pantheon/pull/624)); broker-balloon + qubits verification and R9 baseline still open** |
| **1 — Safe to run continuously** | R1 external cap (Tier 2 design, Tier 0 build) · R2 KV envelope (Tier 0) | Tier 2 → Tier 0 | days | not started |
| **2 — Cheap distribution** | R4 SME2 ranking | Tier 0 | hours–days | not started |
| **3 — The headline win** | R3 embeddings → ANE, wiring the existing CoreML bridge | Tier 0 | days (not week+, cost already paid) | not started |
| **4 — Benchmark-gated** | R5 Metal 4 prefill | Tier 0 draft, Tier 2 benchmark gate | days–weeks | not started |
| **5 — Multi-node** | R10/sne-31: tied-embedding decision (Tier 2) → TB5 link test → JACCL qualification | Tier 2 gate, Tier 0 execution | days–weeks | in progress on codex-inference's side; gate not yet applied |
| **6 — Throughput lever** | sne-28 speculative decode, wired into the HTTP server with a production default decision | Tier 0 wiring, Tier 2 default-on call | days | built, not activated |

Once Phase 0's live verification clears and the broker is judged safe to
re-enable, **every subsequent phase defaults to Tier 0 (Gemma) authorship**
unless the item is explicitly marked Tier 2 above. That default is the whole
point of unifying these two plans: it turns "billions of tokens of frontier
cost" into "hours of frontier judgment plus Gemma's free compute."

---

## 9. Constraints and honest limits

- **The bootstrap paradox is real and named, not hidden.** The fix for
  "Gemma can't run safely" is itself supposed to be partly built *by* Gemma.
  It can't be, for Phase 0. That's a one-time, bounded frontier cost — not a
  standing exception to the token-economy constraint.
- **Electron cannot be distributed.** Claude, Codex, and VS Code pin their
  main thread by construction. What we control is everything they *spawn*.
- **Core ML is the only ANE door**, and it's 80% built already (§4a) — the
  remaining cost is wiring, not bridging.
- **Per-core Neural Accelerator counts are not locally queryable.** R5's
  benefit is a hypothesis until benchmarked.
- **Decode throughput is stall-bound, not bandwidth-bound, in the regime we
  actually run in** — removing stalls raises delivered throughput even
  though the bandwidth ceiling doesn't move. State the regime with every
  claim (A14).
- **JACCL has never been qualified on this hardware.** Treat multi-node
  fabric numbers as unverified until the link test and qualification session
  both run.

---

## 10. Risks

| Risk | Mitigation |
|---|---|
| Gemma-authored diffs are lower quality than expected, eating the Tier-1/2 review budget anyway | Keep Tier-0 output scoped to well-specified, decomposed tasks (A30); escalate to Tier 1 on repeated failure, never absorb silently |
| Broker re-enable (Phase 0) re-triggers the balloon before the fix is confirmed | Verify live under a short, bounded, monitored window — not a blind re-enable |
| Two-repo plan drifts out of sync again | This document is the single source of truth for both repos on this problem; update it, don't fork it |
| JACCL qualification session reveals TB5 doesn't perform | R10 already scopes this: abandon without touching R1–R6 |
| "We use the whole chip" outruns the code | No public claim ships ahead of a repro script (A14, A33) |

---

## 11. Traceability

| Req | Ledger task (claude-nexus) | codex-inference evidence | Status |
|---|---|---|---|
| R1 | `mlx-limit-is-not-a-cap` | `sne-broker-memory-balloon` (leak fix, distinct layer) | open |
| R2 | `context-window-262k-vs-1k-qualified` | — | open |
| R3 | `sne-is-gpu-only-ane-unused` | `sne-27` (CoreML bridge, unwired) | open |
| R4 | `sne-is-gpu-only-ane-unused` | — | open |
| R5 | `sne-is-gpu-only-ane-unused` | — | open |
| R6 | `rss-blind-to-metal-memory` | — | ✅ done, PR #625 |
| R7 | `worker-build-parallelism-unbounded`, `wake-fabric-throttle-not-durable` | — | ✅ done, PR #624 |
| R8 | (spotlight exclusions) | — | ✅ done |
| R9 | (new) | `sne-55` (harness to reuse) | open |
| R10 | (new) | `sne-31` + sub-items (this IS the work) | in progress, unified |
| — | `capabilities-reports-wrong-model` | — | ✅ done, sirsi-inference PR #2 |
| — | (new) verify-live | `sne-broker-memory-balloon`, `qubits-demo-repetition-incident` | open, Phase 0 |

**Refs:** A14 (Statistics Integrity), A30 (Model Tiering), A32 (Load-Bearing
Recognition), A33 (Humble Claims), A35 (Scope The Check To The Claim),
`feedback_gemma_builds_to_reduce_tokens.md`, `feedback_local_llm_sovereignty.md`,
ADR-031-A/B (resource governance), ADR-032 (Mac-first).
