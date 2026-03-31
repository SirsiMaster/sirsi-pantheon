# ADR-013: Fold sirsi-thoth into sirsi-pantheon

**Status:** Accepted
**Date:** 2026-03-30
**Deciders:** Cylton Collymore

## Context

sirsi-thoth was a standalone Node.js CLI tool (`npx thoth-init`) that scaffolded the three-layer Thoth knowledge system (.thoth/memory.yaml, journal.md, artifacts/) into any project. Meanwhile, sirsi-pantheon already contained Go-native Thoth code for memory sync and journal generation (`internal/thoth/`).

Two separate repos solving halves of the same problem created maintenance overhead and confused the distribution story. Each Pantheon deity must be independently downloadable AND available as part of the full bundle.

## Decision

Fold sirsi-thoth into sirsi-pantheon as part of the existing Thoth deity module:

1. **Subtree merge** sirsi-thoth to preserve git history
2. **Port init logic** from Node.js (298 lines) to Go as `internal/thoth/init.go`
3. **Embed templates** via Go's `//go:embed` in `internal/thoth/templates/`
4. **Wire CLI** as both `thoth init` (standalone deity) and `pantheon thoth init` (bundled)
5. **Deprecate** the npm package and archive the sirsi-thoth GitHub repo

## Consequences

### Positive
- Single binary distribution for all platforms (Go cross-compiles)
- Thoth follows the established deity pattern: `cmd/thoth/` + `internal/thoth/`
- Templates ship embedded in the binary — no external files needed
- Users get `thoth init` + `thoth sync` in one tool

### Negative
- Node.js/npm users lose `npx thoth-init` (mitigated by Go binary being equally easy to install)
- 298 lines of JS became ~300 lines of Go — marginal effort, but a rewrite

### Neutral
- Existing `.thoth/` directories in other repos are untouched (they're outputs, not inputs)
- The `.thoth-template/` directory at repo root was consolidated into `internal/thoth/templates/`

## References
- Source repo: github.com/SirsiMaster/sirsi-thoth (to be archived)
- Specification: docs/THOTH_SPECIFICATION.md
