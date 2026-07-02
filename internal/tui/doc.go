// Package tui is the Pantheon operator console, "Horus" 𓂀 (ADR-015 deity
// hierarchy, ADR-020 Hybrid C) — the interactive terminal surface.
//
// It renders exactly FIVE screens (the cap is LAW, ADR-020), each a projection
// of a merged `sirsi <verb> --json` contract read through an injectable runner
// (Rule A16). Go stays the brain: the TUI never re-implements probing logic — it
// dispatches CLI verbs and renders their JSON.
//
//   - Pulse    — memory-first home: RAM gauge, pressure, top hogs (vitals),
//     one-key relief with a before/after delta (relieve).
//   - Waste    — scan → per-item review with toggles + drill-in → tier-honest
//     clean, with a freed-space proof (scan, clean).
//   - Ghosts   — per-app residuals of uninstalled apps → clean (ghosts, clean).
//   - Health   — diagnose findings, each with its HONEST one-key fix classified
//     by FixKind (instant/relief/guidance); a guidance no-op is never offered as
//     a fix (ADR-033) (diagnose).
//   - Activity — read-only provenance ledger of what sirsi actually changed
//     (activity).
//
// Structural guarantees carried from docs/TUI_DESIGN_PROOF.md:
//
//   - Every status-bar hint is generated from the command Registry, so a shown
//     key is provably wired — a dead hint cannot render (§7 delta 2).
//   - Deity/status identity uses BMP-safe sigils, never layout-bearing
//     hieroglyphs, so terminal-font tofu can never break the grid (Rules G1–G3,
//     glyph.go).
//   - Color is semantic with a truecolor→256→16→attribute ladder; severities
//     also carry a text token so meaning survives NO_COLOR and colorblindness
//     (§2.4, §5).
//   - Destructive verbs (clean) never fire from one keystroke — a confirm modal
//     names the targets and requires a deliberate second confirmation (Rule A1,
//     §4).
//
// Launch. `sirsi` with no arguments opens the console on a genuine terminal;
// every non-interactive context (piped, redirected, --json, --quiet, or
// SIRSI_NO_TUI=1) prints help unchanged. Startup is lazy: only the first screen
// loads on Init, so the first frame is never blocked on a scan.
package tui
