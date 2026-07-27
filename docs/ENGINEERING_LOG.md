# 𓁢 Pantheon Engineering Log

> Continuous internal engineering log. Append-only, newest entry at the top. Distinct from
> [`BUILD_LOG.md`](BUILD_LOG.md), which is the public build-in-public narrative (ADR-003): this file
> is the internal record of measurements, root causes, and the arithmetic behind sizing decisions.
> Entries are never rewritten — a wrong finding gets a follow-up entry that supersedes it, so the
> reasoning stays auditable.
>
> **Entry format:** `## EL-NNN — YYYY-MM-DD — Title` · author agent · status
> (`OPEN` / `SPEC'D` / `IN FLIGHT` / `CLOSED` / `SUPERSEDED BY EL-NNN`).
> Every claim carries the command that produced it. No number goes in here that was not measured.

---

## EL-001 — 2026-07-27 — Pantheon cannot yet honestly claim to run on 8 GB or 16 GB hosts

**Author:** claude-home (conduit) · **Status:** SPEC'D — implementation routed to claude-pantheon
**Trigger:** owner question — *"exactly how much memory do I need to run this computer with Pantheon
so processes don't OOM"* — following `JetsamEvent-2026-07-27-104218.ips` on the 48 GB development box.

### 1. The finding in one line

**Pantheon's own core is ~80 MB. Everything that OOMs is a co-tenant Pantheon *chose* — and it chooses
those co-tenants using fixed constants and disk-size arithmetic instead of measured host capacity.**
That is why the 48 GB box died, and it is why an 8 GB box would die faster.

### 2. Measured footprint of Pantheon proper

```bash
ps -Axo rss=,command= | grep -i "[s]irsi"
```

| Component | RSS |
|---|---|
| `sirsi` router daemon (pid 1585) | 33.3 MB |
| `sirsi` triage daemon (pid 1567) | 16.7 MB |
| `sirsi` gemma-worker (pid 1603) | 8.5 MB |
| Sirsi menubar app (pid 52561) | 21.2 MB |
| **Pantheon core total** | **≈ 80 MB** |

Go binaries, no runtime, no VM. **Pantheon core fits in 8 GB with 99% of the machine to spare.** The
claim is defensible for the product; it is the *default configuration* that is not.

### 3. What actually consumed the machine

`JetsamEvent-2026-07-27-104218.ips`, page size 16384, decoded via `raw.split("\n",1)[1]`:

| Process | RSS at jetsam |
|---|---|
| Python (Gemma `gemma-4-12B-it-8bit`) | **19.30 GB** |
| `com.apple.Virtualization.VirtualMachine` (Colima → hypergraph consensus node) | **10.02 GB** |
| WindowServer | 0.92 GB |
| Claude Helper (Renderer) | 0.92 GB |
| WebKit.WebContent | 0.56 GB |
| Claude Helper | 0.44 GB |
| Chrome Helper (Renderer) | 0.40 GB |
| Codex (Service) | 0.38 GB |
| macOS wired (`vm_stat`, 324083 pages × 16 KB) | ≈ 5.07 GB |
| **Hard demand** | **≈ 38 GB of 48 GB** |

Plus a 1.7 GB compressor working set and file cache. This was not a leak. **It was a budget that was
never going to fit**, and the machine did the only thing it could.

### 4. Root cause A — the resolver sizes models on disk, not by measured residency

`~/.local/bin/sirsi-gemma-model-resolver.sh` selects a model with a `gb_per_b` table whose own comment
says *"safetensors on disk ≈ RAM at load"*. For `gemma-4-12B-it-8bit` that predicts `12 × 1.05 = 12.6 GB`.

Observed reality for the same model, same box:

```bash
ps -o rss=,etime= -p $(cat ~/.sirsi/gemma-server.pid)
# 13.56 GB at 15h43m uptime — and 19.30 GB at the jetsam peak
```

**The prediction is low by 1.1×–1.6×**, because disk size counts weights only: the KV cache,
activations, MLX allocator slack and framework overhead are all invisible to it. The resolver reserved
what it thought was safe headroom against a number that was up to 6.7 GB too small.

This is the second appearance of this exact class (`gemma-resolver-sizes-on-disk-not-rss`, first seen
in the 2026-07-26 jetsam forensics). It was fixed in the *forensics* path and left standing in the
*sizing* path.

**Corollary — the footprint is bimodal, and the resolver only ever models one mode.** Within a single
15-minute window this run, the same broker process measured **2.37 GB → 13.56 GB**: idle-with-weights-
evicted versus loaded. Jetsam happens at the loaded peak; the budget is computed against something
closer to the idle floor.

### 5. Root cause B — the budget floor inverts the safety rule on small hosts

The resolver computes `budget = max(8.0, total × 0.35)`. The 35% rule is sound. The `max(8.0, …)`
floor — added to stop a tiny budget selecting nothing — **overrides it in exactly the direction that
kills small machines**:

```bash
for t in 8 16 32 48 64 128; do python3 -c "print($t, max(8.0, round($t*0.35,1)))"; done
```

| Host RAM | Resolver default-budget | % of physical RAM |
|---|---|---|
| **8 GB** | 8.0 GB | **100%** |
| **16 GB** | 8.0 GB | **50%** |
| 32 GB | 11.2 GB | 35% |
| 48 GB | 16.8 GB | 35% |
| 64 GB | 22.4 GB | 35% |
| 128 GB | 44.8 GB | 35% |

**On an 8 GB host, Pantheon budgets 100% of physical RAM to model weights alone** — before the kernel's
~1.5 GB wired, before the desktop, before Pantheon itself. On 16 GB it budgets half the machine to a
number that is itself 1.5× optimistic, so the true ask is ~75%. Both configurations OOM on first load.
Neither has ever been run, which is why this has not been caught: the fleet has only ever run on ≥48 GB.

