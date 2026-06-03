# Continuation Prompt — Pantheon Menubar UX + mds_stores Mitigation

**Resume name:** "Pantheon menubar UX" · **Repo:** sirsi-pantheon · **HEAD:** `0f7a6a1` (pushed, main) · **Date:** 2026-06-03

## What this session did
Theme: **make the menubar ACT, not just inform** (user's #1 complaint: "great information that doesn't resolve into any ACTION"), then a live **`mds_stores` write-amplification** mitigation.

### Shipped (all in `main`, pushed)
- `a2379ab` **menubar in-place actions** — safe commands (Sync Memory, Quality Audit, Scan, Find Leftover Apps, Hardware Info, Risk, Consistency Check, Ingest Knowledge, Ra Collect) execute in-process + write results to the notify store (Recent Activity) instead of opening a Terminal. Destructive (clean, ra deploy/kill, guard) keep the Terminal/confirm path (Rule A1). `cmd/sirsi-menubar/actions.go` `runActionInPlace`.
- `39a0ec4` **function labels, not deity names** — menu shows what each item DOES (deity names stay internal command taxonomy). Also fixed `anubis clean`→`clean`.
- `b7040ff`/`154cb3b` **in-app clean (two-click)** — "Clean Waste…" runs dry-run preview (arms a hidden "Confirm Clean — X → Trash"), "Confirm Clean" pipes "y" to `anubis clean --confirm`. Rule A1 satisfied (dry-run + explicit confirm + trash-first + protected paths).
- `2710811` **native ~/.Trash move** — replaced `osascript tell Finder` (needs Automation TCC the launchd menubar lacks → was rejected with the error sound) with native `os.Rename` into ~/.Trash. `internal/platform/darwin.go` + tests.
- `0f7a6a1` **disk-visibility spectrum (all/some/none)** — `internal/platform/CheckDiskAccess()` → `DiskAccessLevel` none/some/full + which locations readable/blind. Menubar `applyFDAState` shows "⚠ No Disk Access" / "◐ Partial Visibility" / hidden-at-full; click opens FDA Settings pane. (Live probe: this machine = **some**.)
- Earlier in-thread: ADR-025 R3 (`e10355a`), ADR-024 Amendment 1 reap-key+worker-gate (`cc14ae0`), ADR-026 4a/4b menubar ops rows (`b84396a`/`08f78cb`/`a86aeed`/`d91acb1`), stranded-work surfacing (`fef3591`), suspended-retention (`6b6b811`), binary unification ADR-023.

## CRITICAL operational state
- **BINARY FROZEN (user decision):** `sirsi` + `sirsi-menubar` deployed to all 3 PATH copies (~/.local/bin, ~/go/bin, /opt/homebrew/bin). **DO NOT rebuild+resign+redeploy** — each ad-hoc resign changes the code-signature hash and REVOKES any Full Disk Access grant. The user is granting FDA once against the frozen binary; a redeploy breaks it. Only redeploy with explicit user approval (ideally stable Developer-ID signing).
- **`mds_stores` mitigation:** created `~/Development/.metadata_never_index` (Spotlight exclusion) + pruned CTR registry 209→52 records (`threads.json` 124 KB→31 KB) via `sirsi thread prune --older-than 1h --suspended-older-than 6h`. `mds_stores` dropped to ~1.3%. **USER TODO:** `sudo mdutil -i off -d ~/Development` to drop the existing index (no-sudo `.metadata_never_index` only stops future indexing).

## Pending / next
1. **Codex reviews owed** (codex offline most of session): ADR-025 (`024046`), ADR-024 Amendment 1 (`032136`), suspended-retention, stranded-work surfacing+concurrence, **native-trash safety** (`033638` — A1 destructive path), menubar UX commits. On codex reply: address verdicts; the A18 canon-text reinforcement folds into a coordinated ADR-024 canon-sync.
2. **Durable signing** — FDA/permissions only persist with a stable **Developer-ID** signature (Apple enrollment pending). Routed to claude-home as the real "see everything" product blocker.
3. **Proposed (user to confirm):** auto-run the registry prune in the hourly `sweep.sh` so `threads.json` never bloats again (prevents `mds_stores` recurrence).
4. **Deeper UX:** make the ops ROWS themselves actionable (click stale thread → act; click drift → fix).

## Thread/loop
- Multi-agent: claude-pantheon (me), live peer `thr-fb73` (pantheon-pro-ux-loop), claude-home (lane coordinator/root-authority), codex-pantheon (reviewer, offline). Shared git repo + working tree — **surgically stage only your files**, verify committed tree in a throwaway worktree, never co-opt another lane's uncommitted work.
- Use `bin/sirsi` for router CLI. Heartbeat loop via `/loop` + a Monitor on `items/`.
