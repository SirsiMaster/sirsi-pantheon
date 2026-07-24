# ADR-048 — MLX Go Bindings: Verdict and Work Path

**Status:** Accepted (owner-directed analysis 2026-07-23)
**Context owners:** claude-home (analysis), claude-pantheon (execution), claude-nexus (substrate consumer)

## Question

We are a Go shop. The only Python left in production is the gemma inference
path (`gemma-capped-server.py` on `mlx-lm` 0.31.3 / `mlx` 0.31.2). Should we
build our own MLX Go bindings to eliminate it and gain efficiency?

## What the Python actually is

MLX is three layers:

1. **mlx/core (C++/Metal)** — all compute kernels. This is where 100% of
   inference FLOPs run. Language above this layer is irrelevant to kernel
   throughput.
2. **Bindings** — Python (nanobind, official), Swift (official), **mlx-c
   (official C API, built precisely for foreign-language bindings)**. A Go
   binding via cgo over mlx-c is technically routine: array ops, lazy graph
   build, eval, streams. Cgo call overhead (~100ns) amortizes to nothing
   under GPU kernel latency.
3. **mlx-lm (Python-only model runtime)** — the part we actually run:
   safetensors + group-quant loaders, per-architecture forward passes
   (gemma today, every future model tomorrow), HF tokenizers, Jinja chat
   templates, KV/prompt-cache management, sampling, batching. **There is no
   mlx-c equivalent of this layer.** MLX-Swift reimplements it per-model by
   hand; a Go port means the same treadmill.

## Verdict: full Go bindings for LLM serving — NO

- **Zero kernel-level perf gain.** Python orchestrates ~ms/token; Metal does
  the work. Same kernels either way.
- **Our real incidents were not Python's language.** Prompt-cache balloon =
  mlx-lm cache policy (now bounded via `--prompt-cache-bytes`); Jetsam = RSS
  policy (now capped); GPU kernel panics under churn = MLX/Metal driver
  level, language-agnostic (see reference: gpu-kernel-panic-mlx-churn).
- **The 80% is not bindings, it is a model runtime.** Tokenizers, Jinja
  templates, quant layouts, and a per-architecture port for every model we
  ever want. mlx-lm gets new models from the community in days; our Go port
  would gate every model upgrade on our own engineering.
- **Maintenance treadmill.** mlx-c historically lags mlx; we would chase two
  fast-moving APIs to stand still.

Sanctioned exception recorded: **MLX serving is the one legitimate Python in
the fleet** (Go Standard otherwise). Go owns supervision around it.

## What DOES make sense — three-step work path

### Step 1 (DONE): Go supervision hardening — claude-pantheon
`--prompt-cache-bytes` passthrough in `sirsi gemma serve` so the
governed Go path fully owns lifecycle, bounds, and restart of the Python
kernel. Python shrinks to a supervised compute kernel, like a GPU driver.

### Step 2 (re-homed to Win/Linux — see Amendment 2026-07-24): llama.cpp GGUF backend — claude-pantheon
llama.cpp is a single C++ binary, Metal-first, no Python runtime, gemma
supported, OpenAI-compatible `llama-server`. **On Apple Silicon it is NOT
adopted** (owner ruling 2026-07-24): our workload is prefill-dominated (8–32K
agent prompts + prompt-cache reuse) — MLX's home turf — so the ≥90%-of-mlx-lm
prompt-throughput adopt bar is the exact one llama.cpp is least likely to clear
on M-series, and a multi-hour 13 GB download to most-likely print "no-go" is not
worth the spend. The Python-serving fragility that motivated "delete Python from
serving" is already closed on the Mac (bounded cap wrapper + `--prompt-cache-bytes`,
KeepAlive-durable broker #286, self-healing #295), so swapping one external
binary for another is lateral and does not advance the substrate thesis.

llama.cpp is instead **re-homed as the cross-platform GGUF backend for the first
non-Apple Sirsi build (Windows/Linux), where there is no MLX.** Benchmark it
THERE, against that platform's own baseline — not against Apple Silicon. `sirsi
gemma serve` stays the abstraction: platform detection picks MLX on Apple,
llama.cpp GGUF elsewhere. Deferred until a non-Apple target exists.

### Step 3 (PRIMARY Apple-Silicon sovereignty play — see Amendment 2026-07-24): `mlx-go` embeddings binding — claude-pantheon, nexus consumer
With llama.cpp re-homed off Apple, this is the primary de-Python play on Apple
Silicon: Go-native, in-process, no HTTP hop. A NARROW cgo binding over mlx-c for a fixed embedding model only — no
model-architecture treadmill, small stable op surface (load, matmul,
norm, eval). Serves hypergraph/CTR-board semantic query in-process in Go
instead of shelling to Python. This is also strategic: an on-device Go
inference component is a substrate-thesis asset for Nexus (sovereign
inference on Sirsi silicon). Timebox: go/no-go spike, one week equivalent.
No-go criteria: mlx-c API instability, cgo/Metal interaction issues, or
<2x latency win over the HTTP hop to the Python broker.

## Platform backend matrix

`sirsi gemma serve` is the stable abstraction; the local-inference backend is
chosen by platform detection:

| Platform | Backend (today) | Backend (future) | Rationale |
|---|---|---|---|
| Apple Silicon | MLX (`mlx_lm.server`, supervised) | mlx-c cgo (Step 3, if the spike wins) | Prefill-dominated workload is MLX's home turf; Go-native cgo removes the HTTP hop and Python |
| Windows / Linux | — (no non-Apple target yet) | llama.cpp GGUF (`llama-server`) (Step 2) | No MLX off Apple; llama.cpp is the natural cross-platform local brain |

## Consequences

- On Apple Silicon, Python inventory converges to exactly one supervised kernel
  today (MLX), and to zero if Step 3's mlx-c cgo spike wins.
- llama.cpp is not dead — it is the cross-platform backend for the first
  non-Apple Sirsi build, benchmarked there against that platform's baseline.
- Step 3, if it survives its spike, seeds a `sirsi`-native inference
  capability that Nexus hardware (Cube/TPU roadmap) can later target.

## Amendment — 2026-07-24 (owner decision; claude-home concurred)

Owner ruling (verbatim intent): *"llama.cpp was not necessary for Apple
Silicon; stage it for Windows/Linux machines Sirsi builds."* Effect:

- **Step 2** re-homed from an Apple-Silicon benchmark to the cross-platform
  (Win/Linux) GGUF backend behind the `sirsi gemma serve` seam; deferred until a
  non-Apple target exists, benchmarked against THAT platform's baseline.
- **Step 3** (mlx-c cgo embeddings spike) elevated to the **primary** Apple-Silicon
  de-Python / sovereignty play. Accepted as scoped; its one-week timebox and
  no-go gates (mlx-c API instability, cgo/Metal issues, <2× win over the HTTP
  hop) are unchanged. **Implementation is scheduled for a dedicated session
  (owner) — this amendment records the decision, not the spike.**
- Added the platform backend matrix above.

Authority: pantheon lane (claude-pantheon holds the pen); claude-home concurred
as ADR-048 authority-path conduit (router item 20260724-151054). Refs: ADR-031
(local models through Pantheon), ADR-045 (durable broker), #286/#295.
