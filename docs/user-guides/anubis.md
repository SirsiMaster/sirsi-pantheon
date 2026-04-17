# Anubis — Hygiene Engine

Anubis finds junk on your machine that cleaning apps miss. 58 scan rules across 7 domains, plus ghost app detection and file deduplication.

## Commands

### Scan for waste
```bash
sirsi scan                    # Quick scan (or: sirsi anubis weigh)
sirsi scan --all              # Scan all categories
```

Scans for: stale caches, orphaned build artifacts, unused dependencies, crash reports, browser junk, Docker volumes, and more. Read-only — never deletes anything.

### Reclaim storage
```bash
sirsi anubis judge            # Dry-run by default — shows what would be cleaned
sirsi anubis judge --confirm  # Actually move artifacts to Trash
```

Always runs in `--dry-run` mode unless you explicitly confirm. Safe by design.

### Hunt ghost apps
```bash
sirsi ghosts                  # Scan for remnants of uninstalled apps
sirsi ghosts --sudo           # Include system directories (requires sudo)
```

Detects: Launch Services ghosts, orphaned plists, leftover caches, and Spotlight phantoms from apps you've already uninstalled.

### Find duplicate files
```bash
sirsi dedup ~/Downloads ~/Documents   # Compare two directories
sirsi anubis mirror ~/Photos          # Same thing, deity syntax
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
