# MLX + Gemma 2 27B Local Install (Apple Silicon)

Foundation for the `sirsi-gemma` MCP server. Runs Gemma 2 27B (4-bit quant)
locally on Apple Silicon via [mlx-lm](https://github.com/ml-explore/mlx-lm).

## TL;DR for downstream consumers (chip B / sirsi-gemma MCP)

The MCP server should subprocess this exact invocation:

```bash
~/.venvs/mlx/bin/mlx_lm.generate \
  --model mlx-community/gemma-2-27b-it-bf16-4bit \
  --prompt "<prompt>" \
  --max-tokens <N> \
  --temp 0.0
```

Alive probe: `scripts/gemma-smoke.sh` (exits 0, prints `generation_tok_per_sec=...`).

## Environment

| Item | Value |
| --- | --- |
| Hardware | Apple M5 Max, 18 cores, 48 GB unified RAM |
| OS | macOS (Darwin 25.4.0) |
| Python | 3.12.4 |
| venv | `~/.venvs/mlx` |
| mlx-lm | 0.31.3 |

## Install

```bash
python3 -m venv ~/.venvs/mlx
source ~/.venvs/mlx/bin/activate
pip install --upgrade pip
pip install mlx-lm
```

`mlx-lm` brings `mlx` (Apple's array framework, Metal-backed) as a transitive
dependency. Apple Silicon only — there is no CUDA path.

## Model

| Field | Value |
| --- | --- |
| Model id | `mlx-community/gemma-2-27b-it-bf16-4bit` |
| On-disk size | 14 GB |
| Cache location | `~/.cache/huggingface/hub/models--mlx-community--gemma-2-27b-it-bf16-4bit/` |
| Quant | 4-bit, group_size=64 |
| Context | 8192 tokens |

First run downloads automatically. ~9 minutes on a typical home connection.

### Gotcha: avoid the original `gemma-2-27b-it-4bit`

The repo `mlx-community/gemma-2-27b-it-4bit` (no `bf16-` prefix) is an older
conversion that emits **only the pad token (id 0)** on mlx-lm 0.31.x — every
generation returns `<pad><pad><pad>...` regardless of prompt or chat template.
Use the `bf16-4bit` variant, which is a fresh re-quantization from the bf16
base and works correctly. The smoke script (`scripts/gemma-smoke.sh`) detects
the pad-spam failure mode and exits non-zero.

## Performance (measured, M5 Max / 48 GB)

Prompt:

> Explain in 5 sentences how a Go select statement differs from a switch
> statement, then write a function `merge(a, b chan int) chan int` that
> interleaves them until both close.

Reproduce:

```bash
source ~/.venvs/mlx/bin/activate
for i in 1 2 3; do
  /usr/bin/time -l mlx_lm.generate \
    --model mlx-community/gemma-2-27b-it-bf16-4bit \
    --prompt "$PROMPT" --max-tokens 400 --temp 0.0
done
```

| Run | Wall (s) | Prompt eval (tok/s) | Generation (tok/s) | Peak mem (GB) |
| --- | --- | --- | --- | --- |
| 1 (cold load, disk cache warm) | 14.41 | 142.5 | 33.77 | 15.55 |
| 2 (warm) | 13.59 | 235.9 | 33.64 | 15.55 |
| 3 (warm) | 13.81 | 232.8 | 33.38 | 15.55 |

- All runs generated 346 tokens (model finished naturally before max-tokens=400).
- "Cold load" = model loaded from disk cache into RAM; the very first run
  (with download) took ~531 s (download dominates).
- Per-invocation load overhead is ~2 s — the CLI re-loads the model on every
  call, which is why chip B's MCP server should keep a long-lived `mlx_lm`
  Python worker rather than subprocess'ing per request once latency matters.

## Memory headroom

With the model loaded, peak RSS is ~15.5 GB. On a 48 GB machine that leaves
~32 GB for the OS, browsers, and IDEs. `vm_stat` after a benchmark run
(model not resident) shows ~15.5 GB free + ~11 GB inactive; well above the
8 GB headroom requirement.

## Caveats

- **Apple Silicon only.** MLX uses Metal; there is no x86 or CUDA path.
- **CLI re-loads per call.** Each `mlx_lm.generate` invocation re-reads the
  model from disk (~2 s after the page cache is warm). Acceptable for the
  alive-probe; not acceptable for production traffic.
- **No HF token configured.** Downloads work unauthenticated but at lower
  rate limits. Set `HF_TOKEN` if pulling many models.
- **Chat template not required for raw completion.** For instruction-tuned
  use, pass `--use-default-chat-template` (the bf16-4bit variant supports it
  correctly).
