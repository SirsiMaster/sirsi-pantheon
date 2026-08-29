# Pantheon Menubar Action Closure

**Status:** Local candidate closure accepted; Developer-ID release remains separate  
**Date:** 2026-08-22  
**Authority:** Sirsi owner directive

## Product contract

Every actionable menubar signal must support this loop without opening an
informational Terminal window:

> observe -> explain -> prepare -> confirm when required -> execute -> verify -> record

Terminal remains valid only when macOS must visibly collect credentials or when
the operator deliberately opens the TUI. It is not the default remediation UI.

## Root cause

Pantheon already contains a typed dashboard action registry, runner, event
stream, confirmation-token engine, notifications, and native menubar result
rows. The menubar separately hardcoded command strings instead of consuming the
canonical registry. Later services therefore drifted into four inconsistent
states: native execution, Terminal launch, disabled information row, or
dashboard-only action.

## Current closure matrix

| Surface | Prior behavior | Current state | Required closure |
|---|---|---|---|
| Gemma broker | Terminal status window | Native start/check/status/stop/quarantine plus SNE Control Center | Closed |
| Router Doctor | Terminal information | Native safe repair (`--fix`) | Closed |
| Compute Profile | Terminal information | Native receipt + drill-down | Closed |
| System Diagnostics | Terminal information | Native receipt plus prepared Repair action | Closed |
| Horus | Opened TUI/Terminal | Opens real Horus dashboard | Make dynamic ops rows actionable |
| Guard | Terminal monitor | Native bounded check | Add renice/slay prepare-confirm actions |
| Ra deploy/kill | Terminal | Native server-issued prepare/hash/token/commit | Closed |
| Restart | Visible authenticated Terminal | Correct exception | Preserve Apple-owned credential UI and resume receipt |
| Anubis safe clean | Native preview/confirm | Accepted | Keep manifest identity and post-clean verification |
| Ghost findings | Native report only | Incomplete | Add selected ghost cleanup prepare-confirm flow |
| Network findings | Dashboard-only repair | Native audit plus confirmed fix | Closed |
| SNE lifecycle | Open Nexus + broker check | Native lifecycle rows plus SNE Control Center | Closed for menu scope |
| App recovery | Backend/API only | Governed Recovery Center entrypoint | Target-specific mutation remains in Recovery Center |
| Binary drift/update | Diagnosis only | Native prepared signed updater | Closed |
| Permissions | Repeated folder probes | Quiet TCC-only status plus explicit System Settings action | Closed |
| Liveness supervisor | Containment-disabled | Truthful absence | Expose status; do not reenable until containment ends |
| Vault prune | Dashboard-only | Missing | Add preview-confirm-prune flow |
| Fabric/Ops rows | Disabled information | Clickable rows route to the exact Horus/Fabric control center | Closed |
| Recent Activity | Detail only | Partial | Offer retry/remediate only when original action is safe/current |

## Implemented architectural layers

1. Exported one canonical registry and removed stale command aliases.
2. Menubar actions invoke the same typed loopback runner as the dashboard.
3. Diagnostics lead to a prepared system repair and specialized repair controls.
4. Destructive registry actions use native prepare/confirm/commit controls.
5. Runner status publishes a terminal receipt; the menubar requires a matching
   successful receipt before displaying success.
6. Unavailable or target-specific workflows route to their governed Pantheon
   control center rather than opening informational Terminal output.
7. SNE, recovery, update, permissions, network, cleanup, and service lifecycle
   entrypoints remain visible.
8. Resident permission detection no longer touches Desktop, Documents,
   Downloads, Mail, or Safari; it cannot trigger folder-consent dialogs.
9. Spotlight remains the sole broad background filesystem indexer.

## Accepted local proof

- Installed CLI SHA-256: `d4888a68e5da7e2f9c568d29c46ef9f013d6996f4e131f5ee8808bed963c8b37`
- Installed menubar SHA-256: `1bc6c2051f8eaeab70f8e04d4edc791ef747b2b08a8f5a4cfd920e3f0f81fa89`
- Focused dashboard/menubar/confirmation/platform tests: pass
- Installed diagnosis: `100`, `green`, zero actionable findings
- Installed safe action: matching `status/success` terminal receipt
- Destructive proof: prepare-only response carried a token and action hash; no
  destructive proof action was committed
- Resident observation: no TCC request, `mdfind`, Jackal, or scan worker

## Acceptance gate

No actionable menubar result may end at prose. Every result must either resolve
in Pantheon, open the exact macOS-owned permission/credential surface required,
or explain why no safe action exists. Dashboard, menubar, TUI, MCP, and CLI must
derive action identity from one registry and produce the same receipt.
