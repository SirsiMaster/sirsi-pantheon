# Legacy relay app operation admission and resolution

## Exact legacy source compatibility

The source selector accepts only a clean build of the canonical main module
at an explicitly audited full revision. Version strings, nearby revisions,
missing or duplicate VCS settings, and dirty builds do not select a profile.

| Retained release | Audited source revision |
|---|---|
| 0.24.121 | `22944b2458f377a9f3e5e10c63c3442cb3ea83c3` |
| 0.24.129 | `0fd8463011ee2ebae12494895669b0e7b20b9ddf` |
| 0.24.134 | `ba2a07bb464bbfd68808254a7d489769f122149b` |
| 0.24.137 | `cb59d56f849235eb80f5e4b10514fc89467fb8af` |

The audited 129 and 134 relay transport and publication-lock blobs are identical
(`a9d00bd181fb74358c3293eb3b0b546d1fb764e9` and
`7ac4f3f29277d18580e1d89aa41229c5905cb341`). Neither includes `servicedrain`.
137's startup gate (`5d94d6de8c76789af2812cc4839f0f1c7a8f3211`) returns
without opening a maintenance endpoint when no intent exists at startup. Its
endpoint, when present, supports snapshot and one-way activation, not a live
drain or active-to-held transition. An occupied endpoint therefore still
refuses legacy maintenance; the source profile never overrides that check.

All original PID/kernel/image/plist, retained signed-payload, replacement,
publication-exclusion, retirement and custody checks remain mandatory. These
profiles preserve **unknown old outcomes**, not certified drain: neither signal
cancellation nor a publication lock joins old forwarding work. Source support
does not activate maintenance, replay old requests or certify host adoption.

This is the source contract for the existing legacy maintenance fallback, not
a new service, actor permission, router store, or Studio producer-route gate.
It implements Astra's bounded BR01 design of 2026-10-25, SHA256
`41e7c04a6dfba74e74f0e14acd042c494c652c283ca604c1898617539ea426e1`.
The prior app-bridge review remains changes-required evidence; this document is
not source or runtime acceptance.

## Authority and durable state

The GUI remains unprivileged. Only the fixed protected installed engine handles
the closed OS-authorized app request. Its signature, ownership, nofollow path,
hash, release and implementation capability are qualified before authorization.
Normal registered-agent authorization and public command semantics remain intact.

Preview performs service inspection **and durable admission bookkeeping**. Under
the existing selected-service lock it issues a fresh random operation ID and
transaction nonce at the next monotonic generation. It does not retire a service.
The full plan, verb, original transaction and restore revision/digest are bound
to that ticket. One bounded root0600 nofollow/ACL-checked state record beside the
existing maintenance journal retains the high-water generation/current ticket
and its reserved/running/terminal/fenced disposition. No tombstone farm, expiry
reset, keyring or new database exists. Failed publication retains explicit debt,
bounded to32 staging leaves. After a failed durability acknowledgement, resolution
resynchronizes the exact retained record before reporting a verified fence.

Mutation checks the exact ticket and persists running intent under the SAME
service lock before calling the existing private lock-held controller body.
The ticket nonce is the confirm transaction ID. Repeated accepted IDs return
their recorded outcome, never another retirement. Recovery/abort/restore each
receive a distinct ticket; the original transaction is not a reusable operation.

## Explicit resolution, never absence-only reset

The app retains the complete outstanding public binding and unknown receipt
separately from whatever status was most recently read. Old transaction A and
empty status cannot resolve B. Frame ownership preserves this across navigation;
bounded existing user preferences preserve public bindings across app restart.
No tokens, private plist environment or request bodies are stored there.

“Check result and prevent delayed submission” takes the same service lock.
It returns a matching terminal result, or durably advances the generation and
records a fence before permitting a future review. A late original must acquire
that lock and fails its ticket check. Merely reserved work can be reported never
admitted; running-intent work retains unknown past outcome. Nonce-bound pending
preparation or lifecycle history routes to exact recovery, not a reset. Busy,
corrupt or unverified persistence remains explicit retry/recovery debt.

