# Contributing to Sirsi Pantheon

Thank you for considering contributing to Sirsi Pantheon! This guide will help you get started.

## Getting Started

### Prerequisites

- **Go 1.22+** installed
- **Git** with `SirsiMaster` account access
- Familiarity with the [PANTHEON_RULES.md](PANTHEON_RULES.md) operational directive

### Building

```bash
# Clone the repo
git clone https://github.com/SirsiMaster/sirsi-pantheon.git
cd sirsi-pantheon

# Build the CLI
go build -o sirsi ./cmd/sirsi/

# Build the agent
CGO_ENABLED=0 go build -o sirsi-agent ./cmd/sirsi-agent/

# Run tests
go test ./...

# Run linter
golangci-lint run ./...
```

### Project Structure

```
cmd/
  sirsi/           CLI entrypoint (weigh, judge, ka commands)
  sirsi-agent/     Lightweight fleet agent (placeholder)
internal/
  jackal/          Scan engine + rule interface
  jackal/rules/    81 built-in scan rules
  ka/              Ghost detection engine
  cleaner/         Safety module + deletion engine
  output/          Terminal UI (lipgloss theme)
  guard/           RAM management (Phase 1 TODO)
configs/           Default rule configurations
docs/              Architecture docs, ADRs, guides
```

## Adding a New Scan Rule

1. Create a new Go file in `internal/jackal/rules/`
2. Implement the `ScanRule` interface (see `internal/jackal/types.go`)
3. Register the rule in `internal/jackal/rules/registry.go` → `AllRules()`
4. Add at least one unit test (Rule A6)
5. Two rule types are available:
   - `baseScanRule` — for path-based scanning with glob expansion
   - `findRule` — for searching directories by name in project trees

### Example: New cache rule

```go
func NewMyAppCacheRule() jackal.ScanRule {
    return &baseScanRule{
        name:        "myapp_cache",
        displayName: "MyApp Cache",
        category:    jackal.CategoryGeneral,
        description: "MyApp temporary cache files",
        platforms:   []string{"darwin", "linux"},
        paths:       []string{"~/.cache/myapp"},
        minAgeDays:  7,
    }
}
```

## Safety Rules (PARAMOUNT)

Before contributing, understand these non-negotiable safety rules:

1. **Rule A1**: NEVER delete without `--dry-run` available
2. **Rule A2**: `Scan()` has ZERO side effects — read-only filesystem access
3. Protected paths in `internal/cleaner/safety.go` are **HARDCODED** and CANNOT be overridden
4. Every deletion passes through `ValidatePath()` — no exceptions

### Binary-write invariant (fresh inode — AMFI-safe)

**Never `os.WriteFile`/`cp` over a live executable's existing inode.** On macOS,
writing over an existing binary (`O_TRUNC`) leaves a stale code-signing cdhash
bound to the reused inode, so the next `exec` is SIGKILL'd (137) by AMFI — the
exact class that makes `sirsi` its own #1 crasher in `sirsi diagnose`, and which
has silently killed LaunchAgents/heartbeats. See
`reference_macos_amfi_cp_sigkill`.

Every code path that installs or replaces an executable MUST land it on a
**fresh inode**, one of:

- **`internal/selfupdate.SafeReplace(src, dst)`** — for CLI binaries in the
  allow-listed dirs (`~/.local/bin`, `~/go/bin`, `/opt/homebrew/bin`,
  `/usr/local/bin`). Staged `.new` → `codesign --force --sign -` → atomic
  `rename(2)`. It refuses `.app` paths by design (Rule A19).
- **`os.Remove(dst)` then `os.WriteFile(dst, …)` then `codesign --force
  --sign -`** — for paths `SafeReplace` won't take (e.g. a user-owned
  `~/Applications/*.app` bundle the installer creates). The `os.Remove` is what
  guarantees the new write gets a fresh inode.

A plain `WriteFile`/`cp` over an existing binary path is a **regression of the
#1-crasher class** and must be caught in review (cross-reference
`reference_macos_amfi_cp_sigkill` on any PR touching a binary-install path).

## Commit Protocol (Rule A7)

Every commit must follow the traceability protocol:

```
type(module): description

[optional body]

Refs: [canon docs, ADRs]
Changelog: [version entry]
```

**Types:** `feat`, `fix`, `docs`, `test`, `refactor`, `chore`
**Modules:** `jackal`, `ka`, `guard`, `core`, `ci`, `docs`, `agent`

## CI/CD (Rule A6)

Every push must pass:
1. `golangci-lint run ./...` — zero errors
2. `go test ./...` — zero failures
3. `go build ./cmd/sirsi/` and `go build ./cmd/sirsi-agent/` — must succeed

## Code Style

- `gofmt` is mandatory — no exceptions
- Table-driven tests
- Wrap errors with context: `fmt.Errorf("context: %w", err)`
- Go naming conventions (PascalCase exported, camelCase unexported)

## License

By contributing, you agree that your contributions will be licensed under the Apache License 2.0.
