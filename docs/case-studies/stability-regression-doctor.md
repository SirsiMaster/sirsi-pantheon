# Case Study 016 — The Invisible Regression: Memory Bombs, Menubar Storms, and the Birth of Doctor

**Date:** April 1, 2026
**Session:** 40+
**Severity:** P0 — System Instability (kernel panics, Jetsam memory kills)
**Deities Involved:** Sekhmet (Guard), Hapi (Compute), Horus (Index), Thoth (Memory)
**Outcome:** 9 fixes, 1 new command, 24 new tests, 29/29 packages passing

---

## The Problem

The development machine — an M1 Max MacBook Pro with 32 GB RAM — was crashing repeatedly. Kernel panics during sleep/wake cycles. Forced reboots. Memory kills (Jetsam events) almost daily. Eight reboots in seven days.

The instinct was to blame macOS or hardware. But the timing was suspicious: the crashes started around March 25, 2026 — the same week Pantheon shipped its VS Code extension, menubar LaunchAgent, and Crashpad Monitor. Sixty-six commits in one week.

The question: **was Pantheon itself causing the crashes it was designed to prevent?**

## The Investigation

### Phase 1: Crash Forensics

A kernel panic log from the morning revealed an `AppleHIDTransport` sleep transition timeout — a macOS kernel bug unrelated to Pantheon. But that only explained 1 of 8 reboots.

The real signal was in the Jetsam logs: **6 memory-kill events in 7 days.** The system was exhausting 32 GB of RAM regularly, and macOS was killing processes to survive.

### Phase 2: Deep Regression Analysis

A full static analysis of every `.go` file under `internal/` and `cmd/` — 176+ files across 27 packages — revealed:

**Finding 1: The `ioreg` Bomb (`platform/compute.go:149`)**
The ANE detection code was calling `ioreg -l -w0`, which dumps the *entire* macOS I/O Registry. On this machine: **8 MB per call.** The output was captured into a `[]byte`, then copied to a `string` — doubling the allocation to 16 MB of transient memory. If `DetectCompute()` was called multiple times per session (via MCP health checks, CLI commands, or the menubar), Go's lazy GC would hold onto hundreds of MB that the OS still counted as "in use."

**The fix:** Replace with `sysctl -n hw.optional.ane` — returns `1` or `0`. ~10 bytes instead of 8 MB.

**Finding 2: The Menubar Storm (`cmd/pantheon-menubar/stats.go`)**
The menubar was polling system stats every **10 seconds**, spawning 7 subprocesses per cycle: `sysctl`, `memory_pressure`, `vm_stat`, `git rev-parse`, `git status`, `git log`, `ps`. That's:
- 42 process spawns per minute
- 2,520 per hour
- **60,480 per day**

The `memory_pressure` command itself is expensive — it probes kernel memory state and can take 200-500ms.

**The fix:** Polling interval 10s → 60s (6x reduction). Dropped `memory_pressure` entirely, using lightweight `vm_stat` only.

**Finding 3: `lsregister -dump` Memory Buffer (`ka/darwin.go`, `sight/launchservices.go`)**
Both Ka (ghost detection) and Sight (Launch Services hunter) called `cmd.Output()` on `lsregister -dump`, which produces 20-50 MB of output. This was buffered entirely in memory before parsing.

**The fix:** Stream via `cmd.StdoutPipe()` + `bufio.Scanner`. Line-by-line processing, never holding the full dump.

**Finding 4: Nil File Descriptor Panic (`thoth/sync.go:108,155`)**
Two instances of `f, _ := os.Open(path)` followed by `defer f.Close()`. If the file can't be opened (permissions, broken symlink), `f` is nil, and `f.Close()` panics. A latent crash waiting to happen.

**The fix:** Proper error check before defer.

**Finding 5: Horus Manifest Never Released (`mcp/tools.go`)**
The MCP server loaded the Horus filesystem manifest but never called `Release()`, leaving the entire directory tree in heap memory.

**The fix:** `defer m.Release()` after load.

**Finding 6: Dead PIDs in Throttler Map (`guard/throttle.go`)**
The Sekhmet throttler tracked reniced PIDs but never cleaned up entries for processes that died.

**The fix:** New `Prune()` method that checks liveness via `kill -0`.

### Phase 3: What Was Clean

Not everything was broken. The regression analysis confirmed:
- **All 29 goroutines** properly managed (context cancellation, WaitGroup cleanup)
- **Watchdog self-throttle** working correctly (backs off if consuming >5% CPU)
- **AlertRing** bounded at 50 entries (no unbounded growth)
- **`go vet`** clean across all packages

## The Birth of Doctor

The investigation revealed a gap: Pantheon could monitor resources continuously (Guard watchdog) and enforce policies (Scales), but there was no **one-shot diagnostic** — a single command that checks everything and gives you a health score.

`pantheon doctor` was born as a Sekhmet subcommand:

```
$ pantheon doctor

┌──┬────────────────────┬──────────────────────────────────────────────────┐
│  │Check               │Result                                            │
├──┼────────────────────┼──────────────────────────────────────────────────┤
│🟢│RAM Pressure        │RAM healthy at 45%                                │
│🟢│Swap Usage          │No swap in use                                    │
│🟢│Disk Space          │Disk healthy at 3% — 563Gi available              │
│🟢│Top Memory Consumers│No individual process exceeding 4 GB              │
│🟡│Kernel Panics (7d)  │1 kernel panic(s) in the last 7 days              │
│🔴│Jetsam Events (7d)  │6 Jetsam memory kills in 7 days                   │
│🟢│Pantheon Processes  │2 Pantheon process(es) healthy (69.5 MB total)    │
└──┴────────────────────┴──────────────────────────────────────────────────┘

Health Score: 🟡 70/100
Completed in 91ms
```

Seven checks. Sub-100ms. JSON output with `--json` for scripting and CI.

## Impact

| Metric | Before | After |
|--------|--------|-------|
| `ioreg` memory per call | 8 MB (16 MB with string copy) | ~10 bytes |
| Menubar process spawns/day | 60,480 | 10,080 (6x reduction) |
| `lsregister` peak memory | 20-50 MB buffered | ~0 (streamed) |
| Latent panic bugs | 2 | 0 |
| Dead PID accumulation | Unbounded | Pruned |
| System health diagnostic | None | 91ms, 7 checks, 0-100 score |
| New tests | — | 24 (21 doctor + 3 throttle) |
| Packages passing | 29/29 | 29/29 |

## The Lesson

**The tools you build can become the problem you're solving.** Pantheon was designed to detect infrastructure waste, memory pressure, and runaway processes — and then it became all three. The menubar, designed to monitor health, was itself degrading health. The compute detector, designed to optimize hardware routing, was wasting hardware resources.

The difference between a good tool and a great one is whether it can catch its own regressions. This case study proves Pantheon can — but only because we had the regression analysis infrastructure (Ma'at), the persistent context (Thoth), and the forensic capability (Guard) to trace symptoms back to root causes.

Rule A18 (incremental commits) saved us from losing the fix session itself. Rule A14 (statistics integrity) ensured every number in this case study is verifiable. And the new `pantheon doctor` command ensures that the next time system health degrades, the diagnosis takes 91 milliseconds instead of 4 hours.
