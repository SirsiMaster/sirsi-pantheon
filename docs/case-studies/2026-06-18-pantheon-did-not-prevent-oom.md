# Case Study — Pantheon Did Not Prevent an OOM It Was Built to Prevent

**Date:** 2026-06-18 · **Severity:** Critical (host froze; OS Jetsam) · **Repo:** sirsi-pantheon
**Author:** claude-pantheon · **Status:** post-mortem; fix in progress (3-layer, below)
**Refs:** PANTHEON_RULES.md A1 (Safety) / A5 (Hapi must not kill active inferencing) / A23 (Truth); ADR-031 (inference broker); [[project_pantheon_ai_resource_broker]]; the 2026-06-19 owner directive *"ensure this never happens again."*

> The owner's verdict, verbatim: *"should pantheon have stopped that given all the engineering that went into these same type of issues in the past… clearly Pantheon is NOT ready for primetime."* This document agrees, with evidence.

---

## 1. What happened

While testing a new feature — `sirsi gemma serve`, a warm local-inference broker — a concurrency test fired 4 requests at once. The 48 GB machine ran out of memory, macOS Jetsam-killed processes, and the system froze. `JetsamEvent-2026-06-18-212649.ips`.

## 2. Forensics (verifiable)

From the JetsamEvent the OS killed **~5 Python/MLX processes**, each holding a near-full copy of the model:

| Process | RSS |
| :--- | ---: |
| Python (mlx) | 12,984 MB |
| Python (mlx) | 10,020 MB |
| Python (mlx) | 10,017 MB |
| Python (mlx) | 9,941 MB |
| Python (mlx) | 9,940 MB |
| **Total** | **≈ 52.9 GB** on a 48 GB box |

Reproduce: `python3 -c "import json;b=json.loads(open('<JetsamEvent>.ips').read().split(chr(10),1)[1]);print(sorted([(p['name'],p['rpages']*16384//2**20) for p in b['processes']],key=lambda x:-x[1])[:5])"`

**Root cause is two failures, not one.**

**(a) The feature was unsafe.** `sirsi gemma serve` defaulted to `--decode-concurrency 4` with no RAM gate, and — worse — the 4 concurrent `sirsi gemma` calls did not all reach the warm server; they **fell through to the cold path** (`mlx_lm.generate`), which spawns a fresh process that loads the *entire* ~10 GB model. Four cold loads + the warm server = five model copies resident at once. Neither the broker nor the cold CLI path had a memory ceiling or a serialization lock.

**(b) Pantheon — whose one job is to prevent this — was absent and toothless.** This is the part that matters.

## 3. Why Pantheon did not stop it

A tool that brands itself "clean, hydrate, **protect**" and was built specifically against Jetsam/memory pressure should have caught a process ballooning toward OOM. The audit of what actually exists:

