# Sirsi Pantheon v0.23.8-beta Release Notes

**Date:** June 30, 2026 (tag `v0.23.8-beta`)
**Status:** Beta — signed + notarized macOS release
**Platform:** macOS Apple Silicon (primary, per ADR-032 Mac-first); Linux CLI builds; Windows deferred

---

## What is Pantheon?

Pantheon is a unified infrastructure-intelligence CLI + native macOS menubar. One binary, all deities. It monitors, identifies, and **fixes** infrastructure waste and resource problems on developer workstations.

```bash
brew tap SirsiMaster/tools && brew install sirsi-pantheon
# or
go install github.com/SirsiMaster/sirsi-pantheon/cmd/sirsi@v0.23.8-beta
```

Or download the signed + notarized DMG from [GitHub Releases](https://github.com/SirsiMaster/sirsi-pantheon/releases/tag/v0.23.8-beta).

## Highlights of the v0.23.x line

- **Signed + notarized DMG releases** (since v0.23.3-beta) — stable TCC/Full Disk Access identity; no re-approval churn on update.
- **`sirsi update`** (#98) — self-update that installs signed releases and verifies the download.
- **Native macOS menubar** (ADR-030) — NSStatusItem + NSPopover + SwiftUI; Eye of Horus health icon that tints to real machine state.
- **Surfaces honesty arc** — an element alarms only for a *current, fixable* condition: 7-day trends are plain Info, never alarms (#107, #108, #127); stale thread registrations no longer paint Horus yellow (#105); waste titles no longer count protected AI model weights.
- **Router/thread hardening** — truthful per-thread liveness, stranded-inbox surfacing, `sirsi router doctor`, blocking `router_wait` MCP tool (#103).
- **Menubar clean-result screen no longer dead-ends** (#112, the tagged commit) — every pushed view has a BackBar + Done.

## Landed on `main` since the tag (ships in the next release)

The **ADR-033 remediation arc** (#113–#131) — "every finding maps to a real macOS lever, never a monitor" ([ADR-033](ADR-033-REMEDIATION-CATALOG.md), accepted 2026-06-30):

- **Security:** `sirsi update --cli` verifies the downloaded binary's SHA-256; the cleaner is symlink-safe (#113).
- **Real memory remediation** — flush caches + name the hog, instead of opening a monitor (#124).
- **`sirsi reclaim-snapshots`** — thins local Time Machine (APFS) snapshots via the macOS admin-auth dialog, with before/after disk-free proof (#126).
- **Honest swap alarm** — gated on live memory pressure + real RAM stats, not the Go heap (#123).
- **Watchdog fixes** — `AutoRenice` actually fires (was dead code) via an A21-safe seam (#121); `Darwin.Kill` SIGTERMs, waits, then SIGKILLs (#120); `CleanFile` gained a dry-run preview (Rule A1 parity, #122).
- **Test honesty** — failure-path coverage for the kill/trash/update engines (#114, #118, #119); deterministic agentguard preflight (#131); flaky-test fixes (#129); canon reconciled with reality (#116).

Full detail: [CHANGELOG.md](../CHANGELOG.md).

## Verified Metrics (2026-07-01)

- `VERSION` file = `0.23.8-beta` = latest tag `v0.23.8-beta`
- 2,078 test functions (`grep -r '^func Test' --include='*_test.go' | wc -l`)
- Go 1.25 (`go.mod`)
- CI: macOS-first (ADR-032)

## What's Next

**v1.0.0-rc1 — the Star-Grade sprint.** Earned through the release PRD (`docs/prd/RELEASE_V1_STAR_GRADE.md`, PR #132), not declared.

---

*Building in public. The feather weighs true. No excuses.*

**GitHub:** https://github.com/SirsiMaster/sirsi-pantheon
**Web:** https://sirsi.ai/pantheon
