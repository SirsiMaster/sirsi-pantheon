# SNE Native Mac Control Surface

**Date:** 2026-08-21  
**Classification:** Commercial-product UI integration  
**Status:** Source implemented, Swift build passed, and deterministic minimum/wide visual QA accepted; live accessibility and clean-host evidence pending

## Product result

Pantheon's Swift Mac application now exposes **SNE — Models & Engine** as a first-class navigation destination. It uses Pantheon's existing Go loopback API and therefore shares the same authoritative lifecycle, admission, model-store, catalog, and recovery state as the CLI, dashboard, MCP, and Nexus integration.

The native surface provides:

- exact active model, runtime, manifest, profile, and lifecycle state;
- supported-model discovery with architecture, precision, execution mode, memory, cache topology, context capacity, support status, and next-gate information;
- explicit license review and confirmation before verified installation;
- governed start, stop, and retry-after-recovery actions;
- inactive model removal with shared-object retention disclosure;
- signed runtime-catalog retained versions, rollback, and inactive-version removal;
- actionable recovery and lifecycle-tool status;
- loading, success, and failure states; and
- keyboard-native SwiftUI controls with destructive actions separately identified.

## Authority boundary

Swift is an API client, not a second inference or lifecycle engine. It does not decide tuple admission, memory limits, package validity, model support, licensing identity, removal safety, or rollback eligibility. Those decisions remain in Pantheon's Go engine. Mutation requests require the same private bearer capability as every other local control surface.

The capability reader accepts only the canonical private file:

`~/Library/Application Support/Sirsi/Pantheon/sne-local-api.token`

It rejects missing, non-regular, symlinked, group/world-readable, malformed, or short credentials before issuing a mutation.

## Evidence

- Added: `macapp/Sources/SirsiMenubar/SNEControl.swift`
- Navigation: `macapp/Sources/SirsiMenubar/Views.swift`
- Visual-regression registration: `macapp/Sources/SirsiMenubar/Snapshot.swift`
- Build: `swift build --package-path macapp` passed after source compilation and linking.
- Accepted minimum-width rendering: `docs/evidence/artifacts/sne-native-mac-control-20260821/sne-models-engine-min.png`
- Accepted wide rendering: `docs/evidence/artifacts/sne-native-mac-control-20260821/sne-models-engine-wide.png`
- Artifact checksums: `docs/evidence/artifacts/sne-native-mac-control-20260821/SHA256SUMS`

The initial sandboxed build failed before source compilation because SwiftPM's nested `sandbox-exec` was denied. The identical escalated build reached compilation, identified three contained styling errors, and passed after those exact corrections. A final build after snapshot registration also passed.

## Deterministic visual QA

Both accepted renderings use a populated, non-live snapshot fixture. They prove layout and information hierarchy only; they do not claim a running model or successful lifecycle mutation.

- Minimum-width rendering accepted: complete navigation, engine readiness, active tuple, supported-model distinctions, licensing, and signed-catalog state are visible without clipping.
- Wide rendering accepted: the same information and action hierarchy remains intact at the expanded panel width, with no placeholder glyphs or content loss.

Rejected iterations are retained as engineering history:

1. The first rendering showed only the shell because SwiftUI `.task` does not execute under the snapshot renderer. The repair introduced explicit deterministic fixture state.
2. The second rendering showed only card labels because AppKit-backed `GroupBox` content did not render through `ImageRenderer`. The repair added a snapshot-renderable `SNECard` while preserving native `GroupBox` in the live application.
3. The third rendering exposed unsupported live `Link` and progress controls as prohibition glyphs and clipped the lower content. The repair substituted snapshot-safe license text, removed synthetic loading state, and expanded the registered canvas.

The accepted evidence intentionally avoids pretending that a snapshot is live integration proof.

## Claim boundary

This is source and compile evidence. It does not yet prove live model installation, VoiceOver behavior, visual quality on an unlocked display, signed-package integration, clean-host installation, M1/M5 behavior, or serving performance. It changes no immutable SNE artifact and makes no token-rate, bandwidth, occupancy, cache, MMU, register, or ANE claim.

