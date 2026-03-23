# 𓂓 Case Study: Ka — Ghost App Detection

> **You uninstalled Parallels Desktop. macOS says it's gone. But 23 GB of virtual machine caches, kernel extensions, preferences, and a Spotlight registration are still on your disk. Ka finds them.**

---

## The Problem

When you drag an app to Trash on macOS, you delete the `.app` bundle. But macOS apps scatter files across your system:

- **~/Library/Preferences** — settings files
- **~/Library/Application Support** — data, databases, configs
- **~/Library/Caches** — cached data (often the largest)
- **~/Library/Containers** — sandboxed app data
- **~/Library/Saved Application State** — window positions, state
- **~/Library/Cookies** — authentication cookies
- **~/Library/Logs** — crash reports, diagnostic data
- **/Library/LaunchAgents** — background processes (still running!)
- **/Library/LaunchDaemons** — system daemons (still running!)
- **Launch Services (Spotlight)** — the app is still "registered"

None of these are removed when you uninstall an app. They're orphaned — digital ghosts consuming space, polluting search results, and sometimes running background processes.

**No existing cleaning tool comprehensively detects these remnants.** CleanMyMac shows some, but misses Launch Services ghosts and doesn't cross-reference filesystem orphans with Spotlight registrations. BleachBit is Linux-focused. AppCleaner only works *at uninstall time* — not for apps you uninstalled months ago.

### The Origin Story

Anubis was born from a 3-hour manual cleanup that recovered 47 GB of waste from a Mac. Virtual machines from Parallels (uninstalled months earlier), cached AI/ML models from multiple frameworks, and ghost app remnants from dozens of apps that were dragged to trash. "Why am I doing this by hand?"

---

## The Solution: Ka Ghost Detection

Ka implements a 5-step cross-referenced ghost detection algorithm:

### Step 1: Build Installed App Index
Scan `/Applications`, `~/Applications`, and the Homebrew cask list. Build a map of every currently installed app's bundle ID and name.

### Step 2: Scan 17 Residual Locations
Check 12 user-level and 5 system-level directories for entries containing bundle IDs that don't match any installed app. These are orphaned residuals.

| Level | Locations | Requires Sudo |
|:------|:---------:|:-------------:|
| User | 12 directories under `~/Library/` | No |
| System | 5 directories (`/Library/`, `/var/db/`) | Yes |
| **Total** | **17 locations** | — |

### Step 3: Query Launch Services
Use `lsregister -dump` to find apps registered in Spotlight whose `.app` bundles no longer exist on disk. These are "pure Spotlight ghosts" — invisible to filesystem scanning.

### Step 4: Filter System Components
Exclude all `com.apple.*` bundle IDs and known system services (Google Keystone, Microsoft AutoUpdate, etc.) that create preferences but aren't traditional user-installed apps. This prevents false positives.

### Step 5: Merge and Cross-Reference
Group all residuals by bundle ID into Ghost structs. Each ghost includes:
- Total size across all residual types
- File count
- Whether it's registered in Launch Services (Spotlight)
- Detection method: `filesystem`, `launch_services`, or both

---

## What Ka Finds That Others Don't

### 1. Spotlight Ghosts
After uninstalling an app, macOS Launch Services still "knows" about it. It appears in Spotlight suggestions, shows up in "Open With" menus, and claims resources. Ka detects these by comparing the Launch Services database against the actual filesystem.

### 2. Cross-Referenced Detection
A single uninstalled app leaves remnants across multiple directories. Ka groups them by bundle ID:

```
🦴 Ghost: Parallels Desktop (com.parallels.desktop)
├── ~/Library/Preferences/com.parallels.desktop.console.plist (8 KB)
├── ~/Library/Caches/com.parallels.desktop/ (2.1 GB)
├── ~/Library/Application Support/Parallels/ (19.8 GB)
├── ~/Library/Saved Application State/com.parallels.desktop.savedState/ (4 MB)
├── ~/Library/Group Containers/group.com.parallels/ (1.2 GB)
└── Launch Services: ✅ Still registered
    Total: 23.1 GB across 5 locations
```

*Note: The Parallels example above is illustrative of the types of remnants Ka detects. The 47 GB figure from the origin story was from a manual cleanup that included model caches and VMs alongside ghost app remnants.*

### 3. Sudo-Level Detection
With `--sudo`, Ka also scans system-level directories:
- `/Library/LaunchAgents` — finds orphaned background processes still running
- `/Library/LaunchDaemons` — finds orphaned system daemons
- `/var/db/receipts` — finds orphaned package receipts
- `/Library/Preferences` — finds system-level preference files

---

## Safety

Ka uses the same safety infrastructure as the rest of Anubis:

- **29 hardcoded protected paths** — `/System/`, `/usr/`, `~/.ssh/`, etc. are never touched
- **Scan has zero side effects** — `Scan()` only reads, never writes
- **Clean uses trash-first** — all deletions go through `cleaner.DeleteFile()` which respects macOS Trash
- **Dry-run mode** — `anubis ka --dry-run` shows what would be cleaned without touching anything

---

## How to Use

```bash
# Scan for ghost apps (user-level only)
anubis ka

# Include system-level scanning
sudo anubis ka

# Dry-run cleanup
anubis ka --dry-run

# Full scan and clean (moves to trash)
anubis ka --clean
```

---

## Architecture

| Component | Description | Lines |
|:----------|:------------|------:|
| `scanner.go` | Core 5-step algorithm | ~520 |
| `Ghost` struct | App name, bundle ID, residuals, size, detection method | — |
| `Residual` struct | Path, type, size, file count, sudo requirement | — |
| 17 residual types | Preferences through Crash Reports | — |
| `isSystemBundleID()` | Apple + known platform service filter | — |
| `extractBundleID()` | Filename → bundle ID with TLD validation | — |

### Tests
- 28 tests covering bundle ID extraction, app name guessing, system filtering, installed checks, file counting, orphan merging, dry-run cleaning, protected path safety, and location completeness
- Coverage: 42.7%

---

## How to Verify

```bash
# Run Ka on your machine
anubis ka

# Count the residual types
grep -c 'Residual' internal/ka/scanner.go      # Type definitions

# Verify the location count
grep -c 'Dir:' internal/ka/scanner.go           # Should be 17 (12 user + 5 system)

# Run tests
go test -v -cover ./internal/ka/
```

---

*All architecture facts verified against source code. The Parallels/47GB example is from the real manual cleanup that inspired building Anubis — it is anecdotal, not a controlled benchmark.*

*Published as part of the Sirsi Anubis build-in-public process (ADR-003). March 22, 2026.*
