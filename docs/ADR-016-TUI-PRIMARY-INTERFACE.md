# ADR-016: TUI as Primary Interactive Interface

**Status:** Accepted
**Date:** 2026-05-06
**Deciders:** Cylton Collymore

## Context

Sirsi Pantheon v0.18.0 had three interactive surfaces — CLI, menu bar, and the Horus web dashboard — but none provided a cohesive post-command experience. After running any command, users saw "Done" and a dead terminal with no guidance on what to do next. The menu bar was a lightweight monitor but not an interactive workspace. The Horus dashboard required a browser.

User testing revealed that the TUI (launched via `sirsi` with no arguments) was the natural primary interface but suffered from:
- No post-run guidance across all 10 deities
- No findings drill-down
- No back navigation
- No progress feedback during long-running commands
- No state persistence between sessions
- No shared suggestion logic — each surface had independent, inconsistent UX

## Decision

**The BubbleTea TUI is the primary interactive interface.** All UX intelligence flows through a shared `internal/suggest/` package consumed by TUI, CLI, and menu bar.

### Architecture

```
internal/suggest/          ← Single source of truth
├── suggest.go             ← After(), OnError(), Placeholder(), Commands()
├── types.go               ← Context, Action structs
└── suggest_test.go        ← 14 tests, all deities covered

Consumers:
├── TUI (internal/output/tui.go)     ← Primary: full interactive experience
├── CLI (cmd/sirsi/*.go)             ← NextSteps footer after commands
├── Menubar (cmd/sirsi-menubar/)     ← Toast notifications + SSE events
└── Dashboard (internal/dashboard/)  ← SSE "complete" events with suggestions
```

### TUI Features (v0.19.0)

| Feature | Description |
|---------|-------------|
| **Post-run suggestions** | "What's Next" panel with gold-highlighted commands after every deity completes |
| **Contextual placeholder** | Input bar shows deity-specific hints instead of generic "What next?" |
| **Error remediation** | Pattern-matching guidance (permission denied, timeout, connection refused) + deity fallbacks |
| **Findings drill-down** | `findings <category>` with full detail: path, remediation, fixability, breaking warnings |
| **Streaming output** | Line-by-line via `chan string` + `bufio.Scanner` (replaces batch buffering) |
| **Elapsed timer** | "Anubis running... (12s)" with per-second updates |
| **Deity state indicators** | Roster dots: green (succeeded), red (failed), amber (has data), gold (active), grey (never run) |
| **View stack** | Esc pops to previous view — safe drill-down and back navigation |
| **Tab-to-cycle** | Tab cycles through suggested next commands in the input bar |
| **Persistent state** | Deity outcomes saved to `~/.config/pantheon/tui-state.json` |
| **Context-aware quick actions** | Suggestions rotate based on what's been run, what failed, what has data |

### Suggest Package API

```go
suggest.After(ctx Context) []Action      // Success suggestions
suggest.OnError(ctx Context) []Action    // Error remediation
suggest.Placeholder(ctx Context) string  // Input bar hint
suggest.Commands(ctx Context) []string   // Tab-cycle list
```

## Consequences

### Positive
- One edit to `internal/suggest/` updates all three surfaces simultaneously
- TUI is now fully navigable — no dead-end states for any deity or error
- Menu bar toast notifications include actionable next steps
- Dashboard SSE events carry suggestion arrays for frontend rendering
- Net code reduction: -441 lines (replacing duplicated switch statements)

### Negative
- TUI requires a terminal emulator (not accessible from mobile/web without the dashboard)
- Streaming via `chan string` is Go-specific — future Kotlin/Swift clients would need their own implementation
- View stack is in-memory only — doesn't survive TUI restart (deity state does, but navigation history doesn't)

### Risks
- Suggestion quality depends on correct `Context` construction — if deity/subcommand is wrong, suggestions are wrong
- Tab-cycling may conflict with shell tab completion in some terminal emulators

## Alternatives Considered

1. **Dashboard-first** — Make the Horus web dashboard the primary interface. Rejected: requires browser, adds latency, less discoverable than typing `sirsi`.
2. **Menu bar with dynamic items** — Make the systray menu context-aware. Rejected: systray API is limited — can't add/remove items dynamically after initialization.
3. **Separate suggestion logic per surface** — Keep TUI/CLI/menubar independent. Rejected: led to inconsistent UX and triple maintenance burden.

## Amendment (2026-06-05) — Menubar in-place SAFE ops

`a2379ab` shipped menubar **in-place actions**, a deviation from the original
"all menubar actions route through the TUI" letter. claude-home RULING (router
item `…032100`) BLESSED it with codex arch-concurrence; canonized here.

**Amended rule:** The menubar executes **SAFE / idempotent** ops **in-place**,
off the UI thread, with observable feedback (status glyph ⏳/✓/✗ + the Recent
Activity notify store). **DESTRUCTIVE and VERBOSE** ops route to the **TUI** or a
**confirm / Terminal** path. A **single shared action registry** (`Action →
Runner`) feeds both surfaces — neither forks the action set.

Why the deviation is correct, not a violation:
1. **User is sole arbiter** (Rule A23) and mandated in-place action.
2. **The route-to-TUI clause is now satisfiable.** ADR-016's original letter
   assumed a launchable TUI that did not exist when this was written (Gate-2
   scaffold, ADR-018/020). As of 2026-06-05 the TUI **is** launchable —
   `sirsi tui` runs live (commits `4f44dcd` event loop, `f850609` live thread +
   router-inbox data) — so destructive/verbose ops have a real TUI to route to.
3. **The deviation honors ADR-016's spirit.** ADR-016 existed to kill
   fire-and-forget `osascript`-Terminal spawns. In-process exec off the UI
   thread with observable feedback is *more* aligned (observable, non-orphaning)
   than the Terminal spawn it replaced.
4. **Safety preserved (Rule A1).** SAFE/idempotent ops run in-place; DESTRUCTIVE
   (`clean`, `ra deploy`/`kill`, `guard`) keep the confirm/Terminal path.

Refs: `a2379ab` (in-place actions), `4f44dcd`/`f850609` (live TUI launch path),
router items `…031942`/`…032100` (ship + ruling), ADR-010, ADR-020.

## References
- PANTHEON_RULES.md Rule A10 (Terminal UI Fidelity)
- ADR-010: Menu Bar Application (companion — menu bar launches TUI)
- ADR-015: Deity Hierarchy (deity roster and routing)
