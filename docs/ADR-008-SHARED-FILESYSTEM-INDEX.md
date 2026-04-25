# ADR-008: Shared Filesystem Index — The All-Seeing Eye

**Status:** Accepted
**Date:** 2026-03-23
**Decision Makers:** Cylton Collymore, Thoth
**Deity:** 👁️ Horus (The All-Seeing Eye)

## Context

Pantheon has three deities that independently traverse the filesystem:

| Deity | Purpose | Time | Method |
|-------|---------|------|--------|
| **Jackal** (Weigh) | Find caches/artifacts | 15.6s | `filepath.Walk` × 81 rules |
| **Ka** | Find ghost apps | 10.9s | `filepath.Walk` × 17 locations |
| **Seba** | Infrastructure mapping | ~12s | `filepath.Walk` + external commands |

Total: **~38 seconds** of redundant filesystem traversal per full assessment.

All three walk overlapping directory trees (`/Library`, `~/Library`, `/Applications`,
`/usr/local`, etc.) independently. They're each building their own view of the same
filesystem — three separate cartographic expeditions to map the same territory.

## The Insight

**The filesystem is the shared resource. The index should be shared too.**

This is exactly how modern desktop search works:
- **macOS Spotlight** maintains a persistent index via `mds` daemon. `mdfind` returns
  results in milliseconds because it reads the index, not the disk.
- **Windows Everything** uses the NTFS USN Journal to maintain a real-time file index.
  Searches across 1M+ files return in <100ms.
- **Linux locate/mlocate** maintains an overnight-updated file database.

The pattern: **expensive traversal happens once; subsequent queries read the index.**

## Decision

### Architecture: Three-Phase Optimization

```
Phase 1: Walk Once, Share Many (IMMEDIATE — this commit)
─────────────────────────────────────
• Replace filepath.Walk with filepath.WalkDir (no stat per file)
• Parallel goroutine tree traversal
• Shared manifest cache: one walk feeds all deities
• Target: 15s → 2-4s

Phase 2: Change Detection (NEXT SPRINT)
─────────────────────────────────────
• macOS: FSEvents API — journal of changes since last scan
• Linux: fanotify (inotify successor) — kernel-level file event stream
• Windows: USN Journal / ReadDirectoryChangesW
• Docker: docker events + docker system df
• Target: 2-4s → 200-500ms (incremental)

Phase 3: Persistent Daemon (FUTURE)
─────────────────────────────────────
• Background service maintains live filesystem index
• Deities query the daemon, never touch disk
• Push-based: daemon notifies deities of relevant changes
• Target: <50ms for any deity query
```

### Phase 1 Design: The Manifest

```go
// ~/.config/pantheon/index/manifest.json
type Manifest struct {
    Version   string            `json:"version"`
    Timestamp time.Time         `json:"timestamp"`
    Platform  string            `json:"platform"`
    Entries   map[string]Entry  `json:"entries"` // path → metadata
    WalkStats WalkStats         `json:"walk_stats"`
}

type Entry struct {
    Path    string    `json:"path"`
    Size    int64     `json:"size"`
    ModTime time.Time `json:"mod_time"`
    IsDir   bool      `json:"is_dir"`
    Mode    uint32    `json:"mode"`
}
```

All deities read from the same manifest. The first deity to run populates
it; subsequent deities in the same session read from cache.

**Staleness policy:** Manifest is valid for 5 minutes (configurable).
After that, the next deity to run refreshes it. Users can force refresh
with `--fresh`.

### Parallel Walk Design

```
Standard filepath.Walk: Sequential, one directory at a time
    /Library → /Library/Caches → /Library/Caches/com.apple... → ...
    Total: 15.6s (I/O bound, single-threaded)

Parallel WalkDir: Fan-out at directory boundaries
    /Library ──┬── /Library/Caches ──── (goroutine 1)
               ├── /Library/Logs ────── (goroutine 2)
               ├── /Library/Application Support (goroutine 3)
               └── /Library/Frameworks (goroutine 4)
    Total: ~3-4s (limited by deepest subtree, not sum of all)
```

### Cross-Platform Strategy

| Platform | Phase 1 | Phase 2 | Phase 3 |
|----------|---------|---------|---------|
| **macOS** | WalkDir + goroutines | FSEvents via `CGO_ENABLED=0` Go wrapper | launchd daemon |
| **Linux** | WalkDir + goroutines | fanotify (Go stdlib) | systemd service |
| **Windows** | WalkDir + goroutines | USN Journal via syscall | Windows Service |
| **Docker** | `docker system df --format` | `docker events` stream | Container sidecar |
| **VMs** | SSH+WalkDir (scarab) | Guest agent push | Agent poll |

### Deity Roles in the Index

```
Horus (👁️) — Owns the index. Provides the query API.
    "I see all. Ask me, don't walk."

Jackal (Weigh) — Consumes index. Applies 58 classification rules.
    "Tell me what exists, I'll tell you what's waste."

Ka (Ghost) — Consumes index. Cross-references with app registry.
    "Tell me what exists, I'll tell you what's dead."

Seba (Map) — Consumes index + process list + network state.
    "Tell me what exists, I'll draw the constellation."

Ma'at (Governance) — Already optimized (diff-based coverage).
    "I only check what changed."

Guard (RAM) — Independent (reads /proc, ps). No filesystem walk needed.
    "I watch the living, not the filed."
```

## Consequences

### Positive
- All filesystem deities drop from 10-15s to <500ms on cached runs
- New deities automatically benefit from the shared index
- Platform-specific optimizations are centralized, not duplicated
- Dogfooding: our own governance (Ma'at pre-push) stays fast

### Negative
- Index staleness is a new failure mode (mitigated by 5-min TTL + --fresh)
- First run on a cold cache is still ~3-4s (parallel walk)
- FSEvents/fanotify (Phase 2) adds platform-specific code paths
- Daemon (Phase 3) increases deployment complexity

### Risks
- Index could grow large on systems with millions of files
  (mitigated: we only index known deity-relevant paths, not /)
- Race conditions between concurrent deity runs updating the manifest
  (mitigated: atomic file writes with temp+rename)

## References

- ADR-004: Ma'at QA/QC Governance (diff-based precedent)
- ADR-006: Self-Aware Resource Governance (yield module)
- ADR-007: Unified Findings Portal (Horus designation)
- macOS FSEvents Programming Guide (Apple Developer Documentation)
- Linux fanotify(7) man page
- Windows USN Journal Documentation (MSDN)
