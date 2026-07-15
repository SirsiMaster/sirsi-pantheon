# ADR-038: Pantheon Brand (Emerald + Gold) & the Universal Surface

## Status
**Accepted** — 2026-07-13 · claude-pantheon. Custodian: 𓂀 Horus (surfaces) + 𓆄 Ma'at (consistency gate). Supersedes the gold-primary + lapis palette in PANTHEON_RULES A10 v1 and the per-surface hardcoded colors. Owner directive (2026-07-13): "green is the color of Sirsi; green and gold are the colors of Pantheon — make the design emerald-based," and "I should be able to see this in every user viewport (CLI, Menubar, TUI, and Swift app)."

## Context

Two problems, one root cause — **no single source of visual truth**:

1. **Brand.** Colors were hardcoded independently in at least four places — `internal/output/terminal.go` (CLI, gold #C8A951 + lapis), `internal/tui/color.go` (TUI tokens), `internal/dashboard/colors.go` (HTML), `ios/Pantheon/Theme/PantheonTheme.swift` (Swift). They had already drifted (different greens, different grounds). A rebrand meant editing four files and hoping they matched.
2. **The dashboard.** The Router Conduit Supervisor health view — the one the owner said is "exactly how I want my dashboard to look" — lived only as a routine's markdown output and a one-off web Artifact. It could not appear in the CLI, TUI, menubar, or Swift app because there was no shared **data producer** and no shared **palette** for the renderers to draw.

The end-state this ADR sets: one palette and one report producer, consumed by thin renderers on every surface — the ADR-037 "one authority, thin adapters" doctrine applied to presentation.

## Decision

### 1. One palette — `internal/brand`
`internal/brand/brand.go` is the sole source of the Pantheon identity. Semantic **Roles** (never decorative), resolved per **Scheme** (dark/light):

| Role | Dark | Light | Meaning |
|---|---|---|---|
| **emerald** | `#2bd29b` | `#0f7a54` | brand identity · healthy · interactive |
| **gold** | `#cdad5a` | `#8a6d1f` | second accent · owner-action · Horus glyph |
| ok | `#2bd29b` | `#0f7a54` | pass / safe (emerald family) |
| warn | `#e0913a` | `#9a5f14` | needs attention |
| danger | `#f2545b` | `#c0353b` | destructive / error |
| info | `#5a9bc9` | `#2f5aa8` | neutral informational |
| dim · ink · ink2 · bg · panel · line | — | — | neutrals, green-biased |

**Emerald leads; gold is the deliberate second color.** Green is Sirsi's; green + gold are Pantheon's.

Non-Go surfaces do not copy hex — they **derive** from this file via emitters, shipped as a lever (`sirsi brand tokens --format …`):
- `--format css` → custom properties (local dashboard, SirsiNexus, published Artifacts)
- `--format swift` → a generated `Color.Pantheon` extension marked DO-NOT-EDIT (menubar, Swift app)
- `--format json` → deterministic tokens for any tool

CLI and TUI (Go) import `brand` directly. The Swift file is **generated**, so macapp cannot silently drift.

### 2. One report producer, four renderers — the Universal Surface
The supervisor dashboard is not re-implemented per surface. A single Go producer emits the report as structured data (the checks: name · result · state · drill-down evidence); each surface is a **thin renderer** over that data + the brand palette:

- **CLI** — `sirsi supervisor` prints the check table (lipgloss, emerald/gold).
- **TUI** — a screen renders the same report; rows drill down.
- **Menubar / Swift** — a SwiftUI view over `--json`, rows expand to evidence.
- **Web (Horus / Nexus)** — the reference HTML renderer (the Artifact) served live.

The report producer and the palette are the two shared authorities; the four renderers hold no data logic and no colors of their own.

## Neith's Triad (A22)

**1. Data flow**
```
internal/brand (palette) ─┬─ Go import ───────► CLI · TUI  (lipgloss)
                          ├─ tokens --css ────► dashboard · Nexus · Artifact
                          └─ tokens --swift ──► menubar · Swift app (generated)

sirsi supervisor (report) ─┬─ text ──► CLI
                           ├─ model ─► TUI screen
                           ├─ --json ► menubar / Swift view
                           └─ --json ► Horus web renderer
Fallback: any surface with no report data renders an empty, honestly-labelled board — never a fabricated one.
```

**2. Implementation order**
- **P1 — Palette source + emitters + `sirsi brand`** ✅ (this ADR's PR): `internal/brand`, three emitters, tests, command.
- **P2 — Adopt the palette** (additive, per surface): repoint `dashboard/colors.go`, `output/terminal.go`, `tui/color.go` to `brand`; generate `PantheonTheme.swift`. Verify each surface renders unchanged except color.
- **P3 — Report producer**: `sirsi supervisor [--json]` — the structured health/hand-off report (wraps the existing `router status`/`node-status`/`doctor` the routine already runs).
- **P4 — Renderers**: CLI table → TUI screen → menubar/Swift view → Horus web. Each consumes P3 + the palette.
- **P5 — Canon + gate**: A10 rewrite; a Ma'at check that fails CI on a raw hex literal outside `internal/brand`.

P1 is the minimum viable source of truth; P2–P4 make the identity and the dashboard visible everywhere; P5 keeps them from drifting again.

**3. Key decisions**

| Question | Options | Recommendation |
|---|---|---|
| Palette home | per-surface consts / one Go pkg / external design-tokens file | **one Go pkg (`internal/brand`) + emitters** — Go surfaces import; others derive, no network, no drift |
| Swift colors | hand-authored / generated | **generated** (`--format swift`, DO-NOT-EDIT) — macapp cannot drift from canon |
| Brand identity | keep gold-primary / emerald-primary | **emerald-primary, gold second** — owner directive; green = Sirsi heritage |
| Dashboard per surface | reimplement each / one producer + renderers | **one producer + thin renderers** — same data, same palette, every viewport (ADR-037 doctrine) |
| Drift prevention | docs only / CI gate | **Ma'at CI check** (P5) — a raw hex outside `internal/brand` fails the gate |

## Consequences
A rebrand is now a one-file edit that propagates to every viewport. The supervisor dashboard the owner approved becomes a shipped surface renderable in CLI, TUI, menubar, and Swift from one report producer — not four copies. Until P5's gate lands, adoption is enforced by review. Reference renderer (web): the published Artifact.
