# Security Policy — Sirsi Anubis

## Reporting a Vulnerability

If you discover a security vulnerability in Sirsi Anubis, please report it responsibly:

1. **DO NOT** open a public GitHub issue for security vulnerabilities
2. Email: security@sirsitechnologies.com
3. Include: description, reproduction steps, impact assessment

We will respond within 48 hours and provide a timeline for the fix.

## Security Model

Sirsi Anubis is a **filesystem scanning and cleaning tool** that operates with the user's permissions. Understanding its security boundaries is critical.

### Threat Model

| Threat | Mitigation |
|:-------|:-----------|
| Accidental deletion of critical files | Protected paths hardcoded in `internal/cleaner/safety.go` — CANNOT be overridden |
| Deletion without user consent | Every destructive operation requires `--dry-run` or `--confirm` flag (Rule A1) |
| Malicious scan rules | Rules are compiled into the binary — no dynamic plugin loading |
| Agent executing arbitrary commands | Agent implements a fixed command set — no arbitrary execution (Rule A3) |
| Network scanning without consent | Fleet operations require `--confirm-network` flag (Rule A4) |
| Data exfiltration via scan results | Zero telemetry, zero analytics, zero phone-home (Rule A11) |

### Protected Paths

The following paths are hardcoded as protected and **CANNOT be deleted** under any circumstances:

**macOS:**
- `/System/`, `/usr/`, `/bin/`, `/sbin/`
- `/private/var/db/`, `/Library/Extensions/`, `/Library/Frameworks/`
- `.keychain-db`, `.keychain` (any path)
- `.git`, `.env`, `.ssh`, `.gnupg`, `id_rsa`, `id_ed25519` (any directory)

**Linux:**
- `/boot/`, `/etc/`, `/usr/`, `/bin/`, `/sbin/`, `/lib/`, `/lib64/`
- `/proc/`, `/sys/`, `/dev/`
- `/var/lib/dpkg/`, `/var/lib/rpm/`

### Data Privacy (Rule A11)

- **File paths** in scan reports are NEVER transmitted externally
- **Process names/arguments** are sanitized before any fleet reporting
- **Network scan results** are stored locally only
- **Audit logs** (`~/.config/anubis/audit.log`) are local-only and NEVER uploaded
- Anubis has **zero telemetry, zero analytics, zero phone-home**

### Agent Security (Rule A3)

The `anubis-agent` binary (for fleet deployment):
- Statically compiled with `CGO_ENABLED=0`
- Zero external dependencies
- Fixed, auditable command set — no arbitrary command execution
- Does NOT auto-discover or scan network targets

## Supported Versions

| Version | Supported |
|:--------|:----------|
| 0.1.x-alpha | ✅ Current development |

## Disclosure Timeline

- **Day 0**: Vulnerability report received
- **Day 1-2**: Acknowledgment sent
- **Day 7**: Fix developed and tested
- **Day 14**: Patch released
- **Day 30**: Public disclosure (coordinated)
