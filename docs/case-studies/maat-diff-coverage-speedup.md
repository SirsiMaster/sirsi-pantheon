# 🪶 Case Study: Ma'at Diff-Based Coverage — 1,500x Speedup from Dogfooding

**Date:** 2026-03-23 (Session 12)
**Category:** Performance / Process Improvement
**Impact:** Pre-push gate: 55s → 56ms (coverage), 65s → 5s (total gate)
**Rule:** A14 (Statistics Integrity) — all numbers measured, not projected

---

## The Problem

During Session 12 launch execution, every `git push` triggered the Ma'at
pre-push hook, which ran `go test -cover ./...` across all 17+ packages.

**Measured wall time:** 55.3 seconds per push.

In a single session deploying v0.4.0-alpha with Homebrew tap, we pushed
4 times. That's **3 minutes 40 seconds** of waiting — enough to break
developer flow, check your phone, and lose context.

This is not a theoretical problem. It was experienced firsthand during
dogfooding. The insight: **a governance tool that destroys productivity
is worse than no governance at all.**

## The Insight

Ma'at was running `go test -cover ./...` — testing every package from
scratch — even when only 1-2 files changed. This is the equivalent of
rebuilding the entire house to check if a single window was installed
correctly.

The fix: **diff-based coverage.** Only test what changed.

## The Solution

### Architecture

```
git diff --name-only origin/HEAD
    ↓
Extract changed .go files → map to packages
    ↓
Run `go test -cover` ONLY on changed packages
    ↓
Merge fresh results with cached results
    ↓
Save updated cache to ~/.config/pantheon/maat/coverage-cache.json
```

### Key Design Decisions

1. **DiffOnly is the default.** Users get fast performance without
   configuration. Use `--full` only when you need a complete audit.

2. **Automatic cache management.** Every run (full or partial) updates
   the cache. No manual cache warming needed after the first run.

3. **Graceful degradation.** If the cache doesn't exist, Ma'at falls
   back to a full scan and creates the cache automatically.

4. **Exact same output.** Diff mode produces identical assessment
   results — same verdicts, same weights, same format. The speedup
   is invisible to the user except for the timing line.

## Results

| Metric | Before | After | Improvement |
|--------|--------|-------|-------------|
| `maat --coverage` | 55.3s | 36ms | **1,536x** |
| `maat --coverage --canon` | 55.3s | 56ms | **987x** |
| Pre-push gate (total) | ~65s | ~5s | **13x** |
| Pushes per session (avg) | 4 | 4 | — |
| Wait time per session | 4m 20s | 20s | **13x** |

### Benchmark Commands (Reproducible)

```bash
# Seed cache (one-time, 55s)
./sirsi maat --coverage --full

# Subsequent runs (diff mode)
time ./sirsi maat --coverage --canon
# Weighed in 56ms
```

## Broader Impact

This improvement was discovered through **dogfooding** — using the product
on itself before shipping to customers. The timeline:

1. **12:15 PM** — Started v0.4.0-alpha release execution
2. **12:45 PM** — After 4 pushes, noticed ~4 minutes of waiting
3. **12:50 PM** — Identified `go test -cover ./...` as the bottleneck
4. **1:00 PM** — Implemented diff-based coverage with caching
5. **1:05 PM** — Verified 1,500x speedup, pushed in 5 seconds

**Process improvement before v1.0 release.** Most teams discover
performance issues after users complain. Pantheon found and fixed this
issue during internal development — a direct benefit of the
build-in-public philosophy (ADR-003).

## Deity Performance Audit (Post-Fix)

| Command | Time | Category |
|---------|------|----------|
| `profile` | 6ms | ✅ Instant |
| `maat --diff` | 56ms | ✅ Instant (was 55s) |
| `guard` | 140ms | ✅ Fast |
| `ka` | 10.9s | ⚠️ I/O bound (17 filesystem locations) |
| `weigh` | 15.6s | ⚠️ I/O bound (58 rule scans) |
| `maat --full` | 55s | ⏱️ Expected (full test suite) |

Ka and Weigh are I/O-bound (filesystem traversal) — their performance
is proportional to disk state, not wasted computation. Different
optimization strategy needed (parallelism, skip-lists).

## Files Changed

- `internal/maat/coverage.go` — Diff-based coverage, git integration, JSON cache
- `cmd/pantheon/maat.go` — `--full` flag, DiffOnly default
- `.githooks/pre-push` — Tag-push skip, fresh binary build

---

*Measured on Apple M1 Max, macOS Tahoe, Go 1.26.1.*
*All numbers independently verifiable per Rule A14.*
