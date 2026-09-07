# Wire sirsi-gemma into Claude Code (MCP config)

This document is a copy-pasteable MCP config snippet for enabling
sirsi-gemma in any Claude Code session via `~/.claude/mcp.json`.

## How to wire

1. Build the binary:

   ```
   cd ~/Development/sirsi-pantheon
   go build -o ~/.local/bin/sirsi-gemma ./cmd/sirsi-gemma/
   ```

2. Configure one local engine—MLX, SNE, SNE Native v2, or OMLX—using
   [Pantheon engine selection](../user-guides/PANTHEON-ENGINE-SELECTION.md).
   SNE, Native v2, and OMLX endpoints must be loopback URLs; this MCP server
   refuses to send a local prompt to a remote address.
   Native v2 also requires `sne_native_v2_host_profile = "m1"` or `"m5"`.
   Its successful responses include the endpoint, host profile, and configured
   model identity in a second JSON MCP content block; keep it with the result.

3. Add the snippet below to `~/.claude/mcp.json` (create the file if it
   doesn't exist; merge the `mcpServers` map if it does).

4. Restart Claude Code. Run `/mcp` to confirm `sirsi-gemma` shows up as
   `connected`, with two tools: `gemma_chat` and `gemma_complete`.

## Snippet

```json
{
  "mcpServers": {
    "sirsi-gemma": {
      "command": "/Users/YOU/.local/bin/sirsi-gemma",
      "args": [],
      "env": {}
    }
  }
}
```

Replace `/Users/YOU/` with your actual home directory. If you prefer to
keep the binary inside the repo, point `command` at
`~/Development/sirsi-pantheon/cmd/sirsi-gemma/sirsi-gemma` after building
in place — MCP requires an absolute path.

Optional flags:

- `"--config", "/custom/path/gemma.toml"` — override the config path.
- `"--skip-health"` — bypass the startup probe (debugging only).

Example with overrides:

```json
{
  "mcpServers": {
    "sirsi-gemma": {
      "command": "/Users/YOU/.local/bin/sirsi-gemma",
      "args": ["--config", "/Users/YOU/.config/sirsi/gemma.toml"],
      "env": {}
    }
  }
}
```

## Troubleshooting

If tool calls return `selected local inference engine unavailable: ...`, the
binary started but the health probe failed. Check the selected engine first:

- `~/.venvs/mlx/bin/mlx_lm.generate --model mlx-community/gemma-2-27b-it-bf16-4bit --prompt ping --max-tokens 1 --temp 0.0` runs from your venv (the `bf16-` prefix is required on mlx-lm 0.31+).
- `~/.config/sirsi/gemma.toml`'s `venv_path` matches the venv `mlx_lm`
  is installed in (chip A installs to `~/.venvs/mlx`).
- SNE, Native v2, and OMLX configurations point at their own
  `http://127.0.0.1:…/v1` or `http://[::1]:…/v1` endpoint.

For deeper diagnosis, run the binary by hand with stderr visible:

```
~/.local/bin/sirsi-gemma --skip-health < /dev/null
```

You should see `[sirsi-gemma]` log lines from the MCP handshake. Pipe a
real `initialize` request through stdin (see the user guide) to exercise
the full flow.
