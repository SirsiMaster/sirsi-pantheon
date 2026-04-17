# ADR-007: Unified Findings & Reports Portal

## Status
**Accepted** — 2026-03-23

## Context

During Session 11 dogfooding, we discovered that every Pantheon deity (Anubis, Ka,
Ma'at, Guard, Seba) generates valuable diagnostic findings — but they all print to
stdout and vanish. There is:

- No persistent report storage
- No central location to review past findings
- No way for a user to find "what did Pantheon tell me last Tuesday?"
- No dashboard (CLI or GUI) for browsing findings

The user's exact words: "Neither the average user nor the experienced vet wants to
go search their tech stack for their reports and wouldn't remember where it was once
they found it."

Additionally, the `guard` module has no deity owner. "Guard" is not an Egyptian deity.
It needs proper identity within the Pantheon.

## Decision

### 1. Thoth Owns All Reports

Thoth is the god of knowledge, writing, and record-keeping. He already owns
project memory (.thoth/memory.yaml) and the engineering journal (.thoth/journal.md).
It is natural that he also owns the **findings ledger** — the permanent record of
every diagnostic, assessment, and discovery made by any deity.

```
Thoth's Three Layers (updated):
├── Memory    — .thoth/memory.yaml       — Project context (architecture, decisions)
├── Journal   — .thoth/journal.md        — Engineering reasoning (WHY)
└── Findings  — ~/.config/pantheon/findings/  — Diagnostic reports (WHAT)
```

### 2. Guard Is Horus

The `guard` module (RAM audit, process management, CPU pressure, system health)
is designated as **Horus** — the sky god, the all-seeing falcon, protector of the
pharaoh. Horus watches over the system's vital signs.

```
𓅓 Horus — System Sentinel
├── RAM pressure, CPU pressure, swap pressure
├── I/O pressure, network pressure, IPC pressure (planned)
├── Process auditing, orphan detection, process slaying
├── IDE health detection (ADR-006)
└── Yield mode (self-limiting execution)
```

This gives Horus a clear, powerful role: **he watches what's happening RIGHT NOW**
on the machine, while Anubis focuses on what's ACCUMULATED (waste, ghosts, duplicates).

### 3. Unified Findings Directory

All deities write their findings to a central location:

```
~/.config/pantheon/findings/
├── 2026-03-23T16-50-38_horus_cpu-pressure.json
├── 2026-03-23T16-45-00_anubis_workspace-scan.json
├── 2026-03-23T16-30-00_ka_ghost-report.json
├── 2026-03-23T16-00-00_maat_coverage-assessment.json
├── 2026-03-22T14-00-00_seba_infrastructure-map.html
└── index.json   ← Thoth maintains this index
```

Each finding is a JSON file with a standard schema:

```json
{
  "id": "uuid",
  "deity": "horus",
  "type": "cpu-pressure",
  "timestamp": "2026-03-23T16:50:38-04:00",
  "severity": "warning",
  "summary": "Plugin Host processes consuming 219% CPU, starving IDE renderer",
  "findings": [...],
  "referrals": [
    {"deity": "thoth", "action": "Close long-running conversations to free context"}
  ],
  "system_snapshot": {
    "cpu_load_ratio": 0.88,
    "ram_free_pct": 88,
    "swap_used_mb": 253
  }
}
```

### 4. CLI: `sirsi findings`

A new top-level command for browsing and searching past findings:

```bash
sirsi findings                    # Show last 10 findings (default)
sirsi findings --all              # Show all findings
sirsi findings --deity horus      # Filter by deity
sirsi findings --severity warning # Filter by severity
sirsi findings --last 24h         # Last 24 hours
sirsi findings --json             # Machine-readable output
sirsi findings --open 3           # Open finding #3 in browser (HTML)
sirsi findings --clear            # Clear old findings (with confirmation)
```

### 5. GUI: Findings Tab in Mirror

The existing Mirror GUI (`sirsi mirror`) already has an HTTP server and
embedded HTML. Add a `/findings` route that renders a browsable dashboard:

- Timeline view of all findings (newest first)
- Filter by deity, severity, date
- Click to expand full finding details
- Referral links ("Ma'at suggests running `sirsi maat --coverage`")

### 6. Every Deity Writes Findings

Every deity command must:

1. Run its diagnostic/scan/assessment
2. Print results to stdout (for immediate visibility)
3. **Also** write findings to `~/.config/pantheon/findings/` as JSON
4. Include referrals to other deities when applicable (ADR-005 principle #7)

Example flow:
```
$ sirsi horus
𓅓 Horus — System Health

CPU Pressure:    🔴 HIGH (88% of capacity)
  └─ Antigravity Plugin Host: 104% CPU (PID 37363)
  └─ Antigravity Plugin Host: 77% CPU (PID 37364)
  💡 Referral: Close long-running IDE conversations

RAM Pressure:    🟢 HEALTHY (88% free)
Swap Pressure:   🟢 MINIMAL (253 MB / 1 GB)
I/O Pressure:    🟢 NOMINAL
Network:         🟢 NOMINAL

📋 Finding saved to ~/.config/pantheon/findings/2026-03-23T16-50_horus_health.json
   View all: sirsi findings
```

## Consequences

- **Positive**: Users always know where to find reports. One command: `sirsi findings`.
- **Positive**: Guard gets a proper Egyptian deity name (Horus) and a clear mandate.
- **Positive**: Cross-deity referrals create a cohesive ecosystem. Horus says "ask Ka."
- **Positive**: Findings persist — users can review last week's diagnostics.
- **Positive**: Machine-readable JSON enables integration with CI/CD, dashboards, Grafana.
- **Negative**: Every command now has a write side-effect (creating the JSON file).
- **Risk**: Findings directory grows over time. Mitigated by `sirsi findings --clear`.

## References
- [ADR-005](ADR-005-PANTHEON-UNIFICATION.md) — Pantheon vision, principle #7 (referral)
- [ADR-006](ADR-006-SELF-AWARE-RESOURCE-GOVERNANCE.md) — Self-aware resource governance
- Session 11: "neither the average user nor the experienced vet wants to search"
