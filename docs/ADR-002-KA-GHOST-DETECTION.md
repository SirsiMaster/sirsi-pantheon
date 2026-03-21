# ADR-002: Ka Ghost Detection Algorithm

- **Status**: Accepted
- **Date**: 2026-03-20
- **Author**: SirsiMaster
- **Refs**: ANUBIS_RULES.md, ARCHITECTURE_DESIGN.md, ADR-001

## Context

After uninstalling macOS applications, significant filesystem remnants remain:
preferences, caches, containers, application scripts, saved state, launch agents,
and registrations in Launch Services (Spotlight). These "ghosts" consume disk space,
pollute Spotlight search results, and leave orphaned background processes.

No existing cleaning tool comprehensively detects and correlates these remnants
back to their source application. Anubis needs a dedicated module that goes beyond
simple path scanning to provide **cross-referenced ghost detection**.

## Decision

Implement the **Ka module** (`internal/ka/`) as a dedicated ghost detection engine
using a 5-step algorithm:

### Algorithm

1. **Build Installed App Index** — Scan `/Applications`, `~/Applications`, and
   Homebrew cask list to build a set of known bundle IDs and app names.

2. **Scan Residual Locations** — Check 17 macOS directories (12 user-level +
   5 system-level) for entries containing bundle IDs that DON'T match any
   installed app. These are orphaned residuals.

3. **Query Launch Services** — Use `lsregister -dump` to find apps registered
   in Launch Services whose `.app` bundles no longer exist on disk.

4. **Filter System Components** — Exclude all `com.apple.*` bundle IDs and
   known system services (Google Keystone, Microsoft AutoUpdate, etc.) that
   create preferences but aren't traditional user-installed apps.

5. **Merge into Ghost Structs** — Group all residuals by bundle ID into Ghost
   objects with total size, file count, residual breakdown, and Spotlight status.

### Residual Locations Scanned

| Location | Type | Requires Sudo |
|:---------|:-----|:-------------|
| ~/Library/Preferences | Preferences | No |
| ~/Library/Application Support | App Support | No |
| ~/Library/Caches | Caches | No |
| ~/Library/Containers | Containers | No |
| ~/Library/Group Containers | Group Containers | No |
| ~/Library/Saved Application State | Saved State | No |
| ~/Library/HTTPStorages | HTTP Storages | No |
| ~/Library/WebKit | WebKit Data | No |
| ~/Library/Cookies | Cookies | No |
| ~/Library/Application Scripts | App Scripts | No |
| ~/Library/Logs | Logs | No |
| ~/Library/Logs/DiagnosticReports | Crash Reports | No |
| /Library/Preferences | System Prefs | Yes |
| /Library/LaunchAgents | Launch Agents | Yes |
| /Library/LaunchDaemons | Launch Daemons | Yes |
| /var/db/receipts | Package Receipts | Yes |
| /Library/Application Support | System App Support | Yes |

### Data Model

```go
type Ghost struct {
    AppName          string
    BundleID         string
    Residuals        []Residual
    TotalSize        int64
    TotalFiles       int
    InLaunchServices bool
    DetectionMethod  string  // "filesystem" or "launch_services"
}
```

## Consequences

### Positive
- Detects remnants that other cleaning tools miss entirely
- Cross-references multiple data sources (filesystem + Launch Services)
- Groups residuals by source app for clear user presentation
- System component filtering prevents false positives on Apple services
- Distinguishes between "has files on disk" vs "only in Spotlight" ghosts

### Negative
- `lsregister -dump` is expensive (~2-5 seconds on machines with many apps)
- System-level scanning requires sudo access
- Bundle ID extraction is heuristic — some entries don't follow conventions
- `isInstalled()` uses string matching which may have false positives/negatives

### Future Work
- Launch Services rebuild command (`lsregister -kill -r`)
- Vendor grouping (e.g., all Adobe ghosts together)
- `--min-size` flag to filter small ghosts
- LaunchAgent/LaunchDaemon detection for zombie processes
- Trie/prefix-map optimization for `isInstalled()` performance
