# The Apple AI stack, explained plainly — and where Sirsi sits

Owner-requested explainer, 2026-08-03. Written to be readable by anyone;
the technical claims behind it live in docs/REPRODUCTION.md (A33: scoped,
humble, reproducible). Canonical source: sirsi-inference @ 823d1dc.

## The map

```
┌─ PRODUCTS ────────────────────────────────────────────────────────┐
│  Pantheon Anubis (open, free)   Pantheon Ra (commercial fleet)    │
│  ...any OpenAI-compatible app                                     │
└──────────────────────── published API contract ───────────────────┘
┌─ SERVING LAYER (the contested ground) ────────────────────────────┐
│  SNE (Sirsi, sealed)   |   oMLX (open competitor)  |  mlx_lm      │
│  batching + determinism|   MLX server w/ batching  |  (reference, │
│  + our Metal kernels   |                           |  single-user)│
└───────────────────────────────────────────────────────────────────┘
┌─ FRAMEWORKS ──────────────────────────────────────────────────────┐
│  MLX (Apple open source)  |  CoreML (app lane,     |  BNNS        │
│  arrays -> Metal GPU work |  sole key to the ANE)  |  (CPU math)  │
└───────────────────────────────────────────────────────────────────┘
┌─ GPU ACCESS ──────────────────────────────────────────────────────┐
│  Metal — Apple's GPU language (CUDA's equivalent)                 │
└───────────────────────────────────────────────────────────────────┘
┌─ APPLE SILICON ───────────────────────────────────────────────────┐
│  GPU (workhorse) | ANE (untapped for LLMs) | CPU+AMX | unified mem│
└───────────────────────────────────────────────────────────────────┘
```

Rendered version: docs/diagrams/apple-ai-stack-map.html.

## Each piece in plain words

**Metal** — Apple's language for talking to its GPU; the equivalent of
NVIDIA's CUDA. Anyone who wants GPU math writes "kernels" (small GPU
programs) in Metal or uses something that writes them for you. Sirsi's
custom attention kernels — including the deterministic-serving kernel —
are Metal code. The deepest layer we touch.

**MLX** — Apple's open-source math framework from its research group, on
top of Metal. You describe array math in a friendly form; MLX turns it
into Metal GPU work, batching operations into a graph before running
(lazy evaluation). Built around Apple silicon's superpower: unified
memory — CPU and GPU share one pool, so nothing is copied back and forth.
SNE builds directly on MLX's core (via its C interface, from Go),
replacing pieces our measurements showed losing with our own kernels.

**mlx_lm** — the reference "run an LLM on MLX" tool. Python, excellent for
one user at a time; the deployed-stock comparator for our certified
single-stream measurement. It is not a serving system — there is no
machinery for eight simultaneous users.

**oMLX** — the open-source community's LLM *server* on MLX, with batching.
Our nearest true competitor and the comparator for the ~1.43× equal-caps
concurrent measurement. Polished and fast-moving — one reason SNE stays
closed (ADR-051).

**CoreML** — Apple's "put a model in your app" system: compile a model
into a package and CoreML decides where it runs — CPU, GPU, or the ANE
(the Neural Engine, a dedicated AI chip that only CoreML can practically
reach). Superb for small in-app models; historically clumsy for large
chat-style models.

**BNNS** — Basic Neural Network Subroutines inside Accelerate: CPU-side
neural-net math that reaches the CPU's own matrix hardware (AMX).

CoreML and BNNS matter to Sirsi for one reason: they are the keys to
silicon the GPU path never touches.

## How the Sirsi Nexus ecosystem compares

The serving layer — many users, one machine — is the empty seat at
Apple's table. Apple builds frameworks, not multi-user serving; that
missing layer is what made NVIDIA's ecosystem (vLLM and friends)
dominant, and on Apple silicon it barely exists. SNE occupies that seat:
in our published, reproducible tests — faster than the reference
single-stream (deployed-stock scope), ~1.43× the nearest server at equal
concurrency limits, and the only engine we know of with provable
same-answer-under-load. We are not competing WITH Apple; we build what
Apple left unbuilt, on Apple's foundation, and report potential defects
upstream with reproductions as we go.

## The path from "ahead in our benchmarks" to actually competing

1. **Apple Stack Lab** (queued next engineering block): one harness that
   measures the same work across MLX / Metal / CoreML / BNNS with energy
   capture. Today we believe the GPU is right for everything; after the
   Lab we will know what belongs on the ANE and AMX — silicon nobody
   serving LLMs uses today. Untapped silicon is free performance.
2. **Speculative decoding + 5-bit** — the next speed and memory
   multipliers.
3. **Multi-Mac RDMA** (unblocked) — several Macs serving as one brain;
   the scale story no Apple-silicon competitor has.
4. **Distribution** — open Anubis puts the engine in enthusiasts' hands
   free; Ra sells fleet + determinism; the reproduction pack makes every
   claim checkable. Speed gets noticed; being checkable and everywhere
   makes a default.

## Pantheon's role in this picture

Pantheon Anubis (this repo) is the open product layer. It talks to SNE
(or any OpenAI-compatible engine) through the published API contract; it
never contains engine source. Pantheon Ra adds the commercial fleet
management and the Ra license grant; the `--profile interactive|fleet`
flag is the license seam (ADR-051). Anubis is the free distribution
channel that gets the engine in front of every Apple silicon enthusiast.

Refs: ADR-051, sirsi-inference/docs/adr/ADR-002, docs/REPRODUCTION.md
