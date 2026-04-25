# ADR-006: Self-Aware Resource Governance

## Status
**Accepted** — 2026-03-23

## Context
During Session 11, we experienced firsthand what a user would experience when running
multiple AI agents inside an IDE for an extended session. The Antigravity IDE's plugin
workers consumed ~219% CPU across 3 processes, starving the UI renderer and making
buttons unclickable. The system had 88% free RAM — this was purely CPU contention.

**Critical insight**: Pantheon's Guard module can audit RAM and process counts, but it
CANNOT detect CPU pressure, IDE degradation, or recommend when to stop running tools.
If a user installs Pantheon and runs `sirsi weigh` during a heavy IDE session, the
scan itself could make things worse. A tool that claims to "guard your machine" but
makes things worse under load is a product failure.

## Decision

### 1. Guard Gets CPU Pressure Awareness

Guard currently groups processes by name and reports RSS memory. It must also:

- Monitor **CPU pressure** per-process and per-group (using `ps` or `/proc/stat`)
- Detect **sustained high CPU** (>80% for >30 seconds) vs. spikes
- Flag processes that are **starving the UI/renderer** thread
- Specifically identify IDE processes (Antigravity, VS Code, Cursor, Xcode) and
  report their health separately

### 2. Self-Limiting Execution ("Yield Mode")

Every Pantheon command must check system load before running heavy operations:

```
func shouldYield() bool {
    // Check 1-minute load average vs core count
    // If load > 80% of cores, defer non-critical operations
    // Always allow: health_check, version, mcp (lightweight)
    // Defer: weigh (full scan), mirror (dedup), ka (ghost hunt)
}
```

When a command detects high system load:
- Print a warning: "⚠️ System under load (CPU: 85%). Deferring heavy scan."
- Offer `--force` flag to override
- Log the deferral for Ma'at to assess later

### 3. IDE Health Check (New MCP Tool)

Add `ide_health_check` as a new MCP tool that AI agents can call to check whether
the IDE itself is healthy. This enables agents to self-diagnose:

```json
{
  "name": "ide_health_check",
  "description": "Check if the IDE is healthy enough for agent operations. Returns CPU pressure, renderer health, and recommended actions."
}
```

This tool would:
- Identify the parent IDE process (by walking up the PID tree)
- Check if the renderer/UI process is CPU-starved
- Recommend: "Close long-running conversations" or "Restart IDE"
- Return a simple health grade: HEALTHY / DEGRADED / CRITICAL

### 4. Inter-Deity Referral for Resource Issues

When Guard detects resource pressure, it should refer to other deities:

- **High CPU from IDE plugins** → "Consider closing unused agent conversations"
- **High RAM from ghost apps** → "Run `sirsi ka` to identify ghost remnants"
- **Disk pressure** → "Run `sirsi weigh` to find waste (when CPU stabilizes)"
- **Network saturation** → "Run `sirsi scarab` to identify chatty processes"

This implements the Cross-Agent Referral Logic from ADR-005 principle #7.

### 5. The "First, Do No Harm" Rule for Resource Tools

**New Rule A16**: Pantheon tools MUST NOT make a bad situation worse.

- Before any scan that touches the filesystem: check CPU load
- Before any operation that spawns goroutines: check available cores
- Before any network scan: check existing network pressure
- If system is under duress: warn, defer, or run in reduced mode

## Consequences

- **Positive**: Pantheon becomes truly self-aware — it helps when it can and yields
  when it should. This is a differentiator from every other DevOps tool.
- **Positive**: The IDE health check enables AI agents to self-diagnose their own
  impact on the system. This is unprecedented.
- **Positive**: Users will never say "Pantheon made things worse" because it won't
  run heavy operations when the system is already struggling.
- **Negative**: Adds complexity to every command's startup path (load check).
- **Risk**: Load thresholds may need tuning per-platform (M1 vs Intel vs Linux).

## Implementation Priority

1. **Immediate**: Add `shouldYield()` check to `cmd/sirsi/root.go` (gate all heavy commands)
2. **Session 12**: Build `ide_health_check` MCP tool
3. **Session 13**: Full Guard CPU monitoring with IDE-specific detection
4. **Session 14**: Cross-deity referral messages in all findings

## References
- [ADR-005](ADR-005-PANTHEON-UNIFICATION.md) — Pantheon vision, principle #7 (inter-deity referral)
- [SIRSI_PORTFOLIO_STANDARD](SIRSI_PORTFOLIO_STANDARD.md) — Rule 28 (Cross-Agent Referral Logic)
- Session 11 diagnosis: Antigravity IDE consuming 219% CPU, rendering buttons unclickable
