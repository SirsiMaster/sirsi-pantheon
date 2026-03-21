# Scan Rule Guide — Sirsi Anubis

This guide explains how to add new scan rules to Anubis.

## Architecture

Every scan rule implements the `ScanRule` interface in `internal/jackal/types.go`:

```go
type ScanRule interface {
    Name() string
    DisplayName() string
    Category() Category
    Description() string
    Platforms() []string
    Scan(ctx context.Context, opts ScanOptions) ([]Finding, error)
    Clean(ctx context.Context, findings []Finding, opts CleanOptions) (*CleanResult, error)
}
```

## Two Rule Types

### 1. `baseScanRule` — Path-Based Scanning

Best for scanning known directories (caches, logs, app data).

```go
func NewMyAppCacheRule() jackal.ScanRule {
    return &baseScanRule{
        name:        "myapp_cache",        // Unique identifier
        displayName: "MyApp Cache",        // Human-readable name
        category:    jackal.CategoryDev,   // One of: General, Dev, AI, VMs, IDEs, Cloud, Storage
        description: "MyApp temporary files and download cache",
        platforms:   []string{"darwin", "linux"},  // Which OSes to run on
        paths: []string{
            "~/.cache/myapp",              // Tilde-expanded automatically
            "~/Library/Caches/com.myapp",  // macOS-specific
        },
        excludes:   []string{"~/.cache/myapp/config"},  // Optional: never delete these
        minAgeDays: 7,                                   // Optional: only flag if older than N days
    }
}
```

### 2. `findRule` — Directory Name Search

Best for finding build artifacts scattered across project trees.

```go
func NewBuildOutputRule() jackal.ScanRule {
    return &findRule{
        name:        "build_outputs",
        displayName: "Build Outputs",
        category:    jackal.CategoryDev,
        description: "Build output directories in development projects",
        platforms:   []string{"darwin", "linux"},
        targetName:  "dist",              // Directory name to find
        searchPaths: []string{            // Root directories to search
            "~/Development",
            "~/code",
            "~/projects",
        },
        maxDepth:   4,                    // How deep to search
        minAgeDays: 7,                    // Age threshold
        matchFile:  "package.json",       // Optional: parent must contain this file
    }
}
```

## Registration

After creating your rule, register it in `internal/jackal/rules/registry.go`:

```go
// If macOS-only, add to darwinRules()
func darwinRules() []jackal.ScanRule {
    return []jackal.ScanRule{
        // ...existing rules...
        NewMyAppCacheRule(),  // ← Add here
    }
}

// If cross-platform, add to crossPlatformRules()
func crossPlatformRules() []jackal.ScanRule {
    return []jackal.ScanRule{
        // ...existing rules...
        NewMyAppCacheRule(),  // ← Add here
    }
}
```

## Categories

| Category | Constant | Description |
|:---------|:---------|:------------|
| General | `CategoryGeneral` | System caches, logs, downloads, trash |
| Dev | `CategoryDev` | Developer frameworks, build tools, package managers |
| AI/ML | `CategoryAI` | Model caches, training artifacts, GPU caches |
| VMs | `CategoryVMs` | Virtualization (Parallels, Docker, VMware, UTM) |
| IDEs | `CategoryIDEs` | IDE caches, workspace storage, language servers |
| Cloud | `CategoryCloud` | Cloud CLIs, Kubernetes, Terraform, Firebase |
| Storage | `CategoryStorage` | Cloud storage clients (OneDrive, Google Drive, iCloud) |

## Testing

Add tests for any custom logic in your rule. If using `baseScanRule` or `findRule`,
the base implementations are already tested — you only need to verify your paths
are correct.

## Safety Checklist

Before submitting a new rule:

- [ ] Paths never overlap with protected paths in `internal/cleaner/safety.go`
- [ ] `minAgeDays` is set appropriately (don't delete recent files)
- [ ] `excludes` protect any config files inside the cache directory
- [ ] Rule only targets **regenerable** data (caches, logs, build outputs)
- [ ] Rule NEVER targets source code, databases, or credentials
- [ ] Rule works correctly with `--dry-run`

## Example: Full Workflow

1. Create `internal/jackal/rules/myapp.go`
2. Write `NewMyAppCacheRule()` using `baseScanRule`
3. Add to `registry.go` → `darwinRules()` or `crossPlatformRules()`
4. Run `go build ./cmd/anubis/ && go test ./...`
5. Test locally: `./anubis weigh --dev`
6. Commit with: `feat(rules): add myapp_cache scan rule`
