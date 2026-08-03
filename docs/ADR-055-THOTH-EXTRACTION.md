# ADR-016: Extract Thoth as Standalone Package

**Status**: Accepted — Phase 1 Complete (May 5, 2026)
**Deciders**: Cylton Collymore
**Context**: Thoth memory system was reimplemented in both Go (sirsi-pantheon) and Node (sirsi-thoth). Competitor (siddsachar/Thoth) uses same name. Need single source of truth.

## Decision

**sirsi-thoth (Node)** is the canonical Thoth implementation. Pantheon consumes it.

### Phase 1: Standalone Package (DONE)

sirsi-thoth v2.0.0 published to GitHub with:
- `thoth-init` — project scaffolder (npx thoth-init)
- `thoth-sync` — memory.yaml + journal.md sync from git/filesystem
- `thoth-compact` — persist session decisions before context compression
- `thoth-mcp` — JSON-RPC 2.0 MCP server with 5 tools
- `lib/detect.js` — project detection, line counting
- `lib/sync.js` — ported from internal/thoth/sync.go
- `lib/compact.js` — ported from internal/thoth/compact.go
- `lib/mcp-server.js` — MCP protocol handler

Zero dependencies. Node 16+. MIT license.

### Phase 2: Pantheon Integration (TODO)

Replace Pantheon's Go Thoth reimplementation with thin wrappers that delegate to sirsi-thoth binaries.

**Files to modify in sirsi-pantheon:**

| File | Current | Target |
|------|---------|--------|
| `cmd/sirsi/thoth.go` | Calls `internal/thoth` Go package | Shells out to `thoth-sync`, `thoth-compact` |
| `internal/thoth/sync.go` | Go reimplementation (188 LOC) | Thin wrapper: `execFile('thoth-sync', ...)` |
| `internal/thoth/compact.go` | Go reimplementation (243 LOC) | Thin wrapper: `execFile('thoth-compact', ...)` |
| `internal/thoth/init.go` | Go reimplementation (420 LOC) | Thin wrapper: `execFile('thoth-init', ...)` |
| `internal/mcp/tools.go` | Go handlers for thoth_read_memory, thoth_sync | Delegate to thoth-mcp subprocess |

**Dependency strategy:**
- Option A: Require `npm install -g sirsi-thoth` as a prerequisite
- Option B: Bundle thoth-* binaries via `pkg` into Pantheon's release
- Option C: Embed Node runtime + sirsi-thoth in the Go binary (not recommended)
- **Recommended**: Option A for now. Add a `sirsi thoth install` command that runs `npm install -g sirsi-thoth` automatically.

**What stays in Go:**
- `internal/brain/` — Neural weight downloader, CoreML inference (Anubis Pro, NOT Thoth)
- `internal/mcp/server.go` — MCP server framework (serves non-Thoth tools: Vault, Horus, RTK)
- `internal/mcp/tools.go` — Non-Thoth tool handlers stay in Go

### Phase 3: npm Publish (TODO)

Publish `sirsi-thoth` to npm registry to:
1. Establish package name ownership
2. Enable `npx thoth-init` for any developer
3. Enable `npx thoth-mcp` as standalone MCP server config

## Consequences

- Single source of truth for Thoth logic (Node)
- Pantheon Go binary stays lean — Thoth logic is externalized
- npm package establishes trademark presence against competitor
- Node dependency for Thoth features (acceptable — Node is ubiquitous on dev machines)
- Go tests for internal/thoth/ become integration tests against the Node binary
