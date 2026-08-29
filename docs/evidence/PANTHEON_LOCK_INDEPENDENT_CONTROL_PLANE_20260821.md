# Pantheon Lock-Independent Control Plane

**Date:** 2026-08-21  
**Classification:** platform-foundation / product-operational capability  
**Candidate:** `Pantheon-0.23.8-beta-20260821.8-arm64.app`  
**Status:** accepted as the active development caretaker; ad-hoc signed and not distributable

## Problem

Pantheon's launch agent could be running while its dashboard, SNE lifecycle,
recovery, diagnostics, and router bridge were absent. The process entered
`systray.Run`, but all nonvisual services were initialized inside AppKit's
`onReady` callback. macOS may defer that callback while the graphical session is
locked. A resident PID therefore looked ready even though the control plane was
not listening.

## Repair

The menu bar executable now creates exactly one `controlPlane` before entering
AppKit. That object owns:

- the loopback dashboard and protected SNE API;
- SNE install and lifecycle managers;
- notification storage;
- Guard, periodic scan, and live-refresh workers;
- router registration;
- one context, cancellation path, and idempotent shutdown.

`onReady` now attaches the visual menu to the already-running object. It does
not create a second dashboard, lifecycle manager, router registration, or state
authority. Headless mode uses the same control plane. SIGTERM, SIGINT, menu Quit,
and normal return converge on the same `sync.Once` shutdown.

## Evidence

- Focused tests passed:
  - `go test ./cmd/sirsi-menubar`
  - `go test ./internal/dashboard`
- Transactional package identity passed for `0.23.8-beta (20260821.7)`.
- The first sandboxed image attempt failed at `hdiutil` with
  `Device not configured`; the prior candidate remained untouched.
- The identical outside-sandbox build completed and produced the copied app and
  DMG.
- Pantheon's own `surface install gui` transaction installed the copied app.
- Launchd reports exactly one `ai.sirsi.pantheon` job, `runs = 1`, PID `96074`,
  and no prior exit.
- PID `96074` owns `127.0.0.1:9119`.
- A host-loopback request returned the live Horus dashboard and `/api/stats`
  returned structured telemetry.
- The copied `.8` successor then passed the missing true lock gate in one
  atomic host sample: `IOConsoleLocked = Yes`; launchd held exact `.8` PID
  `2534`, `runs = 1`, and no prior exit; a host-loopback request returned fresh
  `/api/stats` telemetry timestamped `2026-08-21T23:46:27.944527-04:00`.

Artifact hashes:

| Artifact | SHA-256 |
|---|---|
| `sirsi-menubar` | `77f0a3f287e06e8b92968f3fd2f6d35f5de3c1367df62673d7d020fac6cb08fc` |
| `sirsi` | `ad535b15393727055cf55497de20a5ff233d3fd513e761ab6596ab973cb01220` |
| DMG | `36c5c524d8517504d5083c45db82fcc4b7c3c8debcaae86e42a3de13ae6ddc81` |

Candidate `.8` also repaired the live RAM read model. Shared vitals now parses
the page size declared by `vm_stat` rather than assuming 4 KiB, and publishes
exact byte fields instead of zero placeholders. The live `.8` endpoint reported
51,539,607,552 total bytes, 10,911,121,408 used bytes, 40,628,486,144 free
bytes, 21.1704% used, and low pressure. Regression tests cover both 4 KiB and
16 KiB page hosts.

## Boundaries and remaining gates

- This candidate is ad-hoc signed. It is not a public release and must not be
  distributed as one.
- Locked-session service continuity is proven. Full reboot/login restoration is
  still a separate distribution gate because FileVault may require an interactive
  login after an unplanned cold boot.
- Developer ID signing and notarization remain blocked on unlocking the dedicated
  release keychain and configuring notarization credentials.
- No SNE model, Metal workload, or performance benchmark was run by this repair.
  No SNE performance claim changed.
