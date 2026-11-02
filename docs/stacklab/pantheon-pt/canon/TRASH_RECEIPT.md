# Review-bound native Empty Trash correction

Status (durable-history qualification at source 7960a495): Go normal and race
runs on exact source 4629f327 passed; all Go blobs are unchanged at 7960a495.
Each run reported 29 top-level tests and 93 passing test events across two
packages (normal 40.016s; race 120.613s). At 7960a495, strict-concurrency Swift
build passed and XCTest reported 18 passes, but the qualification wrapper failed
afterward because its expected XML was missing; the native qualification is
incomplete and independent review is pending. Release qualification and
installed-history verification remain unrun.

Historical browse qualification: b966f2c9 compiled in Main's actual pr42NX
qualification but failed three cursor/coverage tests; race and Swift did not
run. That immutable failure is not acceptance of its stateless design.
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

The explicit guide return uses the same owned `Nav.pop` as BackBar, not
presentation dismissal. The originating Nav frame retains its workflow model
across remount, preserving the explicit-retry fence and discarded consent.
The direct action fixture pushes the actual Trash and guide view values, invokes
the button's return method, verifies exact origin frame identity and no automatic
browse/select/apply. It does not claim installed rendering/interaction proof.

Focused inert author fixtures: `TrashBrowseRecoveryTests` (ten methods), plus
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

## Durable operation history

The apply path keeps Report v1 and its reviewed manifest/confirmation contract.
An apply result may add `operation_id`; preview and failures before durable intent
creation omit it. Go stores owner-local receipts under `~/.sirsi/trash-receipts`
using private, no-follow files. Before the first unlink, it writes and fsyncs the
exact manifest, then fsyncs the receipt directory. Before every unlink it fsyncs
an attempt-start event; after the syscall it fsyncs the known result before
starting another member. A terminal Report is written only after the durable
member events reconcile to its exact reverse-order deleted prefix, counts and
logical-byte total. A writer persistence failure poisons that writer: it cannot
append a success-shaped terminal after a torn or unsynced event.

The append-only operation index supports bounded newest-first pages without a
directory-wide scan. `sirsi anubis empty-trash history --limit N [--cursor TOKEN]`
returns at most20 summaries, scans at most64KiB and at most1,024 bytes per index
row, and returns a device/inode-bound byte cursor. Each operation record is
bounded to1MiB. `sirsi anubis empty-trash history --operation-id ID` returns the
exact reviewed manifest, one state per manifest entry, and the validated
terminal Report when present. The index contains only operation identity, start
time and manifest digest; the per-operation journal contains the reviewed paths
and results. No history command constructs approval, replays deletion, or
changes the Trash contents.

The operation byte budget derives from the 64 KiB manifest limit, 1,024 maximum
entry-attempt pairs, bounded event/error fields, and the terminal deleted-path
prefix. Every append checks the 1 MiB cap; an attempt-start that cannot fit
fails before its unlink. Index appenders hold the verified descriptor and the
nonblocking exclusive lock while checking the final byte. If an earlier crash
left an unterminated row, they append and fsync a newline boundary before the
next row; they never truncate the old bytes. A failed boundary write or sync
stops the current apply before any unlink.

Existing index and operation files are opened nonblocking and rejected unless
they are private regular single-link files; substituted FIFOs, symlinks and
hardlinks are surfaced as incomplete/unknown evidence without changing them.
Index writers take a nonblocking exclusive lock. Contention aborts before any
Trash unlink and returns a visible retry error.

If an attempt-start is durable without a durable syscall result, that member is
`outcome_unknown`. A missing terminal is `not_finalized`; the record does not
claim whether a process is live or interrupted. Corrupt records remain visible
as `unknown` with bounded integrity issues, while readable neighboring rows can
still be paged. There is no automatic pruning or silent record cap. The native
Empty Trash screen can reopen the read-only history and show the reviewed
inventory, known per-entry outcomes, exact terminal report and errors. History
does not restore deleted bytes: Empty Trash remains permanent and has no undo.
Receipt fsync does not fsync the Trash namespace, prove power-loss durability of
an unlink, or measure physical bytes reclaimed. Cooperative-owner stat/unlink
limitations remain unchanged.

The invocation consumes approval before its first await. Double taps, .task,
page changes, Retry and Refresh do not replay deletion. Read-only page/selection
changes invalidate old consent. A failed remaining-items read preserves the
original operation receipt and deleted prefix. UI branding/layout remains the
incumbent black/emerald/gold native surface, with explicit loading/error/empty/
browsing/ready/applying/result states.

## Qualification status and remaining browse handoff

### Durable-history qualification at source 7960a495

The Go results above are pinned to exact source 4629f327; every Go source and
test blob is identical at 7960a495. Normal completed in 40.016s and race in
120.613s; each reported 29 top-level tests and 93 passing test events across two
packages. This qualifies only the executed Go package inventory.

For exact source 7960a495, the strict-concurrency Swift build passed and the
test output reported 18 XCTest passes. The enclosing qualification wrapper then
failed because its expected XML output was missing. Treat that native attempt as
incomplete, not accepted; independent review is pending. Release qualification
and installed Empty Trash history verification remain unrun.

### Remaining browse qualification handoff

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
