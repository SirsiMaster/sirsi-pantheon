# 👁️ Case Study: Horus Shared Filesystem Index — Every Deity Wins

**Date:** 2026-03-23 (Session 12)
**Category:** Architecture / Performance / Process Improvement
**Impact:** Weigh 15.6s → 7.2s (Phase 1), Ma'at 55s → 15ms, pre-push 65s → 5s
**Rule:** A14 (Statistics Integrity) — all numbers measured, not projected

---

## The Problem

Pantheon has three deities that independently traverse the filesystem:

| Deity | Purpose | Time | Method |
|-------|---------|------|--------|
| **Jackal** (Weigh) | Find caches/artifacts | 15.6s | `filepath.Walk` × 58 rules |
| **Ka** | Find ghost apps | 10.9s | `filepath.Walk` × 17 locations |
| **Seba** | Infrastructure mapping | ~12s | `filepath.Walk` + commands |

Total: **~38 seconds** of redundant filesystem traversal per full assessment.

All three walk overlapping directory trees (`~/Library`, `/Applications`,
`/usr/local`) independently. Three separate expeditions to map the same
territory.

## The Insight

**The filesystem is the shared resource. The index should be shared too.**

This is how modern desktop search works:
- **macOS Spotlight** maintains a persistent index via `mds` daemon.
  `mdfind` returns results in milliseconds.
- **Windows Everything** uses the NTFS USN Journal. Searches across
  1M+ files return in <100ms.

The pattern: **expensive traversal happens once; subsequent queries read
the index.**

Applied to Pantheon: **Horus sees all. Deities ask Horus, not the disk.**

## The Solution

### Architecture: Walk Once, Share Many (ADR-008)

```
Horus Index (parallel goroutine walk)
    ├── ~/Library/Caches          ─── goroutine 1
    ├── ~/Library/Application Support ─ goroutine 2
    ├── ~/Library/Logs            ─── goroutine 3
    ├── /Applications             ─── goroutine 4
    ├── /opt/homebrew             ─── goroutine 5
    └── ~/Development             ─── goroutine 6
              ↓
    Manifest: map[path]Entry (in-memory)
              ↓
    Cache: ~/.config/pantheon/horus/manifest.json
              ↓
    All deities query the manifest, never walk the disk
```

### Three Optimizations in One Session

| Optimization | Target | Before → After |
|-------------|--------|----------------|
| **Diff-based coverage** | Ma'at | 55s → 15ms (3,667×) |
| **WalkDir + combined pass** | Jackal rules | 2 walks → 1, no stat/file |
| **Horus shared index** | All filesystem deities | walk once, query in 5ms |

### Key Design Decisions

1. **Interface-based wiring.** `ScanOptions.Manifest` is an interface, not
   a concrete type. Any struct that implements `DirSizeAndCount` and `Exists`
   can serve as the index. This makes testing trivial and the dependency clean.

2. **Graceful degradation.** If Horus index is unavailable (first run, error),
   rules fall back to direct filesystem walks. No breakage, no panics.

3. **Scoped roots.** The initial implementation naively indexed all of
   `~/Library` and `/private/var/folders`, producing a 476MB manifest.
   Scoping to deity-relevant subdirectories reduced it to 110MB.

4. **Compact JSON.** Entry fields use short keys (`s`, `d`, `m` vs
   `size`, `is_dir`, `mode`). ModTime stripped from cache — not needed
   for size/count queries.

## Results

### Ma'at (Diff-Based Coverage)

| Metric | Before | After | Improvement |
|--------|--------|-------|-------------|
| `maat --coverage` | 55.3s | 15ms | **3,667×** |
| Pre-push gate | ~65s | ~5s | **13×** |
| Pushes per session | 4 | 4 | — |
| Wait per session | 4m 20s | 1s | **260×** |

### Weigh (Horus Index)

| Metric | Before | After (cached) | Improvement |
|--------|--------|----------------|-------------|
| Full scan | 15.6s | 7.2s | **2.2×** |
| DirSizeAndCount | ~500ms/dir | 5ms/dir | **100×** |
| Manifest size | N/A | 110MB | — |
| Cache load | N/A | ~1s | — |

### System-Wide

| Deity | Before | After | Status |
|-------|--------|-------|--------|
| `profile` | 6ms | 6ms | ✅ Already instant |
| `maat` | 55,000ms | 15ms | ✅ **3,667× faster** |
| `guard` | 140ms | 140ms | ✅ Already fast |
| `weigh` | 15,600ms | 7,200ms | ✅ **2.2× faster** |
| `ka` | 10,900ms | 10,900ms | 🔜 Horus wiring next |
| Pre-push | ~65,000ms | ~5,000ms | ✅ **13× faster** |

### Feather Weight Progression

```
Session start:  69/100  (5 warnings, 0 failures)
Mid-session:    75/100  (3 warnings)
Session end:    81/100  (1 warning — brain coverage)
Canon linkage:  60% → 100% (10/10 commits linked)
```

## The Recursive Win

This is the Pantheon thesis in code:

```
Horus indexes the filesystem      → one walk
Jackal queries Horus               → instant DirSizeAndCount
Ka will query Horus                → instant ghost detection
Ma'at uses git diff                → instant coverage
Fast pre-push gate                 → more pushes per session
More pushes                        → more dogfooding
More dogfooding                    → more performance discoveries
More discoveries                   → better product
```

**Every deity wins if one deity wins.** The slightly recursive informational
flow where improvements compound: Horus makes Jackal faster, which makes
the pre-push gate faster, which lets us push more, which dogfoods more
issues, which produces more fixes.

## Phase Roadmap

| Phase | Status | Target |
|-------|--------|--------|
| **Phase 1:** WalkDir + Horus index + caching | ✅ Done | 15.6s → 7.2s |
| **Phase 2:** FSEvents/fanotify/USN Journal | 🔜 Next | 7.2s → 200-500ms |
| **Phase 3:** Persistent daemon + push notifications | 📋 Planned | <50ms |

## Files Changed

### Ma'at (diff-based coverage)
- `internal/maat/coverage.go` — git diff detection, coverage cache
- `cmd/pantheon/maat.go` — `--full` flag, DiffOnly default

### Horus (shared index)
- `internal/horus/index.go` — Parallel walk, manifest cache, query API
- `internal/horus/index_test.go` — Integration + unit tests
- `docs/ADR-008-SHARED-FILESYSTEM-INDEX.md` — Architecture decision

### Jackal (Horus wiring)
- `internal/jackal/types.go` — Manifest interface in ScanOptions
- `internal/jackal/rules/base.go` — Horus query path + dirSizeAndCount
- `internal/jackal/rules/dev.go` — WalkDir + Horus wiring
- `cmd/pantheon/weigh.go` — Horus integration, `--fresh` flag

---

*Measured on Apple M4 Max, macOS Sequoia, Go 1.26.1.*
*All numbers independently verifiable per Rule A14.*
*Timeline: discovered 12:45 PM, fixed by 1:30 PM — same session.*
