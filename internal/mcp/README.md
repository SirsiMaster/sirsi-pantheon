# 🔌 MCP Module — Model Context Protocol Server

**Package:** `internal/mcp`
**Protocol:** JSON-RPC 2.0 over stdio
**Spec:** [MCP 2025-03-26](https://modelcontextprotocol.io/specification/2025-03-26)
**Status:** Complete — shipping in v0.2.0-alpha

---

## Architecture

```
mcp/
├── server.go        — JSON-RPC 2.0 server, message routing, lifecycle
├── tools.go         — Tool implementations (scan, ghost, health, classify)
├── resources.go     — Resource providers (health, capabilities, brain)
└── server_test.go   — 14 tests covering all endpoints
```

### Protocol Flow

```
Client                          Server (anubis mcp)
  |                                |
  |── initialize ─────────────────>|
  |<────────── capabilities ───────|
  |── notifications/initialized ──>|
  |                                |
  |── tools/list ─────────────────>|
  |<────────── tool list ──────────|
  |                                |
  |── tools/call (scan_workspace) >|
  |<────────── scan results ───────|
  |                                |
  |── resources/read (health) ────>|
  |<────────── health JSON ────────|
```

## Tools

| Tool | Description | Parameters |
|:-----|:-----------|:-----------|
| `scan_workspace` | Scan directory for waste | `path` (optional), `category` (optional) |
| `ghost_report` | Hunt dead app remnants | `target` (optional filter) |
| `health_check` | System health summary | none |
| `classify_files` | Semantic file classification | `paths` (comma-separated, required) |

## Resources

| URI | Description | Format |
|:----|:-----------|:-------|
| `anubis://health-status` | System health + platform info | JSON |
| `anubis://capabilities` | Modules, commands, rules | JSON |
| `anubis://brain-status` | Neural brain install status | JSON |

## IDE Configuration

### Claude Code / Claude Desktop
```json
// ~/.claude/claude_desktop_config.json
{
  "mcpServers": {
    "anubis": {
      "command": "anubis",
      "args": ["mcp"]
    }
  }
}
```

### Cursor / Windsurf
```json
// .cursor/mcp.json
{
  "mcpServers": {
    "anubis": {
      "command": "anubis",
      "args": ["mcp"]
    }
  }
}
```

## Security

- **Rule A11**: No telemetry, no data transmission
- **Rule A4**: Network scanning requires explicit opt-in
- All scanning is local — no data leaves the machine
- MCP runs over stdio — no network ports opened
