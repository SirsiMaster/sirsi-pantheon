# Contributing to Sirsi Anubis

Thank you for considering contributing to Sirsi Anubis! This guide will help you get started.

## Getting Started

### Prerequisites

- **Go 1.22+** installed
- **Git** with `SirsiMaster` account access
- Familiarity with the [ANUBIS_RULES.md](ANUBIS_RULES.md) operational directive

### Building

```bash
# Clone the repo
git clone https://github.com/SirsiMaster/sirsi-anubis.git
cd sirsi-anubis

# Build the CLI
go build -o anubis ./cmd/anubis/

# Build the agent
CGO_ENABLED=0 go build -o anubis-agent ./cmd/anubis-agent/

# Run tests
go test ./...

# Run linter
golangci-lint run ./...
```

### Project Structure

```
cmd/
  anubis/          CLI entrypoint (weigh, judge, ka commands)
  anubis-agent/    Lightweight fleet agent (placeholder)
internal/
  jackal/          Scan engine + rule interface
  jackal/rules/    34 built-in scan rules
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
3. `go build ./cmd/anubis/` and `go build ./cmd/anubis-agent/` — must succeed

## Code Style

- `gofmt` is mandatory — no exceptions
- Table-driven tests
- Wrap errors with context: `fmt.Errorf("context: %w", err)`
- Go naming conventions (PascalCase exported, camelCase unexported)

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
