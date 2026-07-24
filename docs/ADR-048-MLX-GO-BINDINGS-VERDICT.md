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

### Step 1 (in flight): Go supervision hardening — claude-pantheon
Finish `--prompt-cache-bytes` passthrough in `sirsi gemma serve` so the
governed Go path fully owns lifecycle, bounds, and restart of the Python
kernel. Python shrinks to a supervised compute kernel, like a GPU driver.

### Step 2 (new): llama.cpp-Metal benchmark — claude-pantheon
llama.cpp is a single C++ binary, Metal-first, no Python runtime, gemma
supported, OpenAI-compatible `llama-server`. If it reaches parity on OUR
workload, we delete Python from serving without writing any bindings.

Benchmark on m5-sirsi, gemma-4-12B (8bit MLX vs Q8 GGUF), agent-shaped
workload: long prompts (8-32K), prompt-cache reuse ratio as in the router
conduit, decode-concurrency 2. Measure: prompt tok/s, decode tok/s, RSS
ceiling, prompt-cache hit behavior, cold-start. Decision rule: llama.cpp
adopted iff ≥90% of mlx-lm prompt throughput AND no RSS regression; MLX
prompt-processing advantage on Apple silicon is the thing to beat.

### Step 3 (new, scoped spike): `mlx-go` embeddings binding — claude-pantheon, nexus consumer
A NARROW cgo binding over mlx-c for a fixed embedding model only — no
model-architecture treadmill, small stable op surface (load, matmul,
norm, eval). Serves hypergraph/CTR-board semantic query in-process in Go
instead of shelling to Python. This is also strategic: an on-device Go
inference component is a substrate-thesis asset for Nexus (sovereign
inference on Sirsi silicon). Timebox: go/no-go spike, one week equivalent.
No-go criteria: mlx-c API instability, cgo/Metal interaction issues, or
<2x latency win over the HTTP hop to the Python broker.

## Consequences

- Python inventory converges to exactly one supervised kernel (or zero if
  Step 2 wins), with Go owning every operational surface.
- Step 3, if it survives its spike, seeds a `sirsi`-native inference
  capability that Nexus hardware (Cube/TPU roadmap) can later target.
