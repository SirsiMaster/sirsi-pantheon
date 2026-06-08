# sirsi-gemma — User Guide

`sirsi-gemma` is a Model Context Protocol (MCP) server that exposes a
locally-running MLX-Gemma model to any MCP-capable client — Claude Code,
Cursor, IDE plugins — as two tools:

- **`gemma_chat`** — multi-turn chat with an optional system prompt.
- **`gemma_complete`** — single-shot text completion.

Everything runs on your machine. No tokens billed. No data leaves the
host (Rule A11).

## When to use it

Use sirsi-gemma when:

- Anthropic tokens are tight or unavailable (offline, rate-limited, etc.)
- You want a fast local pass over a chunk of code before paying for a
  larger Claude turn (rewrite, summarize, extract).
- You're iterating on prompts and don't want to spend cloud credits per
  iteration.

Honest about limits (Rule A23 — Truth Vector): Gemma 2 27B 4-bit is good
at code/refactor reasoning and short-context tasks. It is **weaker than
Claude on deep architectural calls, multi-file reasoning, and long
contexts**. Don't reach for sirsi-gemma when you need Claude's depth — use
it for the bounded, repeatable work where local speed wins.

## Prerequisites

- macOS (Apple Silicon) with the MLX runtime + Gemma 2 27B 4-bit
  installed per [docs/setup/MLX_GEMMA_LOCAL.md](../setup/MLX_GEMMA_LOCAL.md).
- The `sirsi-gemma` binary, built with `go build ./cmd/sirsi-gemma/`.

## Configure

Optional config file at `~/.config/sirsi/gemma.toml`:

```toml
# Hugging Face model id served from the venv.
model_id = "mlx-community/gemma-2-27b-it-4bit"

# Path to the Python venv that has `mlx_lm` installed.
venv_path = "~/.sirsi/mlx-venv"

# Generation defaults — clients can override per-call.
max_tokens  = 1024
temperature = 0.7
```

A missing config falls back to the defaults above, which match the
install layout chip A wrote in `MLX_GEMMA_LOCAL.md`.

## Wire it into Claude Code

See [docs/setup/MCP_CONFIG_SIRSI_GEMMA.md](../setup/MCP_CONFIG_SIRSI_GEMMA.md)
for the exact `~/.claude/mcp.json` snippet to paste.

## Verify it's working

After adding the snippet, in a fresh Claude Code session:

```
> Ask sirsi-gemma to summarize this README in one sentence.
```

Claude should call `gemma_chat` with the README contents and return
Gemma's summary. If sirsi-gemma is misconfigured, the tool call returns:

```
local MLX-Gemma not configured: ... — see ~/Development/sirsi-pantheon/docs/setup/MLX_GEMMA_LOCAL.md
```

That message means the binary started, the MCP handshake worked, but the
1-token health probe at startup failed. Fix the install, restart Claude
Code, retry.

You can also probe the binary directly:

```
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"manual","version":"0"}}}' \
  | sirsi-gemma --skip-health
```

You should see the server's `InitializeResult` come back on stdout.

## Known limits

- **Cold start** — first call after a reboot pays for model load (~6s
  for 27B 4-bit on M-series). Subsequent calls reuse the warm cache.
- **No streaming yet** — responses buffer to completion before the MCP
  client sees them. Streaming via `mlx_lm.server` is on the roadmap; see
  the developer README.
- **Non-Mac targets** — the binary cross-compiles for linux/amd64 and
  windows/amd64 so it can sit in a workspace, but the MLX subprocess will
  fail with a clear error on those targets. MLX is Apple Silicon only.
- **Architecture depth** — see "When to use it" above; Gemma is not a
  Claude substitute for deep cross-file reasoning.
