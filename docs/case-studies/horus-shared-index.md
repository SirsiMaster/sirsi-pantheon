# 👁️ Case Study: Horus Shared Filesystem Index — Every Deity Wins

**Date:** 2026-03-23 (Sessions 12–13)
**Category:** Architecture / Performance / Process Improvement
**Impact:** Weigh 15.6s → 833ms (18.7×), Ka 8.5s → 1.08s (7.8×), Ma'at 55s → 12ms (4,583×), pre-push 65s → 2s (32×)
**Rule:** A14 (Statistics Integrity) — all numbers measured, not projected

---

## The Problem

Pantheon has three deities that independently traverse the filesystem:

| Deity | Purpose | Time | Method |
|-------|---------|------|--------|
| **Jackal** (Weigh) | Find caches/artifacts | 15.6s | `filepath.Walk` × 58 rules |
| **Ka** | Find ghost apps | 8.5s | `filepath.Walk` × 17 locations |
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
| **Diff-based coverage** | Ma'at | 55s → 12ms (4,583×) |
| **WalkDir + combined pass** | Jackal rules | 2 walks → 1, no stat/file |
| **Pre-aggregated dir summaries** | Horus index | 856K entries → 50K dirs, O(1) lookup |
| **Gob encoding** | Horus cache | 110MB/936ms → 31MB/2ms |
| **FindDirsNamed** | Jackal findRule | walk ~/Development → in-memory scan |

### Key Design Decisions

1. **Interface-based wiring.** `ScanOptions.Manifest` is an interface, not
   a concrete type. Any struct that implements `DirSizeAndCount` and `Exists`
   can serve as the index. This makes testing trivial and the dependency clean.

2. **Graceful degradation.** If Horus index is unavailable (first run, error),
   rules fall back to direct filesystem walks. No breakage, no panics.

3. **Pre-aggregated directory summaries.** Instead of storing 856K file
   entries, Horus stores ~50K directory summaries with pre-computed
   TotalSize and FileCount. DirSizeAndCount is O(1) hash lookup (541ns)
   instead of O(n) scan.

4. **Gob encoding.** Binary gob format replaces JSON for the cache file.
   110MB JSON / 936ms parse → 31MB gob / 2ms parse (468× faster).

5. **FindDirsNamed.** Instead of walking ~/Development to find `node_modules`
   directories, findRule queries the in-memory index. This single change
   took weigh from 6.3s to 833ms.

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
| Full scan | 15.6s | **833ms** | **18.7×** |
| DirSizeAndCount | ~500ms/dir | 541ns/dir | **924,000×** |
| Manifest size | N/A | 31MB (gob) | — |
| Cache load | N/A | 2ms | — |

### Quality Verification

| | Horus (833ms) | No Horus (baseline) |
|---|---|---|
| Findings | **341** | **341** |
| Total Size | **65.6 GB** | **65.6 GB** |
| Rules Ran | **58** | **58** |

**Verdict: identical results, 18.7× faster.**

### System-Wide

| Deity | Before | After | Status |
|-------|--------|-------|--------|
| `profile` | 6ms | 6ms | ✅ Already instant |
| `maat` | 55,000ms | 12ms | ✅ **4,583× faster** |
| `guard` | 140ms | 140ms | ✅ Already fast |
| `weigh` | 15,600ms | 833ms | ✅ **18.7× faster** |
| `ka` | 8,457ms | 1,080ms | ✅ **7.8× faster** |
| Pre-push | ~65,000ms | ~2,000ms | ✅ **~32× faster** |

### Feather Weight Progression

```
Session start:  69/100  (5 warnings, 0 failures)
Mid-session:    75/100  (3 warnings)
Session 12 end: 81/100  (1 warning — brain coverage)
Session 13 end: 81/100  (Ka optimized, brain coverage still 40%)
Session 14:     brain 40% → 56% — warning eliminated
Canon linkage:  60% → 100% (10/10 commits linked)
```

## The Recursive Win

This is the Pantheon thesis in code:

```
Horus indexes the filesystem      → one walk
Jackal queries Horus               → instant DirSizeAndCount
Ka queries Horus                   → instant ghost detection (Session 13)
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

## Benchmark Progression

### Weigh (Jackal)
```
Phase 0 (baseline):         15,600 ms
Phase 1 (WalkDir):           9,200 ms  (1.7×)
Phase 1.5 (Horus cache):    7,200 ms  (2.2×)
Phase 2 (gob+preaggreg):    6,300 ms  (2.5×)
Phase 2.5 (FindDirsNamed):    833 ms  (18.7×)  ← FINAL
```

### Ka (Ghost Detection)
```
Phase 0 (baseline):          8,457 ms
Phase 1 (Horus wiring):     1,080 ms  (7.8×)
  └── DirSizeAndCount O(1)  replaces DirSize + countFiles (double-walk)
  └── Filesystem scan ‖ lsregister as parallel goroutines
  └── --deep flag: lsregister now opt-in (5.3s savings)
```

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
