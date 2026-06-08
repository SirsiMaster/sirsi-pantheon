# sirsi-gemma — Developer README

A Model Context Protocol (MCP) server that exposes a locally-running
MLX-Gemma model to MCP-capable clients (Claude Code, Cursor, IDE plugins)
through two tools: `gemma_chat` and `gemma_complete`.

This file is the developer-facing companion to the user-facing guide at
[../../docs/user-guides/sirsi-gemma.md](../../docs/user-guides/sirsi-gemma.md).
Per Rule A8 both must exist.

## Architecture

```
MCP client (Claude Code)
   ↓ JSON-RPC 2.0 over stdio
internal/mcp.Server (reused — framing, dispatch, initialize handshake)
   ↓ ToolHandler
makeChatHandler / makeCompleteHandler   (main.go)
   ↓ Runner interface
MLXRunner ──exec.CommandContext──▶ python -m mlx_lm.generate ▶ stdout text
                                        (Gemma 2 27B 4-bit on Apple Silicon)
```

Files in this directory:

| File | Role |
| --- | --- |
| `main.go` | wires `internal/mcp.Server`, registers the two tools, runs startup health probe |
| `runner.go` | `Runner` interface + `MLXRunner` subprocess impl + `disabledRunner` fallback |
| `chat.go` | renders multi-turn chat history into Gemma's `<start_of_turn>` template |
| `config.go` | flat TOML loader for `~/.config/sirsi/gemma.toml` (zero external deps) |
| `runner_test.go` | unit tests using a fake `Runner` — no live MLX required |

## Why subprocess-per-call, not a long-running `mlx_lm.server`?

Two paths exist for driving MLX from Go:

1. **One-shot `mlx_lm.generate` per call** *(current)* — fork a Python
   process per request, stream stdout, exit. Simple, no shared state, easy
   to mock for tests, fits MCP's request/response shape cleanly.
2. **Long-running `mlx_lm.server`** — start once at boot, talk HTTP to it.
   Faster (no Python startup per call, model stays warm) and supports
   token streaming.

We picked (1) for the first cut because the model load cost dominates
either way (chip A's setup keeps the 27B 4-bit model warm in unified memory
via macOS file cache after the first call) and because (1) keeps the
`Runner` interface trivial to mock. The `Runner` interface is the
swap-point: future PR adds an `HTTPRunner` and selects via config without
touching the MCP layer.

## Rule A16 — Injectable subprocess

`Runner` is the injectable seam. `MLXRunner` is production; tests pass a
`fakeRunner`. No `exec.Command` lives in the handler layer, so tests don't
need a Python install. Coverage paths: success, generation error, missing
prompt, malformed message array, health-probe failure → `disabledRunner`.

## Streaming — current choice

The MCP `tools/call` response shape buffers a single `content` array, so
the first cut buffers `mlx_lm.generate` stdout and returns it whole. When
we move to `mlx_lm.server`, we'll emit MCP `notifications/progress`
during generation so Claude Code can render partial output.

## How to add a new tool

1. Define the `mcp.Tool` (name, description, input schema) in
   `registerGemmaTools`.
2. Write a handler closure that converts `args map[string]any` into a
   typed `Runner` call.
3. Add a unit test that drives the handler with a `fakeRunner`.

## Health probe

On startup, `selectRunner` calls `MLXRunner.Health(ctx)` — a 1-token
generation against the configured model. Probe failure does not kill the
server; we swap in `disabledRunner` so every subsequent tool call returns
the same actionable error pointing the operator at
[docs/setup/MLX_GEMMA_LOCAL.md](../../docs/setup/MLX_GEMMA_LOCAL.md). This
keeps the MCP handshake discoverable from Claude Code even when Gemma is
misconfigured.

## Build / test / lint

```
go build ./cmd/sirsi-gemma/
go test -race -count=1 ./cmd/sirsi-gemma/
golangci-lint run ./cmd/sirsi-gemma/...
```

Cross-compile sanity (Rule A3 — static binary):

```
GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build ./cmd/sirsi-gemma/
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./cmd/sirsi-gemma/
GOOS=darwin  GOARCH=arm64                go build ./cmd/sirsi-gemma/
```

The subprocess call will fail at runtime on non-mac, returning an
actionable error — that's expected and tested.
