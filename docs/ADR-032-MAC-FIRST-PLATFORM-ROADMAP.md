# ADR-032 — Mac-First Platform Roadmap (CLI → Menubar → TUI → GUI; Windows/Linux deferred)

**Status:** Accepted (owner directive 2026-06-19)
**Custodian:** 𓁢 Pantheon · binding review claude-home · owner is the platform arbiter
**Refs:** PANTHEON_RULES.md §0 (Roadmap) / §3 (Technology Stack) / Rule A3 (cross-platform binaries); PR #64 (CI matrix → macOS-only); `docs/PANTHEON-FEATURE-ROADMAP.md`

---

## Context

Pantheon's CI built and released for macOS, Linux, and Windows, and the canon described an interactive surface "first on macOS/Windows/Linux." But the product is sold and used on **Mac**, the menubar/TUI/GUI surfaces are Mac-native, and every cross-platform job is effort spent (and CI minutes burned, and tag releases broken on Windows) on platforms the product is not yet selling on. The gemma broker's process control (`syscall.SysProcAttr.Setpgid`, `syscall.Kill`) is Unix-only and broke the Windows build — a symptom of carrying compat weight with no buyer behind it.

The owner's directive: stop spreading effort across platforms before the Mac product is engineered, working, **and selling**.

## Decision

> **Pantheon is 100% Mac, built in this exact order: (1) Mac CLI → (2) Mac Menubar → (3) Mac TUI → (4) Mac desktop GUI app (built FROM the menubar). Only once all four are engineered, working, AND selling do we revisit Windows/Linux — 3–6 months out, and only if there is demand.**

Concretely:

1. **Build targets are Mac-only.** CI build matrix is `[macos-latest]` (PR #64, merged). No platform shims are added to make non-Mac builds pass — we simply **stop building** non-Mac. The Windows break in `gemma_serve.go` (Unix-only syscalls) is therefore moot, not something to paper over with `//go:build` shims.

2. **Off-strategy CI is gated off, not deleted** (the deferral is reversible in 3–6 months):
   - `release.yml` `windows-installer` job → guarded by `if: ${{ vars.ENABLE_WINDOWS_BUILD == 'true' }}` (off by default; a tag release no longer fails on Windows).
   - `android.yml` / `ios.yml` → `on: workflow_dispatch` only (manual, never auto-trigger). Pantheon's GUI path is **Mac desktop FROM the menubar, not mobile**; the mobile workflows are dormant, not on the roadmap.

3. **Rule A3 (cross-platform agent binaries) gets a carve-out:** cross-platform agent/CLI binaries are **deferred until the fleet/Ra phase** AND cross-platform demand exists. Until then, "single static binary, cross-compile" in §3 means *Mac* (darwin/arm64 + darwin/amd64), not all-platforms.

4. **The feature roadmap** (`docs/PANTHEON-FEATURE-ROADMAP.md`) is scoped Mac-only, every feature advancing through the same surface order: **CLI ✓ → Menubar → TUI → GUI**.

## Consequences

- A tag release builds the Mac CLI + the macOS DMG menubar, and no longer fails on a Windows installer job.
- CI minutes stop being spent on Linux/Windows/mobile builds the product does not ship.
- This does **not** touch the never-exhaust/Hapi work (ADR-031-A) — that proceeds; this only narrows build targets.
- Re-enabling a platform later is a one-line flip (`ENABLE_WINDOWS_BUILD` repo variable; restore the mobile triggers) — no rebuild-from-history.
- Honest scope to buyers and contributors: Pantheon is a **Mac product** today. Cross-platform is a later-phase, demand-gated decision, not an implied promise.
