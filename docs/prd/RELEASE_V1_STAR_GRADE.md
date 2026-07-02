# PRD — Pantheon v1 "Star-Grade" Release

**Status:** DRAFT — awaiting owner bless (Rule 17)
**Author:** claude-pantheon · 2026-07-01
**Basis:** Full end-to-end release audit of origin/main @ fde4635c (2026-07-01): engine verified
release-grade (full test suite green, 63 API endpoints/tools all confirm-gated, 175 CLI commands
no dead ends); blockers are presentational + governance, not architectural.

## /goal

A releasable, demo-able product at the quality bar of a 50K-star GitHub tool:
one-line install → jaw-drop inside 60 seconds. Exit criteria (all must hold):

1. `sirsi` (no args) opens a Mole-grade interactive operator console (TUI) — not help text.
2. README opens with a <15s hero GIF and current, truthful badges.
3. sirsi-pantheon.web.app renders flawlessly (no ghost text), shows the current version, deploys green.
4. Ma'at ≥ 85 (Rule A17), zero version drift across all public surfaces (Rule A14).
5. Menubar #130 merged + notarized DMG current; memory-first root view (owner directive 2026-06-26).
6. 2-minute demo script written, rehearsed, and recorded (YC-ready; Sequoia-sendable link).
7. Tagged release candidate with full multi-platform assets.

Stars are an outcome, not a deliverable. The deliverable is the quality bar that earns them.

## Commercialization Gate (portfolio law — required section)

- **Buyer/User:** Individual developers and AI power-users on macOS (free tier); engineering
  teams/fleets (paid tier later via Ra).
- **Pain:** Dev + AI workloads choke Macs — RAM pressure, beachballs, cache bloat, ghost app
  remnants. AI agents multiply the problem. Existing tools either monitor without fixing or
  fix without showing proof.
- **Primary Workflow:** Notice slowness → `sirsi` → see the cause named → one keystroke fixes it →
  before/after proof on screen. (Monitor → identify → FIX; ADR-033 is the product.)
- **Willingness To Pay:** Free CLI earns trust + stars (top-of-funnel, hiring + fundraise signal).
  Paid: fleet orchestration (Ra), neural ranking, team dashboards.
- **Trust Boundary:** Full Disk Access (granted at setup, never mid-use); 100% local analysis;
  zero telemetry (Rule A11) — this is a headline differentiator, not fine print.
- **Operational Owner:** Cylton/Sirsi. Support via GitHub Issues + Discussions.
- **Done Evidence:** demo video link, live site, release assets, Ma'at report ≥85, CI green,
  README GIF, launch copy drafted.
- **Classification:** `commercial-product` (free tier) upon completion of this PRD; today: `pilot`.

## Design language (all surfaces)

Lean INTO the Egyptian-pantheon identity — it is the memorable differentiator. Gold `#C8A951`
on black `#0F0F0F`, deep lapis `#1A1A5E` accents, hieroglyph glyphs as icons. Copy is plain
English (owner rule: deity names stay, jargon dies). Every alarm is current + actionable
(surfaces canon: "if nothing can clear it, it must not alarm"). Every action shows
before → after proof. Motto stays: "Weigh. Judge. Purge."

---

## Phase 1 — Truth & Trust (Day 1; parallel lanes)

The credibility floor. A YC partner who opens the repo must find zero contradictions.

