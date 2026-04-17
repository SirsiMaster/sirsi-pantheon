# Thoth — Session Memory

Thoth gives your AI coding sessions persistent memory. Instead of re-explaining your project every time, Thoth syncs context automatically — cutting token waste by up to 98%.

## Commands

### Initialize in a project
```bash
sirsi thoth init                              # Interactive setup
sirsi thoth init --yes --name myproject       # Non-interactive
```

Creates `.thoth/` in your project root with:
- `memory.yaml` — Project metadata, dependencies, architecture summary
- `journal.md` — Session decisions and learnings
- `artifacts/` — Supporting files

### Sync memory
```bash
sirsi thoth sync                              # Full sync from source + git
sirsi thoth sync --since "48 hours ago"       # Sync recent changes only
sirsi thoth sync --dry-run                    # Preview what would change
```

Auto-detects: project type, language, dependencies, recent git history, and updates `memory.yaml`.

### Compact before context loss
```bash
sirsi thoth compact                           # Persist session decisions
sirsi thoth compact --summary "key decisions" # With explicit summary
```

Run this before your AI session's context window fills up. Saves session decisions to `journal.md` so the next session picks up where you left off.

### Neural weights (Pro)
```bash
sirsi thoth brain                # Check brain status
sirsi thoth brain --update       # Fetch latest weights
sirsi thoth brain --remove       # Clean up weights
```

Manages CoreML/ONNX weights for Pro-tier analysis features.

## IDE Integration

Thoth works as an MCP server for AI coding tools:

```bash
sirsi mcp                     # Start MCP server
```

Configure in your IDE:
```json
{
  "mcpServers": {
    "pantheon": {
      "command": "sirsi",
      "args": ["mcp"]
    }
  }
}
```

Compatible with: Claude Code, Cursor, Windsurf, VS Code + Continue.

## How It Works

1. `thoth init` scans your project and creates a knowledge base
2. `thoth sync` updates it from git history and source files
3. Your AI IDE reads `.thoth/memory.yaml` via MCP
4. The AI starts every session with full project context
5. `thoth compact` saves session learnings before context compression
