# Isis — Health & Remedy

Isis diagnoses system health, audits network security, and auto-remediates issues. She finds problems and fixes them.

## Commands

### System health diagnostic
```bash
sirsi doctor                  # One-shot health check
sirsi doctor --json           # JSON output
```

Checks: RAM pressure, swap usage, disk space, top memory consumers, kernel panics, Jetsam events, and Pantheon background process health.

### Network security audit
```bash
sirsi isis network            # Read-only security posture audit
```

Audits 6 areas:
- **DNS**: Is encrypted DNS (DoH/DoT) configured?
- **WiFi**: WPA3/WPA2 or open network?
- **TLS**: Verifies TLS 1.3 to known endpoints
- **CA Certificates**: Audits root certificate store for anomalies
- **VPN**: Detects active VPN tunnels
- **Firewall**: Is macOS application firewall enabled?

Returns a security score (0-100) with per-check findings.

### Auto-fix network issues
```bash
sirsi isis network --fix      # Apply safe fixes (encrypted DNS, firewall)
```

Three-layer safety model:
1. **Pre-check**: TCP probe to DNS server before changing any config
2. **Watchdog**: Polls resolution 3x over 6s, auto-reverts on failure
3. **Rollback**: Prior state saved to `~/.config/pantheon/isis/dns_prior.txt`

If the fix breaks connectivity, it reverts automatically within seconds.

### Manual rollback
```bash
sirsi isis network --rollback # Restore DNS to pre-fix state
```

### Autonomous healing
```bash
sirsi isis heal               # Auto-remediate governance failures
sirsi isis heal --fix --full  # Full remediation pass
```

Fixes lint, vet, fmt, and coverage issues detected by Ma'at.

### Resource monitoring
```bash
sirsi guard                   # Real-time RAM/CPU monitoring
```

## Output
```bash
sirsi isis network --json     # JSON for scripting/CI
sirsi doctor --json           # JSON health report
```