## Accessibility semantics hardening

The native surface now exposes stable accessibility identifiers and explicit spoken semantics for engine readiness, in-flight work, active runtime, governed model identity, model-specific install/remove/license actions, safe retry/stop behavior, signed runtime catalog state, and lifecycle-tool availability. Decorative readiness color is hidden from assistive technology rather than used as the sole status signal. Model action labels include the governed model identity, and disabled removal actions expose the server-projected reason.

The identical current-toolchain build completed after this change. The first sandboxed SwiftPM invocation was denied before manifest compilation by nested `sandbox-exec`; the escalated identical invocation compiled `SNEControl.swift`, linked, and completed successfully. This is compile evidence, not live VoiceOver interaction evidence. The graphical-session admission gate still reported the M5 console locked, so live assistive-technology and lifecycle exercise remains pending rather than being inferred.

## Remaining gates
+
## Locked graphical-session recovery

Pantheon now treats a known Metal startup rejection caused by a locked macOS graphical session as a temporary session condition, not as a GPU, model, runtime, or package defect.

- Lifecycle classification emits stable code `metal_session_locked`.
- The read model projects `waiting-for-unlock` with an actionable recovery message.
- The exact model ID, runtime ID, runtime SHA, manifest SHA, and profile remain intact.
- A healthy already-running service is never overwritten by the projection.
- The native UI says **Waiting for unlock**, offers **Retry after unlocking**, and uses stable accessibility identifier `sne.lifecycle.retry`.
- When macOS posts `sessionDidBecomeActiveNotification`, the UI retries only if the lifecycle still carries `metal_session_locked`; ordinary failures and healthy services are untouched.
- Retry re-enters Pantheon's normal admission path. It does not clear caches, change precision, select another model, or permit CPU/cloud fallback.

Focused Go lifecycle/readiness tests and the current-toolchain Swift build pass. This is source and compile evidence. Live lock/unlock, model startup, Metal availability, and VoiceOver behavior remain pending an unlocked qualification session.
+
### Background caretaker behavior
+
### Caretaker overhead repair

The first implementation used a full `IOService` tree dump every two seconds. A bounded same-session 20-sample timing exposed that as unacceptable caretaker overhead:

| Probe | Mean wall time |
|---|---:|
| Full IOService tree | 1,251.682 ms |
| Root-only `IOConsoleLocked` | 9.418 ms |
| Reduction | 99.25% |

Production now queries only `Root` at depth 1 every five seconds while waiting. Parsing accepts only a standalone `"IOConsoleLocked" = Yes|No` property and fails closed on missing, invalid, or embedded lookalike text. This measurement characterizes the host-side registry probe only; it is not model, GPU, token-rate, or power evidence.


The Go lifecycle manager also owns a single cancellable unlock watcher, so recovery does not depend on the native control panel being open. It reads macOS's native `IOConsoleLocked` state through `/usr/sbin/ioreg`. Once unlocked, it retries exactly the preserved request and still requires the complete normal admission chain. Manual start, stop, or runtime-catalog mutation cancels the watcher. Focused tests prove one locked start plus one successful retry, and separately prove that stop prevents any late restart.



1. Exercise keyboard navigation, VoiceOver labels, contrast, reduced motion, and Dynamic Type behavior on an unlocked Mac.
2. Run the complete install/start/conversation/recovery/stop/remove workflow against an admitted copied package.
3. Repeat through the signed/notarized clean-host package on M5 and separately on M1.

## Human-access homes

- Canonical repository source: this file.
- Owner Reading Room: `Pantheon/SNE_NATIVE_MAC_CONTROL_SURFACE_20260821.md` and `.html`.
- Native Google Workspace record: https://docs.google.com/document/d/1Ts1kz9kJpLsN3P3Mq_2-wsyaI-xSFd1BAs7dKguQ9T4/edit
