# Anubis — Hygiene Engine

Anubis finds junk on your machine that cleaning apps miss. 58 scan rules across 7 domains, plus ghost app detection and file deduplication.

## Commands

### Scan for waste
```bash
pantheon scan                    # Quick scan (or: pantheon anubis weigh)
pantheon scan --all              # Scan all categories
```

Scans for: stale caches, orphaned build artifacts, unused dependencies, crash reports, browser junk, Docker volumes, and more. Read-only — never deletes anything.

### Reclaim storage
```bash
pantheon anubis judge            # Dry-run by default — shows what would be cleaned
pantheon anubis judge --confirm  # Actually move artifacts to Trash
```

Always runs in `--dry-run` mode unless you explicitly confirm. Safe by design.

### Hunt ghost apps
```bash
pantheon ghosts                  # Scan for remnants of uninstalled apps
pantheon ghosts --sudo           # Include system directories (requires sudo)
```

Detects: Launch Services ghosts, orphaned plists, leftover caches, and Spotlight phantoms from apps you've already uninstalled.

### Find duplicate files
```bash
pantheon dedup ~/Downloads ~/Documents   # Compare two directories
pantheon anubis mirror ~/Photos          # Same thing, deity syntax
```

Opens a web UI with smart recommendations for which copy to keep.

### Monitor resources
```bash
pantheon guard                   # Watch RAM pressure and system resources
```

Real-time monitoring of memory pressure, swap usage, and top consumers.

### List installed apps
```bash
pantheon anubis apps                        # List all apps
pantheon anubis apps --ghosts               # Show ghost residuals
pantheon anubis apps --uninstall AppName    # Full uninstall with residual cleanup
```

## Output
```bash
pantheon scan --json         # JSON output for scripting
pantheon scan --quiet        # Suppress output except summary
```
