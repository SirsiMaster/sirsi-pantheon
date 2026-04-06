# Isis — Health & Remedy

Isis diagnoses system health, audits network security, and auto-remediates issues. She finds problems and fixes them.

## Commands

### System health diagnostic
```bash
pantheon doctor                  # One-shot health check
pantheon doctor --json           # JSON output
```

Checks: RAM pressure, swap usage, disk space, top memory consumers, kernel panics, Jetsam events, and Pantheon background process health.

### Network security audit
```bash
pantheon isis network            # Read-only security posture audit
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
pantheon isis network --fix      # Apply safe fixes (encrypted DNS, firewall)
```

Three-layer safety model:
1. **Pre-check**: TCP probe to DNS server before changing any config
2. **Watchdog**: Polls resolution 3x over 6s, auto-reverts on failure
3. **Rollback**: Prior state saved to `~/.config/pantheon/isis/dns_prior.txt`

If the fix breaks connectivity, it reverts automatically within seconds.

### Manual rollback
```bash
pantheon isis network --rollback # Restore DNS to pre-fix state
```

### Autonomous healing
```bash
pantheon isis heal               # Auto-remediate governance failures
pantheon isis heal --fix --full  # Full remediation pass
```

Fixes lint, vet, fmt, and coverage issues detected by Ma'at.

### Resource monitoring
```bash
pantheon guard                   # Real-time RAM/CPU monitoring
```

## Output
```bash
pantheon isis network --json     # JSON for scripting/CI
pantheon doctor --json           # JSON health report
```
