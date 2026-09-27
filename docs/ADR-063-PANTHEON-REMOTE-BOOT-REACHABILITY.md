# ADR-063: Pantheon Remote — Boot-Time Reachability Requirements

## Status
**Proposed (design)** — 2026-09-27

## Context
Pantheon Remote (a future iOS/mobile fleet-control surface, no PRD or code
exists yet — nearest groundwork is `docs/PHASE1_MOBILE_GOMOBILE_AUDIT.md`,
Phase-1 draft) will need to reach this Mac from a phone to trigger actions
like restart. Every wake/router mechanism Pantheon runs today lives in
`~/Library/LaunchAgents` — a **user-scope** LaunchAgent. macOS does not run
user-scope LaunchAgents until a user is logged in at the Console; at the
login window (post-boot, pre-login, including after a FileVault-encrypted
restart) nothing in that scope is running. claude-io independently found the
same root cause in Tailscale's GUI-app mode: a phone-triggered restart can
strand itself, unable to reach the Mac again until someone is physically at
the keyboard to log in.

This is a real, already-latent gap in Pantheon's own agent infra (not just a
future mobile concern) — it just has no consequence yet because nothing
remote-triggers a restart. Building Pantheon Remote makes it consequential.

## Decision
Capture the requirement now, as a design gate, before any code is written:

1. **System-level services from boot.** The Mac-side agent (router/wake
   substrate) and the tailnet daemon must run as **system-level** services
   (LaunchDaemon, root-owned, `/Library/LaunchDaemons`) reachable from boot,
   not user-scope LaunchAgents. This requires root install + signing
   considerations distinct from today's user-scope install path.
2. **Distinct "at login window, reachable" state.** The reachability model
   must represent a state between "fully booted, nobody logged in" and
   "user logged in, full session" — Remote can reach the daemon layer at the
   login window even though the interactive agent fabric (Claude sessions,
   menubar, etc.) is not yet running.
3. **FileVault-safe restart action.** A remote-triggered restart must not
   fire-and-forget. It must wait for and report **actual** reachability
   post-restart (daemon-layer ping), not just issue the reboot command and
   assume success — a FileVault Mac stops at a pre-login unlock screen that
   nothing remote can pass.
4. **Phone-side login-window view.** The mobile client needs an explicit
   screen for "Mac is at the login window, waiting for local unlock" plus
   an explicit login step in its own flow, rather than treating "reachable"
   as binary.

## Alternatives Considered
1. **Ship Remote on the existing user-scope LaunchAgent substrate, defer
   the gap**: Rejected — the failure mode (remote restart strands itself
   until physical keyboard access) is worse than the feature it's shipped
   with; a restart action is one of the first things such a surface offers.
2. **Skip system-level services, poll from the phone until the user
   happens to log in**: Rejected — silently unbounded wait with no
   phone-side signal is a worse user experience than an explicit
   login-window state, and doesn't fix the "no daemon-layer answer at all"
   root cause.

## Consequences
- **Positive**: The login-window gap is documented and gated before any
  LaunchAgent→LaunchDaemon migration code lands, instead of being
  discovered mid-implementation or in production after a bad restart.
- **Negative**: A LaunchDaemon migration is a root-level install (signing,
  privilege, security review) — heavier than today's user-scope install,
  and needs its own security pass before shipping.
- **Risk**: If Remote implementation starts without this ADR being
  followed, a remote restart action can leave the fleet unreachable until
  someone is physically at the machine.

## References
- `docs/PHASE1_MOBILE_GOMOBILE_AUDIT.md` (Phase-1 draft, no code yet)
- Task ledger: `pantheon-remote-boot-reachability` (claude-pantheon, opened
  2026-09-26, phase: Design capture)
- Related finding: claude-io, Tailscale GUI-app mode reachability gap
