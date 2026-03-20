# Safety Design — Sirsi Anubis
**Version:** 1.0.0
**Date:** March 20, 2026

> **This document is PARAMOUNT.** Sirsi Anubis deletes files and kills processes.
> Every safety mechanism described here is a hard requirement, not a suggestion.

---

## 1. Core Safety Principles

1. **Never delete without explicit confirmation.** Every destructive operation requires `--confirm` or `--dry-run` first.
2. **Protected paths are hardcoded.** They cannot be overridden by configuration, flags, CLI arguments, or user input.
3. **Scan and Clean are separate phases.** Scanning has zero side effects. Cleaning requires a separate, explicit command.
4. **Dry-run is always available.** Every destructive command supports `--dry-run` to preview what would happen.
5. **System integrity is non-negotiable.** Anubis MUST NOT render a system unbootable, crash a running application, or corrupt data.

---

## 2. Protected Paths (Hardcoded Deny List)

The following paths are **NEVER** touched by Anubis, regardless of scan rules, policies, or user input. This list is hardcoded in `internal/cleaner/safety.go` and enforced at the deletion engine level.

### 2.1 macOS Protected Paths
```
/System/
/usr/
/bin/
/sbin/
/private/var/db/
/private/var/folders/  (partial — only system-owned)
/Library/Extensions/
/Library/Frameworks/
/Library/LaunchDaemons/ (system-owned only)
/Library/LaunchAgents/ (system-owned only)
~/Library/Keychains/login.keychain-db
~/Library/Keychains/System.keychain
~/.ssh/
~/.gnupg/
~/.config/anubis/  (own config)
```

### 2.2 Linux Protected Paths
```
/boot/
/etc/
/usr/
/bin/
/sbin/
/lib/
/lib64/
/proc/
/sys/
/dev/
/var/lib/dpkg/
/var/lib/rpm/
~/.ssh/
~/.gnupg/
```

### 2.3 Universal Protected Paths
```
.git/           (in any project — never delete version control)
.env            (in any project — never delete environment config)
*.key           (any private key file)
*.pem           (any certificate file)
id_rsa*         (SSH keys)
*.keychain*     (macOS keychain files)
```

---

## 3. Dry-Run Guarantee

### 3.1 How Dry-Run Works
When `--dry-run` is active:
- The scan phase runs normally and discovers artifacts
- The output shows exactly what WOULD be deleted, with sizes
- **Zero files are touched.** No deletions, no moves, no modifications.
- The exit code reflects what would have happened (0 = clean, 1 = would have cleaned)

### 3.2 Default Behavior
| Command | Default Mode | Requires Flag to Delete |
|---------|-------------|------------------------|
| `anubis weigh` | Scan only (never deletes) | N/A |
| `anubis judge` | **Error** — must specify `--dry-run` or `--confirm` | Yes |
| `anubis guard --slay` | **Confirmation prompt** | Yes (or `--confirm`) |
| `anubis hapi --kill-orphans` | **Confirmation prompt** | Yes (or `--confirm`) |
| `anubis sight --fix` | **Confirmation prompt** | Yes (or `--confirm`) |

**Key design decision:** `anubis judge` with no flags does NOT default to dry-run — it **errors out** and tells the user to explicitly choose. This prevents accidental muscle memory from causing deletions.

---

## 4. Trash vs Delete

### 4.1 Policy
| Operation | Default Behavior | Override |
|-----------|-----------------|----------|
| User files (caches, logs, temp) | **Move to Trash** via Finder | `--permanent` flag |
| System artifacts (receipts, plists) | **Delete directly** (sudo required) | N/A |
| Container images/volumes | **Docker engine removal** | N/A |
| Processes (kill) | **SIGTERM first**, SIGKILL after 10s | `--force` for immediate SIGKILL |

### 4.2 Trash Implementation (macOS)
Anubis uses the macOS Finder API (`osascript`) to move files to Trash, which:
- Preserves the "Put Back" functionality
- Creates a `.Trashes` entry with the original path
- Allows the user to recover files from Trash

---

## 5. Process Safety

### 5.1 Before Killing a Process
Anubis checks:
1. **Is it a protected process?** (Finder, WindowServer, launchd, kernel_task → NEVER kill)
2. **Is it actively doing work?** (CPU > 0% in last 60s → warn before killing)
3. **Is the user running it interactively?** (check foreground process group)
4. **Would killing it orphan child processes?** (check process tree)

### 5.2 Protected Processes (Never Kill)
```
kernel_task
launchd
WindowServer
loginwindow
Finder
Dock
SystemUIServer
cfprefsd
diskarbitrationd
fseventsd
notifyd
opendirectoryd
securityd
syslogd
```

### 5.3 GPU Process Safety (Hapi Module)
Before killing a GPU process:
1. Check GPU utilization — if actively computing (>5% GPU), warn and require `--force`
2. Check if it's a training process — look for common ML framework patterns
3. Check if it has saved a checkpoint recently — if not, warn about data loss
4. Default: SIGTERM with 30-second grace period for checkpoint save

---

## 6. Network Safety

### 6.1 Fleet Sweep Safeguards
- `anubis scarab discover` only discovers — never scans
- `anubis scarab sweep` requires `--confirm-network` flag
- No "scan all networks" default — user must specify subnet/VLAN
- Agent deployment requires explicit `--deploy` flag
- All network operations are logged with timestamps and target IPs

### 6.2 Agent Trust Model
- Agents accept commands ONLY from authenticated controllers
- Agent command set is fixed (scan, clean, report, update) — no shell access
- Agent results are signed to prevent tampering in transit
- Agent self-update requires controller authentication + checksum verification

---

## 7. Validation Pipeline

Every deletion request passes through this pipeline:

```
Request → Path Validation → Protected Path Check → Size Check → Dry-Run Gate → Confirmation → Trash/Delete → Log
    ↓           ↓                    ↓                  ↓            ↓              ↓            ↓          ↓
  Parse    Is path real?      Is it protected?     Is it > 1GB?   Is --dry-run?  Is --confirm?  Execute   Audit
           ↓ No → Error       ↓ Yes → Block        ↓ Yes → Warn   ↓ Yes → Show   ↓ No → Prompt
```

---

## 8. Audit Logging

Every destructive operation is logged to `~/.config/anubis/audit.log`:

```json
{
  "timestamp": "2026-03-20T11:00:00Z",
  "action": "delete",
  "path": "/Users/user/Library/Caches/com.parallels.desktop",
  "size_bytes": 1048576,
  "rule": "virtualization.parallels",
  "mode": "trash",
  "dry_run": false,
  "confirmed": true
}
```

The audit log is **append-only** and **never cleaned** by Anubis itself.
