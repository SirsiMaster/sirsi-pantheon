# Design: Thoth as Context Root — make per-session context a budgeted resource

**Status:** proposed · **Issue:** #636 · **Companion:** `DISPATCH_PROGRESS_GATE.md` (#637)
**Author:** claude-home · **Date:** 2026-08-07
**Owner framing:** *"if i have thoth why do i need all of those other files… we solved a problem
rather than THE problem."*

---

## 1. The gap this names

Thoth was built to remove the cost of re-reading source: load a ~164-line `memory.yaml` instead
of thousands of lines of code. **It does that, and it works.**

But the 2026-08-06 leak (#636) billed **59.2 M cache-read tokens** with Thoth healthy, current,
and entirely uninvolved. Dividing it out:

```
59,200,000 tokens ÷ 1,197 spawns ≈ 49,500 tokens per spawn
```

Measured directly from the first turn of each transcript:

| Cold start per spawn | Tokens |
|---|---|
| min | 30,172 |
| **median** | **40,843** |
| mean | 43,790 |
| max | 63,967 |

**Thoth was never in that path.** The consumer command is
`claude --print --permission-mode auto` with an inbox-work prompt; nothing reads
`.thoth/memory.yaml`. Thoth is a *convention* attended sessions follow after reading CLAUDE.md —
and CLAUDE.md is itself part of the cost being measured.

**The category error:** Thoth reduces *code-reading* tokens. The bill came from
*session-startup* tokens. Both are called "context cost," so once Thoth shipped the whole
category looked solved. Nothing measured the rest. This is the same shape as the other two false
controls in #636 — a mechanism credited with covering ground it never covered.

---

## 2. What the 41K actually is

| Component | Tokens | Thoth-addressable? |
|---|---|---|
| `~/.claude/projects/…/memory/MEMORY.md` | 5,276 | **yes** |
| `<project>/CLAUDE.md` | 4,160 | **yes** |
| `~/.claude/CLAUDE.md` | 2,413 | **yes** |
| System prompt + tool schemas (7 MCP servers) + plugin/skill catalog (85 plugin dirs) | **~29,000** | **no — not files** |

**Owner-controlled markdown is ~11.8 K of ~41 K (29%). The remaining 71% is loaded by the harness
before any file is read**, so no file-compression scheme can touch it.

Two further facts:

- `.thoth/memory.yaml` is **3,970 tokens**. Where Thoth *is* loaded it **adds** to the cold start
  rather than replacing it. It repays that only in a session that goes on to do real work —
  and 99% of these exited after ~17 transcript lines.
- The auto-memory store behind `MEMORY.md` is **267 files, ~252,000 tokens**. The 5,276-token
  index is the always-loaded tip; recall pulls more.

> **Honest bound:** "Thoth as the only thing loaded" cannot reach 100%. The achievable target is
> ~41 K → ~15 K. Claiming more would repeat the original error of asserting coverage a mechanism
> does not have.

---

## 3. Design

### C5.1 — Thoth becomes the single context root for owner-authored context

One generated, compressed artifact replaces the three hand-maintained markdown files in the
load path.

- **Source of truth stays where humans edit it** (`CLAUDE.md`, memory topic files). Thoth
  *compiles*; it is not a new place to write.
- `sirsi thoth compile` emits `.thoth/context.md` — a single budgeted file containing: agent
  identity and role, standing directives, the memory index (links, not bodies), and project
  state. **Hard ceiling: 4,000 tokens**, enforced at compile time; over budget fails the build
  and names what to cut.
- Sessions load `.thoth/context.md` **instead of** global CLAUDE.md + project CLAUDE.md +
  `MEMORY.md`.
- Everything else stays recall-on-demand, exactly as memory topic files work now.

**Saving: ~11.8 K → ~4 K = ~8 K per spawn (~20% of cold start).**

```go
// internal/thoth/compile.go
type ContextBudget struct{ MaxTokens int } // default 4000

// Compile renders the single context root. Returns an error naming the largest
// contributors when the budget is exceeded — a silent truncation would recreate
// the original bug (a control that appears to cover what it does not).
func Compile(root string, b ContextBudget) (path string, tokens int, err error)
```

**Tests:** compiled output is under budget; over-budget fails loudly and names contributors;
every `[[link]]` in the index resolves; compiling twice is byte-identical (deterministic).

### C5.2 — Per-lane minimal profiles *(the larger saving, and not Thoth's job)*

A headless inbox-worker and an attended session load **identical** context today. Their needs are
opposite. Give each lane a profile in `agents.json`:

```json
"profile": {
  "mcp_servers": ["sirsi"],
  "plugins": [],
  "context_root": ".thoth/context.md"
}
```

- Four of seven configured MCP servers (**stripe, sentry, notion, linear**) are unauthenticated in
  these sessions; their schemas load regardless. A lane declares only what it calls.
- 85 plugin dirs / 53 skills are catalogued for a worker that runs `sirsi router pull` and edits
  files.

**Estimated saving: the dominant share of the ~29 K harness block.** Measure before claiming a
figure — see C5.4.

### C5.3 — Compact the memory index *(immediate, no code)*

`MEMORY.md` is **21,105 bytes / ~5,276 tokens**, already over its own 17 KB compaction threshold,
loaded by **every** session, headless or attended. Compacting to under 17 KB is a real saving on
every session anyone runs, today, with no new machinery.

### C5.4 — Make cold start a tracked budget

Nothing measures this today, so nobody would notice it growing. Add to `sirsi diagnose`:

- cold-start tokens per lane (first-turn input + cache read + cache write, median over N spawns);
- **spawn count × cold start = tokens/day per lane** — the number that actually matters;
- warn when a lane's cold start exceeds its declared budget.

```go
// internal/guard/contextbudget.go
func measureColdStart(agentID string) (median int, samples int, err error)
func checkContextBudget(p platform.Platform, report *DoctorReport)
```

**Test:** a fixture lane at 40 K cold start × 1,200 spawns ⇒ critical. That is the 2026-08-06
shape and must not pass silently.

---

## 4. Priority — the levers are unequal and multiplicative

```
tokens/day  =  spawns/day  ×  cold-start tokens
    59.2 M   =    1,197     ×      ~49 K
```

| Lever | Change | Factor |
|---|---|---|
| **Dispatch progress gate (#637 C1/C3)** | 1,197 → ≤24 spawns/day | **~50×** |
| C5.1 + C5.2 + C5.3 context work | ~41 K → ~15 K | ~2.7× |

**Fix spawn count first.** A perfectly lean 15 K context on an ungated loop still burns ~18 M
tokens/day. Optimising the coefficient on an unbounded term is not a fix. Ship #637 C1+C3, then
C5.3 (free), then C5.1/C5.2.

---

## 5. Product opportunity

Two controls now sit in the same blind spot, for the same reason:

- `checkRunawayExecutor` measures **concurrency**; the damage was **cumulative rate**.
- Thoth measures **code-reading**; the damage was **session startup**.

Each was correct within its frame and silent outside it. The generalisable product primitive is
**per-agent resource budgets with enforcement** — tokens/day, spawns/hour, cold-start ceiling —
declared in the agent's own config and enforced by the runtime, not merely observed by a doctor.
"This lane may spend N tokens/day" is the control an operator actually wants, and no
self-hosted agent runtime ships it.

**The lesson worth keeping:** every mechanism here was well-built and did its job. The failures
were all at the seams — where one mechanism's frame ended and everyone assumed another's began.
Budgets are how you make the seam explicit: an unbudgeted resource has no owner.