### 6. Root cause C — every other size in the stack is a hardcoded constant

| Constant | Current value | Set where | Scales with host? |
|---|---|---|---|
| `--prompt-cache-bytes` | 4 GiB (`4294967296`) | conduit supervisor task, verbatim in the restore command | **no** |
| `--decode-concurrency` / `--prompt-concurrency` | 2 / 2 | same | **no** |
| model RAM budget | `22320611328` (20.8 GB) | same | **no** |
| Colima VM `memory:` | 10 | `~/.colima/default/colima.yaml` | **no** |
| fleet reserve | 16 GB | resolver comments | **no** |

Every one of these was tuned by hand for *this* 48 GB box and would be transplanted unchanged onto an
8 GB one. Note the observed cache high-water mark is **2.19–2.20 GB across seven consecutive conduit
runs** — the 4 GiB reservation is nearly 2× what the workload has ever used, on every host size.

### 7. Answer to the owner's literal question (this box, current config)

- **64 GB** — honest minimum for the configuration as it stands today (12B-8bit + 10 GB VM + desktop).
- **128 GB** — required before `gemma-4-31B-it-qat-4bit` (27 GB on disk → ~30–34 GB resident) can be
  the default while the consensus VM and builds run.
- **48 GB (current)** — sufficient **only** with the resident set capped. Not sufficient as configured.

Config changes that reclaim ~11–14 GB with no hardware: 12B-8bit → a 4-bit 12B (−10 GB), prompt cache
4 GiB → 2 GiB (−2 GB), Colima 10 → 8 GB (−2 GB, **only after measuring the consensus node's JVM heap
inside the VM** — `hedera-local stop` destroys the ledger, so this one is measure-first).

But buying RAM or hand-trimming constants both leave the real defect in place: **the next model bump
silently re-creates this.**

### 8. The requirement: dynamic sizing, and an honest 8 GB claim

Owner directive, 2026-07-27: *"Pantheon has to work in 8 GB and 16 GB systems. If it can't, it's not
as useful as we claim."* Accepted as a product law, not a nice-to-have. Five rules follow.

**R1 — Budget against measured RSS, never disk size.** Maintain `~/.sirsi/model-footprint.json`:
`{model_id: {disk_gb, observed_peak_rss_gb, samples, last_seen}}`, written by the broker on every load
and read by the resolver. Until a model has a measurement, apply a **1.6× disk multiplier** — the
worst ratio observed here — rather than 1.0×. Learned numbers replace the guess on first real load.

**R2 — Delete the `max(8.0, …)` floor.** Replace with a subtractive budget:

```
available = physical − wired_floor(host) − co_tenant_reserve − desktop_reserve − pantheon_core(0.1 GB)
model_budget = available × 0.7
```

When `model_budget` is below the smallest known model's measured RSS, the correct output is
**"no resident model"** — not a floor that pretends one fits.

**R3 — On hosts < 24 GB, no model is resident by default.** Residency is a large-host optimization,
not a precondition. Small hosts get load-on-demand with eviction after an idle timeout, or a remote
endpoint. An 8 GB host should idle at ~80 MB of Pantheon and pay RAM only during an actual inference.

**R4 — Every constant in §6 becomes a function of host RAM.** Prompt cache scales as
`clamp(0.5 GB, available × 0.1, 4 GB)` — which yields 0.5 GB on an 8 GB host and ~2 GB here, matching
the seven-run observed high-water mark instead of doubling it. Concurrency, model budget and the VM
allocation take the same treatment. **`sirsi diagnose` should print the derived plan** so the sizing is
inspectable rather than implicit.

**R5 — Degrade, never die.** If nothing fits, Pantheon runs **LLM-free**: triage falls back to
deterministic rules, the router, board, threads, health and menubar are unaffected. This is the rule
that makes the 8 GB claim honest — at 8 GB the truthful statement is not *"Pantheon runs a 12B model"*,
it is *"Pantheon runs, and the LLM is opt-in and non-resident."*

### 9. Proposed tier table (to be validated on real hardware, not asserted)

| Host | Pantheon core | Local LLM default | VM | Honest claim |
|---|---|---|---|---|
| 8 GB | 80 MB | **none resident**; 3B-4bit (1.6 GB) on demand, evicted after idle | off | full Pantheon, opt-in LLM |
| 16 GB | 80 MB | 3B-4bit resident **or** 12B on demand + evict | off / 4 GB opt-in | full Pantheon, small local LLM |
| 32 GB | 80 MB | 12B-4bit (~8 GB measured) | 6 GB | comfortable |
| 48 GB | 80 MB | 12B-4bit, cache 2 GB | 8 GB | comfortable (this box, after fix) |
| 64 GB | 80 MB | 12B-8bit (~19 GB measured) | 10 GB | headroom |
| 128 GB | 80 MB | 31B-4bit (~30–34 GB) | 16 GB | full fleet |

**Not yet verified:** no 8 GB or 16 GB host has ever run this stack. Until one does, every row above
except 48 GB is arithmetic, not evidence. **The tier table is a hypothesis with a test attached, and
this entry does not close until an 8 GB run is on the record.**

### 10. Disposition

- Implementation is **claude-pantheon's lane** (resolver, broker, `sirsi diagnose` sizing plan) — routed,
  not absorbed.
- **Open owner decision:** whether the 48 GB box moves to a 4-bit 12B now. It changes fleet output
  quality, so it is not a conduit call.
- **Open engineering gap:** the Colima 10 GB allocation is untested downward; the consensus node's JVM
  heap must be measured **inside** the VM before it is touched.

---
