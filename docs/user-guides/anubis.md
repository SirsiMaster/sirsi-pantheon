# Anubis — Hygiene Engine

Anubis finds junk on your machine that cleaning apps miss. 58 scan rules across 7 domains, plus ghost app detection and file deduplication.

## Commands

### Scan for waste
```bash
sirsi scan                    # Quick scan (or: sirsi anubis scan)
sirsi scan --all              # Scan all categories
```

Scans for: stale caches, orphaned build artifacts, unused dependencies, crash reports, browser junk, Docker volumes, and more. Read-only — never deletes anything.

### Reclaim storage
```bash
sirsi clean                   # Dry-run by default — shows what would be cleaned
sirsi clean --confirm         # Actually move artifacts to Trash
```

Always runs in `--dry-run` mode unless you explicitly confirm. Safe by design.

### Review Empty Trash operation history (macOS app)

In the Sirsi menubar app, open **Anubis → Empty Trash → Operation history**. History is read only. It shows up to 20 recent operation summaries at a time; choose **Load older operations** to read the next page. Open a row to inspect the exact reviewed inventory, each entry's durable outcome, and any recorded errors or integrity warnings.

- **Completed** means a terminal receipt confirms the reviewed operation completed.
- **Partially completed** means the receipt records both successful deletions and errors.
- **Needs attention** means the operation ended with errors, or a binding check prevented deletion before its first attempt.
- **Result not finalized** means there is no valid terminal receipt. Entries without a durable result have an unknown outcome; this status does not establish whether the command is still running or stopped.
- **Record could not be verified** means the history record failed integrity checks. Its outcomes are unknown.

Refresh history and review the current Trash contents before any new confirmed action. History never repeats deletion and cannot restore permanently deleted items. Empty Trash has no undo.

### Hunt ghost apps
```bash
sirsi ghosts                  # Scan for remnants of uninstalled apps
sirsi ghosts --sudo           # Include system directories (requires sudo)
```

Detects: Launch Services ghosts, orphaned plists, leftover caches, and Spotlight phantoms from apps you've already uninstalled.

### Find duplicate files
```bash
sirsi dedup ~/Downloads ~/Documents   # Compare two directories
sirsi duplicates ~/Photos             # Same thing, top-level alias
```

Opens a web UI with smart recommendations for which copy to keep.

### Monitor resources
```bash
sirsi guard                   # Watch RAM pressure and system resources
```

Real-time monitoring of memory pressure, swap usage, and top consumers.

### List installed apps
```bash
sirsi anubis apps                        # List all apps
sirsi anubis apps --ghosts               # Show ghost residuals
sirsi anubis apps --uninstall AppName    # Full uninstall with residual cleanup
```

## Output
```bash
sirsi scan --json         # JSON output for scripting
sirsi scan --quiet        # Suppress output except summary
```
