# Menubar Clean Workflow — Scan → Review → Clean, one drillable flow

> Drafted by the local model (gemma, `TASK: draft`), Tier-2 reviewed and bound by
> claude-pantheon (2026-07-17). Canon: `project_pantheon_menubar_ux_rework`.
> Safety: every deletion is Trash-first and dry-run-gated (Rule A1).

## Goal

Consolidate today's fragmented surfaces — Anubis-Hygiene (totals), Scan-Clean
(bulk), Leftover Apps (separate) — into ONE drillable flow: scan once, review
every finding with a per-item toggle, clean exactly what's checked. The operator
sees precisely what will be removed and why before anything moves.

## The three steps

### Step 1 — Scan
- **Trigger:** one "Scan" tap; stays on the same screen (no premature navigation).
- **In progress:** progress bar + "Searching for leftover apps, caches, logs, and snapshots."
- **Empty result:** "Nothing to clean — your system is tidy." (no Review step).
- **Error:** "Scan couldn't finish. Grant Full Disk Access and try again." with a link to the FDA guide.

### Step 2 — Review (the heart of the rework)
- **Header:** "Review findings" · **Sub:** "Check what you want removed."
- **Grouped by category:** Caches · Logs · Leftover Apps · Snapshots. Each group has a header with a per-category **Select-all** toggle and the group's total size.
- **Each row:** path (wrapped, never clipped — the #239 lesson), size, and a plain-English reason ("Safe: regenerated on next launch" / "Caution: you may want this — a saved container").
- **Running total:** a sticky footer — "Clean N items (X GB)" reflecting ONLY checked rows, updating live as toggles change.

### Step 3 — Clean
- **Button:** "Clean N items (X GB)" — the exact checked total, never the scan total.
- **Confirm:** names the count and size ("Move 12 items (3.4 GB) to Trash?") — a deliberate second confirmation (Rule A1; never one-tap-destructive).
- **In progress:** "Moving to Trash…" spinner.
- **Success:** "Freed X GB" with a per-category breakdown and the honest undo note: "Files are in the Trash — restore any from there."
- **Error:** "Couldn't move some items to Trash. Check disk permissions." — partial success reports what DID free.

## Per-item toggle rules
- **Safe** items (caches, logs, regenerable) — pre-checked.
- **Caution** items (leftover-app data, snapshots, anything a user might want) — unchecked; the operator opts in deliberately.
- **Select-all** is per-category and never crosses categories (checking all Caches leaves Snapshots untouched).
- Every row has an individual toggle that overrides its default.

## Out of scope (v1)
Cleaning schedules · a manual path-exclusion editor · deep system-repair tools.
These are deliberately deferred so v1 ships the core loop.

## Acceptance checklist
1. Scan triggers a progress bar and does NOT navigate away immediately.
2. Review groups findings into exactly: Caches, Logs, Leftover Apps, Snapshots.
3. Every row shows path (wrapped), size, and a plain-English reason.
4. Safe items are checked by default on entering Review; caution items unchecked.
5. A Select-all toggle exists per category, independent of the others.
6. The Clean button shows the count + size of ONLY the checked items, live.
7. The confirmation dialog states the exact count and size to be removed.
8. The success screen shows freed bytes broken down per category.
9. All removals go to the Trash — never a permanent delete (A1).
10. An empty scan skips Review entirely; a scan error links to the FDA guide.

## Build notes (implementation, not part of the spec)
- Reuse the existing `sirsi scan --json` (findings + tiers) and `sirsi clean --confirm`
  (trash-first, tier-honest) verbs — this is a SURFACE rework, no new safety code.
- The per-item selection maps to `clean`'s existing per-path/tier flags; the running
  total is a client-side sum of checked rows.
- Ghosts (Leftover Apps) fold in as one category via the existing `sirsi ghosts`/`ghost-clean`.
