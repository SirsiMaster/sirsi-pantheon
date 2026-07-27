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

## EL-002 — 2026-07-27 — The operational budget: who decides it, and what it buys

**Author:** claude-home (conduit) · **Status:** SPEC'D — implementation routed to claude-pantheon
**Trigger:** owner, following EL-001 — *"Why is Colima 10 GB? That's huge. I need Pantheon to decide
the most appropriate operational budget. Deterministic vs LLM based? Which local LLM? Which quantization?"*

### 1. Why Colima is 10 GB — it is not a node, it is a 16-container stack

```bash
colima ssh -- free -m          # total 9922 · used 6444 · available 3477
docker stats --no-stream       # 16 containers
```

| Container | RSS | Needed to anchor? |
|---|---|---|
| `network-node` (consensus, JVM, limit 8 GiB) | **2783 MiB** | **yes — this is the ledger** |
| `mirror-node-importer` | 567 MiB | only to read anchors back |
| `mirror-node-web3` | 537 MiB | no — EVM/smart-contract path |
| `mirror-node-monitor` | 500 MiB | no |
| `mirror-node-rest-java-internal` | 393 MiB | only to read anchors back |
| `mirror-node-grpc` | 377 MiB | no |
| `json-rpc-relay` + `-ws` | 418 MiB | no — Ethereum JSON-RPC compatibility |
| `mirror-node-rest-internal` | 108 MiB | only to read anchors back |
| `mirror-node-db` (Postgres) | 89 MiB | only to read anchors back |
| grafana + prometheus | 129 MiB | no — dev observability |
| explorer, api-proxy, relay-cache, haveged | 16 MiB | no |
| **Total** | **≈ 5.78 GiB** | |

**The consensus node is 2.78 GB — 47% of the VM. The other 53% is a block explorer, an Ethereum
JSON-RPC compatibility layer, a smart-contract execution service, a Postgres mirror and a Grafana
stack**, because `hedera-local start` launches the full local-development environment rather than a
node. The 10 GB was never sized against Sirsi's workload; it is the stack's default, and it fits.

So the honest answer to *"why 10 GB"* is: **nobody chose 10 GB for anchoring — it is what running the
whole developer stack costs.** Anchoring itself costs 2.78 GB, or ~3.5 GB if anchors are read back
through the REST mirror.

**Action (blocked on one measurement, not on judgement):** instrument which endpoints the anchoring
client actually calls. If it submits consensus messages via the gRPC SDK only, the target profile is
`network-node` alone → **VM at 4 GB**. If it verifies anchors through the mirror REST API, keep
`importer` + `db` + `rest` → **VM at 6 GB**. Either way ~2.2 GB comes back with nothing lost.
⚠ `hedera-local stop` DESTROYS the ledger — this is a compose-profile change, never a stop/start.

### 2. Deterministic vs LLM-based sizing — **deterministic, and this is not a close call**

The budget decision must never be made by the thing being budgeted. Five reasons, any one sufficient:

1. **Circularity.** On an 8 GB host the model cannot load, so it cannot be asked whether it fits.
   The tier that needs the decision most is the tier that cannot make it.
2. **Ordering.** Sizing happens at boot, before any model is resident. An LLM-based sizer would have
   to load a model to decide whether to load a model.
3. **Reproducibility.** The same host must yield the same plan every time. A budget that varies
   run-to-run is a heisenbug living in the OOM path — the least debuggable place in the system.
4. **Failure mode.** Arithmetic that is wrong is wrong the same way every time and gets fixed once.
   A hallucinated integer here kills the machine, non-reproducibly.
5. **Cost.** It is six measured inputs and two multiplications. Paying an LLM for it is absurd.

**The LLM's legitimate role is narration, not decision.** `sirsi diagnose` should print the derived
plan deterministically, and may use the local model to explain *in plain English* why a host got the
tier it got ("your 16 GB machine runs the 3B model on demand because the consensus VM holds 6 GB").
That respects the Plain-English-GUI rule without putting a probabilistic component in the OOM path.

**Boundary, stated as canon: sizing is arithmetic; explanation is language; they never swap jobs.**

### 3. Which local LLM — criteria first, because the fleet has no comparative measurement

**What we actually know:** `~/.sirsi/gemma-model-resolver.log` (2318 lines) records `selected:` on
every run and **no comparative benchmark of any kind**. Gemma was chosen by RAM arithmetic and one
behavioural property (it does not refuse), never by measured task accuracy against an alternative.
Any recommendation below that is not marked *measured* is a prior, not evidence.