| Mechanism | Reality on 2026-06-18 |
| :--- | :--- |
| Isis watchdog (`internal/guard/watchdog.go`, *"CPU/memory pressure monitor"*) | **Not running.** No `sirsi guard`/monitor process was live. Zero real-time protection was on. |
| AutoRenice | Even when running, it only **acts on CPU** (a sustained-CPU `hotStreak`). It *reports* RSS in the Sekhmet alert but never intervenes on memory. |
| `reniceByPID` / `sirsi relieve` (#58) | Lowers scheduler **priority**. It does **not free a single byte of RAM**. Renicing a memory balloon does nothing to stop an OOM. |
| **Hapi** — the deity *designed* to "control VRAM, GPU memory" (ADR-015, Rule A5) | A **non-running stub** (`internal/brain/hapi_bridge.go`; no process). The one component meant for exactly this does not exist as a live guard. |
| Health rubric (`sirsi diagnose`, the menubar Eye) | **Counts Jetsam events after the fact** as a 7-day trend. Detection, not prevention. |

**Conclusion:** Pantheon today has **no real-time memory guard**. Its "protection" is (1) a CPU-only renice loop that wasn't running, and (2) a post-mortem report of damage already done. The "protect" leg of clean/hydrate/protect is, for the #1 pathology Pantheon claims to own, **hollow**. The owner is right: not primetime.

The bitter irony: Pantheon already has a case study titled `docker-ghost-64gb.md`. We hit the same 64 GB wall — from our own broker — and the guard slept through it.

## 4. The fix — defense in depth (3 layers)

A pre-launch estimate (shipped) is necessary but not sufficient. The invariant the owner demanded — *Pantheon must NEVER spawn a process that can exhaust the host, its own broker included* — requires three layers, each independent so a miss in one is caught by the next.

1. **Pre-launch RAM gate — SHIPPED (#60, merged).** `gemmaSafeConcurrency`: default concurrency 1; budget each slot at a full model + headroom; **refuse to start rather than OOM**. Caveat: it is an *estimate* made once, before launch.
2. **Hard runtime cap — DO FIRST, before the broker is ever re-enabled.** Bound MLX's actual memory (`mx.set_memory_limit()` / Metal wired-limit / `iogpu.wired_limit_mb`). This makes "never OOM" true even when the estimate is wrong or the KV cache grows mid-decode — the process errors/evicts at the cap instead of taking the machine down.
3. **Live self-governance — the real "protect."** The broker registers under Pantheon's *own* guard/Hapi watchdog, which samples system-free-RAM + per-process RSS and intervenes **before the kernel Jetsams** — warn → suspend/throttle the runaway → kill-with-confirm a non-critical offender, protecting the agents (Claude/Codex/Gemma) and foreground from being the victim. Pantheon must dogfood its own thesis: the broker is governed like any other runaway.

Plus: correct the serial budget to 2×model (the (1+n) growth model implies it; #60 under-budgets serial at 1×); subtract live Claude/Codex RSS from "free" instead of trusting `inactive` as fully reclaimable; and an **invariant regression test** so a concurrency-≥2-without-a-runtime-cap default can never re-enter.

## 5. Lessons

- **A tool that claims to prevent X must itself never cause X.** Our own feature OOM'd the host the tool exists to protect.
- **Prevention ≠ detection.** Counting yesterday's Jetsam is not protecting today's session. Real protection is a live guard with teeth (suspend/cap/kill before the kernel does), not a report.
- **A designed deity is not a running one.** Hapi on paper protected nothing. Until it is a live process with intervention authority, the "protect" claim is marketing.
- **Never re-enable the broker** until layers 2+3 ship. (Owner is holding `sirsi gemma serve`.)

## 6. Recurrence — 2026-07-03: the gate worked, but wasn't in the path (ADR-031-C)

**Status update:** layers 1–4 above all shipped (ADR-031-A/B, merged) and were confirmed correct
by source-deep review on 2026-07-03 — `guard.NodeCapacity.Fits()`, the 2×model serial budget, the
dynamic reserve accounting for live Claude/Codex RSS, the cold-path file lock, and Hapi's governed
suspend/kill ladder are all real, tested, and present on `origin/main`. **This was not another design
failure.** It was a coverage failure: two pieces of local automation — a router-triage daemon
(`sirsi-gemma-worker.sh`) and, more pointedly, **the LaunchAgent that starts the warm broker
itself** (`ai.sirsi.gemma`, still invoking raw `mlx_lm.server`) — predated the broker and were
never migrated onto it. `git grep "ai.sirsi.gemma" -- '*.go'` returned zero hits: the process
actually running the warm model on this machine was invisible to the code that was supposed to
govern it.

The owner's response, verbatim, is the correct verdict on this class of incident: *"this situation
is exactly what the pantheon and router are supposed to prevent and then remedy."* The fix (see
`docs/ADR-031-C-BROKER-ENFORCEMENT-UNIVERSAL.md`) is narrower than 2026-06-18's — no new mechanism,
just closing the door beside the gate — but the lesson generalizes further than the fix does:
**a correctly designed invariant still does nothing for a caller that was never pointed at it.**
Every layer above assumed all local-model dispatch already flowed through the broker. It didn't.
Both bypasses are fixed and verified live as of this commit; a regression guard (grep audit for
direct `mlx_lm.*` invocations outside `cmd/sirsi/gemma*.go`) is recommended, not yet built —
tracked in ADR-031-C so a third bypass isn't the next person's incident to write up.
