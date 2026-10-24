# Review-bound native Empty Trash correction

Status: current child is source-only; its tests, build, race, independent review
and installed UX qualification are unrun. b966f2c9 compiled in Main's actual
pr42NX qualification but failed three cursor/coverage tests; race and Swift did
not run. That immutable failure is not acceptance of its stateless design.
Main's subsequent 0701770c normal/race Go runs passed and Swift built, but one
fixture failed because APFS refused creation of an invalid UTF-8 name. Its
minimal child tests inert raw bytes through the exact production name decoder;
the real filesystem fixture does not claim to have created that invalid leaf.
Full-scope publication remains pending. The earlier
86a96a24/b8815fdd candidates remain immutable ancestors; this child corrects their
whole-inventory limit and 1MiB versus runner-64KiB input mismatch. It does not
reopen the completed PR470 source-publication obligation.

## Browse, select, review, confirm

The module contract is `contracts/stacklab/anubis-trash-receipt-v1.json`
(version4 source candidate). Existing platform.EmptyTrash and legacy text CLI
callers are unchanged. The native path never falls back to unrestricted --yes.

1. Native `SirsiEngine.trashList` delegates read-only v3 browsing to a view-owned
   controller and one serialized non-main I/O actor. It retains the SAME public
   Darwin fdopendir/readdir stream plus no-follow root/ancestor descriptors
   across Next; it never reconstructs a stream from an exported seek cookie.
   A page makes at most34 readdir API calls, yields at most32 eligible/blocked
   rows, consumes at most34*1024 name bytes (including NUL/dot entries), and
   performs at most32 leaf stats plus bounded root/ancestor checks. Work reports
   actual readdir calls/name bytes, NOT underlying kernel syscall counts or
   libc buffer sizes. Only the returned chunk is sorted; no lookahead/index or
   EOF rescan. Union-directory libc read-all mode is refused before fdopendir.
   More and Next are explicit; the final empty EOF page is read-only.
   Unselectable names remain explicit blocked rows with reason codes, never
   selected identities. Stat errors, unsupported types, foreign owners/devices
   do not block eligible siblings or continuation. An all-blocked page is not
   an empty folder. Root/container trust failure still refuses the whole page.
   Continuation binds an in-process session UUID, exact next sequence, directory,
   root and folder generation. Replay/change refuses and closes; it is never
   deletion consent. Refresh/navigation/dismiss/cancel/60-second read or idle expiry
   immediately fence publication on MainActor and coalesce close on the SAME
   serial I/O owner. Only an acknowledged close is marked closed. Before apply,
   the invocation latch is set and owned close is awaited, then launch consent
   is rechecked. A per-read locked cancellation fence prevents stale queued actor
   messages from reopening after close or closing a newer session; no message
   ordering assumption is used. A stalled kernel call does not run on MainActor, but can delay
   close acknowledgement; no hard wall-clock cleanup guarantee is claimed.
   No daemon, child runner, token registry, private libc symbol or cgo is needed.
   The separate `anubis empty-trash browse` CLI now explicitly refuses rather
   than advertise a stateless cursor. Go select/apply remain sole deletion
   authority and freshly validate native metadata without trusting its session.
2. Select manageable entries on that page. Open a large folder to browse its
   descendants. `anubis empty-trash select` is read-only: it captures the full
   recursive tree of explicitly selected directories only if it fits a batch.
   If it cannot fit, choose fewer entries or descend instead. Ancestors are
   recorded as nondeleting containers, not silently added deletion candidates.
3. Read the complete selected manifest and retained-container list. The surface
   freezes that exact manifest and argv/stdin when confirmation opens. An
   approved manifest and every other actual Swift-encoded request must fit the
   unchanged runCaptured64KiB stdin ceiling. Go also reads at most64KiB+1 and
   requires whole-input EOF. There is no 1MiB exception or chunked apply bypass.
4. `anubis empty-trash apply --confirm` consumes only that selected manifest.
   It validates strict exact key sets, rejects aliases/duplicates/unknown/missing
   fields and invalid relative paths/types, and preflights the complete SELECTED
   scope before any delete attempt. Cursor fields cannot decode as a manifest.
5. After completed/partial/failed/unknown outcome, Review next batch / remaining
   items performs only a fresh read. Select, review and explicitly confirm again
   for any subsequent batch; no automatic delete continuation or replay occurs.

## Bounds and filesystem contract

A deletion batch has at most1024 reviewed members,64 retained nested directories
(including containers), depth24, and64KiB serialized input. These are per-batch
limits, not a maximum total Trash inventory or total logical byte size. Native
browse chunks can continue beyond those inventory counts; select at most32
eligible roots per batch. A single oversize
directory is not a retry-only dead end: keep it as a container and select its
descendants. Extremely deep/long paths or unsupported entries can still refuse.

Root and each relative component retain no-follow dev/inode/UID/type/mode/size/
mtime/ctime observations; effective UID must own every admitted object.
Only regular files, directories and symlink leaves are supported. Symlink
targets are never traversed. The fixed owner home/.Trash has no caller override.

Apply retains root and ancestor descriptors. Container/root stable dev/inode/
UID/type/mode must match; unrelated sibling additions or timestamp changes do
not require a global inventory match. Selected files/symlinks match full metadata.
A selected whole directory is fully recaptured and compared, including its
reviewed descendants and metadata, before ANY deletion. Selected drift returns
review_required with attempted0 and deleted[].