A verified fence plus zero current lifecycle debt permits a NEW preview and NEW
consent. It never replays the original. Old detailed results may be bounded out,
but their stale generation remains inadmissible and is not called successful or
never-run. The original unknown receipt remains visible after resolution.

## Unreadable public receipt recovery

Malformed, oversized or invalid public preferences do not reset uncertainty.
The panel exposes Inspect public binding recovery independently of the normal
service-review latch. This reads controller status only. A valid concurrently
persisted operation B is recovered as B, never overwritten with an older status
A. Otherwise an explicit confirmation re-reads the exact controller ticket.
If the controller is empty, the user selects the retained payload and confirms
a fresh **existing preview reservation** under the existing service lock; empty
status alone never authorizes a new service action. No new controller operation,
database, daemon, credential or generation-reset path is introduced.

The returned exact binding is saved and read back before public recovery is
reported. The old unreadable state remains identified by byte count, SHA256 of
at most its first256KiB and an explicit unknown-past-outcome classification.
This bounded summary does not claim to retain or hash the complete oversized
bytes. No raw invalid public data is displayed or treated as authority.
Public replacement checks the inspected stored bytes before writing; these are
ordinary cooperative local-receipt checks, not a hostile same-UID atomicity claim.

The recovered/new reservation remains outstanding. The existing explicit
matching-result/late-arrival-fence action is still required, followed by a fresh
preview and NEW service consent only if current lifecycle debt is clear. Failed
local persistence retains the verified ticket in the frame so exact resolution
and another durable save remain reachable. Busy/corrupt/changed authority or
readback failure never unlocks consent. Current protected high-water state is
never cleared. A shared Go-empty-wire fixture is checked against the actual
controller and consumed by native recovery tests; it is authored, not execution
evidence until separately qualified.

## Finite inert truth matrix

| Case | Required truth |
| --- | --- |
| Old A/new B | Status A never clears B or its receipt |
| Empty journal, original before lock | Fence commits first; late original has zero effects |
| Running holder | Resolver reports busy; no false safe absence |
| Completed reply lost | Matching result, zero second execution |
| Preparation before phase journal | Nonce-bound retained recovery, never never-started |
| Fence reply lost | Re-read fence, no new effect |
| Fence fsync/readback/namespace failure | No safe retry until verified reconciliation |
| Delayed preview | Superseded ticket cannot revive |
| Changed binding | Plan/verb/transaction/restore/schema mismatch refuses before effect |
| Concurrent same ID | One effect path; duplicate busy or recorded result |
| Agent changes instance | Existing fresh-plan check refuses; agent authority unchanged |
| Cancel/remount/restart | Retain exact unknown binding; explicit resolver remains reachable |
| Old result evicted | Future fenced, past detail unknown; no invented completion |

Private-positive fixtures establish0700, negative fixtures establish0755.
Future qualification uses child umask022 with explicitly private evidence roots.
Authored fixtures are not execution evidence.

## In-app installation recovery

Only the normal exact running release's PKG may repair the protected helper.
The existing updater client selects the complete published pair; public asset
SHA256, exact canonical URL/size, Installer signature, Gatekeeper/notarization,
normal PackageInfo/one-app namespace, signed app/helper equality and actual helper
capability are verified before offering Installer. An old package such as136 is
not offered merely because it is present. No DMG drag-only route or unsigned
bundle/PATH root fallback substitutes for the system helper. The app confirms,
revalidates and opens normal macOS Installer, then rechecks the installed pair;
it never calls installer/root/service operations itself during source testing.

Preparation and final verification use the same instance-local one-shot bounded
preflight as the privileged bridge. The Installer signed-app identity and file
qualification run in an owned detached read-only worker with cancellation
propagation; an uncooperative completion is ignored after the deadline rather
than joined by a structured timeout scope. CLI submission and Installer opening
recheck cancellation, current generation, exact captured selection and deadline.
The UI actor remains available for pending status and Cancel Request. No actual
Security/Installer/root operation is exercised by inert stall fixtures.
