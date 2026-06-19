# ADR-031-A — Pantheon Must Never Exhaust the Host (Defense-in-Depth for Spawned Compute)

**Status:** Accepted (invariant binding immediately; layers shipping in sequence — gate #60 done, hard cap + cold-lock #63, **live self-governance / Hapi DONE** — `internal/guard/hapi.go` + `sirsi hapi` + broker self-registration, 10 tests incl. a real-process SIGSTOP/SIGCONT proof)
**Amends:** ADR-031 (Local Models Through Pantheon — the inference broker)
**Custodian:** 𓁢 Pantheon (runtime) · 🌊 Hapi (resource governance) · binding review claude-home
**Refs:** PANTHEON_RULES.md A1 (Safety) / A5 (Hapi) / A23 (Truth); `docs/case-studies/2026-06-18-pantheon-did-not-prevent-oom.md`; the 2026-06-19 owner directive *"ensure this never happens again."*

---

## Context

On 2026-06-18 Pantheon's own warm-inference broker OOM-crashed a 48 GB M5 Max: a concurrency-4 default with no runtime cap, plus a cold fallback path that spawned a fresh full-model load per concurrent caller, left **~5 model copies (~53 GB) resident at once** → macOS Jetsam → host freeze. The cleaner that exists to prevent resource exhaustion exhausted the machine. This is a thesis inversion, not a bug to patch.

A single pre-launch estimate (ADR-031's `gemmaSafeConcurrency`, PR #60) is necessary but **not** an invariant — it can be wrong, the KV cache can grow mid-decode, free RAM can drop after launch, and the cold path bypassed it entirely.

## Decision — the invariant

> **Pantheon MUST NEVER spawn a process that can exhaust the host — its own broker and its own cold CLI path included. Every Pantheon-spawned compute is born under a hard memory cap AND a live governor.** If the invariant can be violated by a wrong estimate, a config flag, or a future edit, it is not yet an invariant.

Enforced by **independent layers** (no single one is the only thing standing between us and Jetsam):

1. **Pre-launch RAM gate** (PR #60, hardened here): refuse-rather-than-OOM. Serial budget is **2×model** (resident weights + ~one model of working memory), not 1×. "Free" is computed net of the RSS of **live Claude/Codex agents** — cross-agent protection is the broker's job. *Layer 1 is an estimate; never the last layer.*
2. **Hard runtime cap** (this ADR): the MLX server launches through a wrapper that calls `mx.set_memory_limit` + `mx.set_wired_limit` + `mx.set_cache_limit(cap/4)` **before the model loads**, cap = free − headroom. MLX evicts/errors at the ceiling instead of growing into Jetsam — true even when the estimate is wrong.
3. **Cold-path lock + gate** (this ADR — the layer that actually caused 06-18): the cold `sirsi gemma` fallback acquires a machine-wide `flock` (one cold model load at a time) and passes the same RAM gate. Concurrent callers can never stack N full loads.
4. **Live self-governance — Hapi made real** (`internal/guard/hapi.go`, DONE): the broker registers its PID as **governed** at launch (`guard.HapiRegisterGoverned`, `cmd/sirsi/gemma_serve.go`). The `MemGovernor` samples free RAM + per-process RSS on a bounded tick (`vm_stat` + `ps`, mirroring the gate's "free") and intervenes **before the kernel**, escalating by consent tier (A1):
   - **WARN** (free < 15%): name the largest non-protected, non-agent process (`FindRunaway`) and recommend — for *any* process.
   - **SUSPEND** (free < 8%, governed only): `SIGSTOP` the largest governed runaway — halts its balloon, **reversible**; Hapi `SIGCONT`s it when pressure clears.
   - **KILL** (free < 4%, governed only): `SIGTERM` governed compute — the host comes first.
   It acts with teeth **only on processes that consented** (registered as governed); every other process is warned/recommended, never auto-killed. WindowServer, the kernel, audio, the session UI, sirsi itself, and live Claude/Codex agents are refused at the `hapiCanAct` gate (defense in depth atop `isProtectedReniceTarget`). Surfaced as `sirsi hapi` (status) / `sirsi hapi watch [--govern]` / `protect` / `release`. This is the "protect" pillar generalized beyond the broker — the gap that was open on 06-18 is now closed at the source: the broker can never again OOM the host because it consented to being stopped first.

**Regression guard:** `TestGemmaNeverAgainInvariants` asserts (a) default concurrency = 1, (b) the cap wrapper sets the MLX memory limit, (c) the 2×model serial budget refuses when it won't fit. The concurrency-4 default cannot silently re-enter.

## Consequences

- The broker stays **disabled on the owner's machine until layers 1–3 ship + are reviewed** (the installed binary still has the old code). It is **not** re-enabled by this ADR.
- "Use the whole machine" is honestly bounded: on 48 GB with a ~12 GB model, safe concurrency is **1** (warm model, no reload — that is the real win); higher concurrency needs a smaller model or more headroom.
- Layer 4 (Hapi) — the largest build and the one that makes the *protect* thesis real for **all** processes, not just gemma — is **shipped** (`internal/guard/hapi.go`, `cmd/sirsi/hapi.go`, broker self-registration). Remaining Hapi follow-ups (a launchd background daemon so it governs without a foreground `watch`, and a menubar pressure surface) are tracked in `docs/PANTHEON-FEATURE-ROADMAP.md`.
- Honest hardware note (carried from ADR-031): MLX is GPU-only (`Device(gpu,0)`), not the ANE.