| # | Item | Detail | Owner |
|---|------|--------|-------|
| 1.1 | Version-truth sweep | README badges, PANTHEON_RULES header, RELEASE_NOTES.md (kill fictional v0.9.0-rc1), PANTHEON_ROADMAP (redate, current phase), .thoth/memory.yaml → all match VERSION. One commit. | claude-pantheon |
| 1.2 | Ma'at ≥ 85 | Lift `ka` 42.7%→80 and `mirror` 60%→80 coverage (table-driven tests on pure functions first); wire coverage data for the ~45 dark modules so the audit can see them. | claude-pantheon |
| 1.3 | Fix docs deploy | Firebase `FAILED_PRECONDITION` on `sites/sirsi-pantheon/channels/live` — repair site/target/credentials so the website can ship again. | claude-pantheon |
| 1.4 | Merge #130 | Menubar UX rework — **owner-gated**: click-through QA of Insight, Scan & Clean toggles, Leftover Apps. | **Cylton** |
| 1.5 | Canon prune | Land COMMERCIALIZATION_GATE.md + docs/prd/ on main (from side branch). The 8 phantom §4 canon docs: write the 3 that matter for release (DEPLOYMENT_GUIDE, VERSIONING_STANDARD, PROJECT_SCOPE — gemma drafts), delete the rest from the §4 list (LEAN: prune, don't accrete). Update ADR-INDEX (ADR-033). | gemma → claude-pantheon |
| 1.6 | Rule A8 docs | User guides for the 6 undocumented features (relieve, reclaim-snapshots, insight, remediation enforcement, binary-drift heal, app-hangs relief) — gemma drafts, thread reviews. | gemma → claude-pantheon |

## Phase 2 — UI Revamp (Days 2–5)

### 2A. The flagship: interactive TUI operator console (ADR-020 Hybrid C)

The star-magnet for a CLI tool is the first keystroke. `sirsi` with no args must open the
console (TTY-detected; falls back to help when piped).

- **Day 2 — TUI_DESIGN_PROOF.md** (canon gate: ADR-020 forbids TUI code before the design
  proof clears independent review). Contents: 5 screens, full keymap, data contracts
  (all reads via existing `--json` commands — Go stays the brain), Mole-quality bar,
  failure/empty states.
- **Days 3–5 — build** (clean-sheet; v0.22's TUI died for being unreleasable — small scope,
  high polish this time). Five screens, no more:
  1. **Pulse** (home, memory-first per owner directive): RAM gauge + pressure, top hogs named,
     one-key `r` relieve with before/after MB delta.
  2. **Waste**: scan → per-item review (toggles, drill-in — same flow as menubar #130) → clean
     → freed-space proof.
  3. **Ghosts**: leftover apps with residual sizes → clean per app.
  4. **Health**: diagnose findings, each with its one-key fix (ADR-033 law: never a monitor).
  5. **Activity**: the provenance ledger — what Pantheon did, when, with what result.
- Keys: `s` scan · `c` clean · `g` ghosts · `r` relieve · `d` diagnose · `?` help · `q` quit.
- Review: independent (claude-home binds; codex SME on the design proof).

### 2B. Website revamp (sirsi-pantheon.web.app)

Rebuild `docs/index.html` as a single stunning page (current one renders as ghost text —
scroll-reveal never fires):

- Hero: animated terminal (pre-recorded cast, CSS/JS playback — no heavy deps), one-line
  install front and center, live version pulled from GitHub releases API (never hardcode again).
- Pantheon grid: the deities as a visual system (what each does, plain English).
- Proof strip: real screenshots (TUI, menubar, dashboard), zero-telemetry badge, Apache-2.0.
- Restyle getting-started / FAQ / build-log to match; build-log stays (build-in-public is on-brand).
- Bar: Lighthouse ≥95, flawless at mobile + desktop, no animation that can strand content.

### 2C. README revamp (the storefront — stars are won here)

- Hero GIF (<15s, vhs-recorded): `sirsi` → Pulse → relieve → proof.
- 3-bullet pitch, truthful badges (auto-derivable), quickstart in 3 commands,
  screenshot grid per surface, honest comparison table (vs CleanMyMac/btop/monitors),
  "why local-only" trust section, contributor CTA + good-first-issues.

### 2D. Menubar + Dashboard polish

- Menubar: after #130 merges — memory-first root view (RAM lead, waste demoted), notarized DMG.
- Dashboard: fix `/api/stats` zero RAM fields, wire `/api/node-status` collector in the shipped
  binary, hide/label the Ra tab until the backend exists ("coming soon", not a 503), center
  layout at wide viewports.

## Phase 3 — Demo & Launch Kit (Days 6–7)

| # | Item | Detail |
|---|------|--------|
| 3.1 | 2-minute demo script | Arc: slow Mac (relatable) → `brew install` → `sirsi` → Pulse names the hog → `r` relieve, before/after proof → `s` scan → clean with toggles → ghosts → "all local, zero telemetry" → substrate tease (the AI-fleet future) — 15s each beat. |
| 3.2 | Recorded demo | vhs/asciinema master + MP4; the Sequoia-sendable link. |
| 3.3 | Launch copy | Show HN post + tweet thread + product page copy (gemma drafts, owner blesses). |
| 3.4 | Star engine | CONTRIBUTING refresh, 10 good-first-issues, Discussions on, issue templates. |
| 3.5 | Release | Tag + goreleaser: DMG, brew, deb/rpm; verify `brew install` cold on a clean path. |

## Sequencing & orchestration

- Day 1: Phase 1 lanes run parallel (gemma drafts docs while coverage work runs).
- Day 2: design proof (gate) + website build starts (independent of TUI).
- Days 3–5: TUI build; README + site converge; menubar DMG after #130.
- Days 6–7: demo + launch kit + tag.
- Pipeline per canon: gemma drafts → thread implements/reviews → claude-home binds → ship + writeback.

## Owner gates (the only asks)

1. **Bless this plan** (Rule 17) — and the three decisions below.
2. **#130 click-through QA** (~10 min) — unblocks the menubar lane.
3. **Design bless** of TUI_DESIGN_PROOF + website hero (Day 2 checkpoint).
4. **Demo rehearsal** (Day 7).

## Decision points (owner)

| Question | Options | Recommendation |
|---|---|---|
| Hero surface for the demo | TUI · menubar · dashboard | **TUI** — it's the GitHub-native wow; menubar is the retention surface |
| Version target | v0.24.0-beta · v1.0.0-rc1 | **v1.0.0-rc1** — "1.0" is the story YC/press/HN respond to; rc keeps honesty |
| Website scope | Fix current page · full rebuild | **Full rebuild** — the broken reveal-animation page isn't worth saving |

## Risks

- **TUI scope creep** (killed v0.22's TUI): mitigated by the 5-screen cap + design-proof gate.
- **`ka` coverage grind** (42.7→80): heaviest technical item; start Day 1, pure functions first.
- **Notarization creds** (Team 9D382WV988): owner-gated; needed for the DMG lane only.
- **Star count is not controllable**: we control install→wow time, README quality, launch surface.

## Done evidence (Completion Law)

Demo video · live site (Lighthouse report) · README GIF · Ma'at ≥85 output · CI green ·
tagged release with assets · brew cold-install transcript · #130 merged · version-drift
sweep diff.