**Criteria that matter for Pantheon's actual workload** — router triage, classification, structured
verdicts, summarization. Not long-form generation:

1. **Refusal rate on security-adjacent text.** Router items discuss credential rotation, kill
   decisions, exploits. A model that refuses is not a screen — it is a second queue. This is the
   property Gemma was picked for and it outranks benchmark scores.
2. **Structured-output reliability.** Triage returns a verdict enum. A model that emits prose around
   its JSON costs more in parsing failures than it saves in RAM.
3. **Quality per GB in the 2–8 GB class**, since that is the whole 8/16 GB tier.
4. **MLX availability.** No `mlx-community` conversion means no Apple-Silicon local path, which means
   no local sovereignty. This is a hard gate, not a preference.
5. **License.** Commercial product. Apache-2.0/MIT beats a bespoke community license.

**Recommendation, with confidence marked:**

| Tier | Model | Confidence |
|---|---|---|
| 8 GB (on demand) | **Qwen-class 3B-4bit** — 1.6 GB, already cached on this box, Apache-2.0, strong structured output at small size | medium — cached, never benchmarked here |
| 16 GB | **Qwen-class 7–8B-4bit** (~4.5 GB) or 3B resident | medium |
| 32–64 GB | **Gemma 12B, 4-bit QAT** (~7–8 GB) | high — the family is proven on this fleet |
| 128 GB | Gemma 31B 4-bit QAT (~30 GB) | measured: 0.5 tok/s on **this** box (router item `20260715-175752`) — a 128 GB claim, not a 48 GB one |

**GLM and Kimi K3: do not put them in the default path without a benchmark.** One structural reason
that does not depend on version specifics: **MoE architectures are the opposite of what a small tier
needs.** Their *active* parameter count is small, but every expert must be resident — you pay the
full parameter count in RAM and only save compute. For an 8 GB host that is the wrong trade in the
wrong direction. Dense small models are the right family for the small tiers, whoever trains them.
My information on the very newest releases may also be behind, which is itself an argument for the
harness in §5 rather than for my opinion.

### 4. Which quantization — **4-bit QAT as the default, everywhere**

| Quant | GB/B param | 12B model | Verdict |
|---|---|---|---|
| 8-bit | ~1.05 (disk) / **~1.6 measured RSS** | **19.30 GB peak** | only when RAM is genuinely free |
| **4-bit QAT** | ~0.55–0.6 | **~7–8 GB** | **default** |
| plain 4-bit | ~0.55 | ~7–8 GB | acceptable; QAT is better at equal size |
| 3-bit and below | ~0.45 | ~5.5 GB | quality cliff — no |

Two rules follow:

- **Prefer more parameters at 4-bit QAT over fewer parameters at 8-bit for the same RAM.** A 12B-4bit
  at ~7 GB beats a 4B-8bit at ~6.5 GB on every task Pantheon runs. QAT exists precisely to recover
  what aggressive quantization costs, which is why it wins between equal-bit variants and is never a
  reason to accept fewer bits.
- **8-bit is a large-host luxury.** On this 48 GB box it is what took the machine to the jetsam wall
  (EL-001 §3). The resolver preferred it because its ranking puts bits above size-realism — correct
  when RAM is free, catastrophic when it is not.

**Stated plainly: the current 12B-8bit default is costing ~12 GB of RAM to buy a quality difference
this fleet has never measured.**

### 5. The open action that outranks every opinion above — EL-002-A

Build a **triage benchmark harness**: 50 real closed router items with claude-home's recorded verdicts
as ground truth. Run each candidate (Gemma 12B-8bit, Gemma 12B-4bit-QAT, Qwen 3B-4bit, Qwen 7B-4bit)
over the same 50 and record **verdict agreement %, refusal count, malformed-output count, tok/s, and
peak RSS**. Ground truth already exists in the router store at zero labelling cost.

That single table replaces every "medium confidence" in §3 with a number, and it is what should decide
the default — not the resolver's ranking heuristics, and not this entry.

**Until it exists, the defensible position is: 4-bit QAT everywhere (RAM-justified, low risk), model
family unchanged (Gemma, behaviourally proven), and Qwen 3B as the 8 GB tier candidate to be
confirmed by the harness.**

### 6. Disposition

- Colima profile trim: **blocked on one measurement** (which endpoints the anchoring client calls),
  then a compose-profile change. Never a `hedera-local stop`.
- Deterministic sizing: canon, routed to claude-pantheon with EL-001's five rules.
- Model + quantization: **4-bit QAT is actionable now**; the family question stays open until EL-002-A.
- **Owner-gated:** moving this box off 12B-8bit changes fleet output quality.

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
