#!/usr/bin/env bash
# gemma-smoke.sh — "is the local MLX Gemma alive?" probe.
# Used by the sirsi-gemma MCP server tests to confirm the model is reachable
# and generating real tokens (not pad spam). Exits 0 on success, prints tok/sec.
#
# Requires: ~/.venvs/mlx with mlx-lm installed, and
#           mlx-community/gemma-2-27b-it-bf16-4bit downloaded to HF cache.
# See docs/setup/MLX_GEMMA_LOCAL.md for setup.

set -euo pipefail

VENV="${MLX_VENV:-$HOME/.venvs/mlx}"
MODEL="${MLX_GEMMA_MODEL:-mlx-community/gemma-2-27b-it-bf16-4bit}"
PROMPT="${MLX_SMOKE_PROMPT:-Reply with exactly one word: alive}"
MAX_TOKENS="${MLX_SMOKE_MAX_TOKENS:-16}"

if [[ ! -f "$VENV/bin/activate" ]]; then
  echo "ERROR: venv not found at $VENV" >&2
  echo "Create with: python3 -m venv $VENV && $VENV/bin/pip install mlx-lm" >&2
  exit 2
fi

# shellcheck disable=SC1091
source "$VENV/bin/activate"

OUT="$(mlx_lm.generate \
  --model "$MODEL" \
  --prompt "$PROMPT" \
  --max-tokens "$MAX_TOKENS" \
  --temp 0.0 2>&1)"

# Reject the broken-quant failure mode: only pad tokens emitted.
if echo "$OUT" | grep -q '<pad><pad><pad>'; then
  echo "ERROR: model emitted only pad tokens (broken quantization?)" >&2
  echo "$OUT" >&2
  exit 3
fi

# Extract reported tok/sec.
GEN_TPS="$(echo "$OUT" | awk -F'[, ]+' '/^Generation:/ {print $4}')"
PEAK_GB="$(echo "$OUT" | awk '/^Peak memory:/ {print $3}')"

if [[ -z "$GEN_TPS" ]]; then
  echo "ERROR: failed to parse generation tok/sec from mlx_lm output" >&2
  echo "$OUT" >&2
  exit 4
fi

echo "model=$MODEL generation_tok_per_sec=$GEN_TPS peak_memory_gb=$PEAK_GB"
