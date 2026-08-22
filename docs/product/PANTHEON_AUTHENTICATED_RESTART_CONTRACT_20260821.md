# Pantheon Authenticated Restart Contract

**Status:** implemented CLI foundation; clean-host and full reboot evidence pending  
**Date:** 2026-08-21

## Purpose

Pantheon can restore its consented services after a user session begins, but a
normal launch agent cannot cross FileVault's pre-boot disk-unlock boundary. A
planned restart therefore needs a separate, explicit operating-system handoff.

## Three Distinct Cases

| Event | Expected behavior |
|---|---|
| App or service crash | `launchd` restarts the consented Pantheon service in the current user session. |
| Planned restart through Pantheon | `sirsi host restart --authenticated --confirm` invokes Apple's interactive `fdesetup authrestart`; after the one-time disk unlock and login handoff, consented services resume. |
| Power loss, kernel panic, or ordinary cold boot | FileVault stops at authentication. Pantheon stores no password and cannot run before the encrypted volume and user session are unlocked. |

## Security Boundary

Pantheon must never:

- disable FileVault;
- enable persistent automatic login;
- store, log, pipe, or synthesize a user's password;
- bypass Apple's authenticated-restart support check;
- describe a launch agent as capable of running before disk unlock;
- claim an unplanned cold boot can resume a graphical Metal workload unattended.

The command requires both `--authenticated` and `--confirm`. It verifies that
FileVault is active and that the Mac reports authenticated-restart support,
then hands terminal input directly to `/usr/bin/sudo` and Apple's
`/usr/bin/fdesetup`. Apple's tool owns credential collection, key staging,
restart, and post-unlock key removal.

## Operator Experience

```text
sirsi host restart --authenticated --confirm
```

An optional `--delay-minutes N` maps directly to Apple's supported delay. `0`
means immediate restart; `-1` prepares the handoff without initiating restart.

## Acceptance Evidence Still Required

- Signed Pantheon package on clean M1 and M5 hosts.
- Planned authenticated restart with the exact Pantheon/SNE tuple preserved.
- Proof that the login item and caretaker each have one installed copy.
- Proof that Nexus reconnects without model, precision, framework, or execution-mode fallback.
- Proof that an ordinary cold boot remains fail-closed at FileVault.
- Upgrade, rollback, uninstall, sleep/wake, and crash-recovery evidence.