Execution uses only the approved sorted list in reverse lexical order,
nonrecursive unlink for files/symlinks and rmdir for selected directories.
Every visible root/ancestor binding and reviewed leaf is rechecked before its
syscall. No new candidate is discovered while deleting. Directory deletion uses
stable identity because our own child deletions change directory timestamps.
A new nested item survives; nonempty rmdir returns partial with the confirmed
successful prefix. Containers and unrelated siblings are never deleted.

Completed means this reviewed batch completed, not that Trash is now empty.
Logical bytes are advisory, not measured physical disk reclamation. Protection
is for an ordinary cooperative Mac owner: stat/unlink is not atomic against a
hostile same-UID writer. No stronger filesystem or process-group cleanup claim.

## Native result and recovery

### Native errno recovery correction (source candidate)

`TrashBrowseFailure.swift` captures errors immediately inside the per-instance
open/openat/fdopendir boundary, before close or formatting. The separately
versioned native-only `pantheon.trash-browse-error.v1` records a closed failure
code, captured POSIX errno when available, stage and bounded message. Logical
identity/generation refusals never invent errno. Existing failure callers still
receive an incomplete CommandExecution; the workflow decodes only its nonzero
native failure envelope, never as page/empty/selection evidence. Unknown or
incomplete envelopes cannot offer permission recovery.

Only EACCES/EPERM offer the existing macOS access guide. Permission/privacy may
be involved; neither errno proves missing FDA. ELOOP/ENOTDIR retain no-follow
safety refusals; ENOENT is unavailable, not empty; EMFILE/ENFILE are resource
failures. No fallback root, link following, Trash creation, grant, automatic
probe, permission write or app/service restart is introduced. The guide uses
the current bundle and explicit Settings/reveal buttons; entry performs neither.
Its reused Settings button no longer invokes the registration probe.

Guide entry fences/closes the existing session and discards page continuation,
selection, preview and approval. Back does not retry automatically: the owner
explicitly starts a fresh browse. Success clears current failure; delayed older
or cancelled failure cannot overwrite a newer result. Deletion still requires
new selection, canonical Go preview and irreversible confirmation. Retained
operation receipts survive failed access retries. Installed cause and native
Settings/return/retry behavior remain unqualified until separately observed.

Focused inert author fixtures: `TrashBrowseRecoveryTests` (nine methods), plus
unchanged `TrashBrowseSessionTests`, `TrashReceiptTests` and
`RemediationExecutionTests`. Qualify strict-concurrency release compilation and
those exact XCTest inventories under Main's ordinary admission. No fixtures,
builds, real Trash, Settings or permission actions were executed by the author.

Existing runCaptured remains unchanged:64KiB stdin, shared4MiB stdout/stderr,
monotonic deadline, cancellation and explicit unconfirmed child-exit debt.
Only complete captures are decoded. Success-shaped reports require exit0;
nonzero partial/failed reports remain evidence. The returned deletion prefix
must match the frozen selected manifest. Malformed, timeout, cancelled,
overflowed or unconfirmed output never becomes success/empty.

The invocation consumes approval before its first await. Double taps, .task,
page changes, Retry and Refresh do not replay deletion. Read-only page/selection
changes invalidate old consent. A failed remaining-items read preserves the
original operation receipt and deleted prefix. UI branding/layout remains the
incumbent black/emerald/gold native surface, with explicit loading/error/empty/
browsing/ready/applying/result states.

## Qualification handoff (not executed by author)

- Go: `./internal/trashreceipt`, selector `^TestTrash(Receipt|Browse|Oversized|Selected|Selection)`.
  Includes >1024 paged inventory, oversized-folder descendant selection,
  preserved unselected siblings, selected drift, stale cursors, aliases,
  symlink/identity substitution, late additions and partial prefix.
- Go: `^TestDarwinDirectoryPositionAcrossFreshOpens$` now requires explicit
  refusal/no apparent data for fresh-open offsets, including zero. Restoring
  b966's apparent success must fail. Its original exact-coverage failure remains
  in immutable b966 and pr42NX evidence; it is not skipped or relabeled passing.
  Go selected-scope fixtures retain a local stream, not a public fake cookie. Run
  `^TestTrashBrowse(MixedBlockedRowsDoNotPoisonSiblings|AllBlockedPageAdvancesToLaterEligible)$`.
  These current child fixtures are unrun.
- Swift: `TrashBrowseSessionTests` MUST prove retained-stream exact coverage,
  all-blocked to later eligible, real work bounds, invalid names/stat/special
  entries, root/parent/leaf substitution, generation/session/sequence refusal,
  exact owned descriptor release, refresh/expiry/cancel/dismiss, close before
  apply, and responsive MainActor/publication fencing during held inert I/O.
  `TrashReceiptTests` plus existing `RemediationExecutionTests` for
  owned inert process-capture qualification. Actual Swift JSONEncoder byte
  length must reject an otherwise valid manifest over64KiB.
- Semantic overlays: omit selected full-preflight equality; recursively remove
  a selected directory; delete container ancestors; admit a stale cursor; omit
  the Swift pre-await latch or encoded-byte check; accept incomplete/nonzero
  success-shaped capture. Corresponding owned fixtures must fail semantically.
  For this child, reopen/rewind the stream on Next, publish a stale async read,
  fail a page on any blocked leaf, or move I/O onto MainActor: corresponding
  exact-coverage/blocked/lifecycle/responsiveness fixtures must fail.
- Main owns ordinary admitted normal/race/Swift/negative/vet qualification.
  There is no extra multi-sample admission gate. No tests, builds, real Trash,
  privilege, service or model operation was performed by the source author.
