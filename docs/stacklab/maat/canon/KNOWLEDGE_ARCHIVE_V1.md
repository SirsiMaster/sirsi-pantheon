# Ma'at canonical knowledge archive — source map

This document traces the first source slice against the retained Seshat/Ma'at
knowledge obligation. It records source structure, not a test, installed-runtime,
coverage, or quality claim. The archive is the only knowledge authority; the
legacy Seshat snapshots are admitted inputs and remain unchanged.

An operator-provided installed-132 observation says `sirsi maat status m1 --json`
returned the full retained history since September 26 (about 1.6 MB / 670k tokens).
That is a bounded-read-model design input, not a benchmark or a retention target:
the archive never deletes or replaces history to shrink a projection. The active
knowledge view is capped at 20 results by default; full item history and its
provenance use explicit cursor-paginated `maat knowledge history <id>` reads.

| Requirement | Implementation path | Boundary / evidence |
|---|---|---|
| Private canonical store and rebuildable full-text index | `internal/maat/knowledge/archive_path_unix.go`, Darwin-only `archive_vfs_darwin.go`, `archive.go`, `archive_search.go` | Fixed `~/.sirsi/maat/knowledge/archive.sqlite3`. Darwin uses a per-archive non-default wrapper VFS registered with modernc SQLite v1.44.0; it wraps `xOpen`, `xDelete`, `xAccess`, `xFullPathname`, and per-file `xClose`, `xSync`, and `xFileControl`. `xOpen` accepts null output-flags pointers via a local slot, validates owner/type/mode/link-count with retained-root `openat`/`fstatat`, verifies `FD_CLOEXEC`, then passes the retained regular-file descriptor path (`/dev/fd/<file-fd>`) to the pinned `unix-posix` VFS, which retains SQLite-compatible inode locking, deferred-close behavior, I/O, and FULL/F_FULLFSYNC file sync. File controls report HAS_MOVED against the retained namespace, allocate temporary names only inside the VFS namespace, refuse proxy-path controls and unknown controls, and forward only descriptor-local operations. Sidecar deletion requires the remembered inode identity; database authority is shared by database inode as well as directory identity to avoid duplicate same-process descriptors releasing POSIX locks. WAL/SHM probes report verified absence while refusing present sidecars and actual WAL opens under DELETE journaling. Successful descriptor retirement is recorded per FD so retries do not close a reused number. The wrapper owns the rooted database/journal/temp namespace and fsyncs the retained knowledge directory on file creation/deletion; it never uses `/dev/fd/<directory-fd>/<leaf>`, changes the global default VFS, or changes process cwd. Other Unix systems retain their prior descriptor-path implementation. The archive uses the persistent DELETE rollback journal and `synchronous=FULL`; macOS additionally enables and verifies `fullfsync`. SQLite's rollback/recovery protocol has an abrupt-process-exit fixture. Durability depends on the filesystem and storage device honoring sync requests; this is not a power-loss test or a hostile same-UID race-atomicity claim. Namespace, inode and sidecar identity are rechecked at open and archive-operation boundaries. Only the child knowledge directory is required private; Windows refuses where equivalent path checks are unavailable. FTS5 is derived from admitted versions. |
| Stable logical identity and complete source versions | `internal/seshat/source_record.go`, `internal/seshat/adapter_claude.go`, `internal/maat/knowledge/archive.go` | Source + instance + native item IDs form identity; content digest is only version/dedup evidence. Versions retain predecessor digest, extraction time, source schema and filter version. Claude traversal passes context through walk/read, reads at most the actual file/source byte budget plus one detection byte, verifies regular-file identity and rejects growth/change rather than promoting truncation to a complete version. |
| Provenance, exact deduplication and source coverage | `internal/maat/knowledge/archive.go`, `archive_search.go` | Immutable version occurrences bind exact source IDs, references and observation time to a revision; detail/history return the requested record's occurrences only from that revision, while exact-content occurrences from other record IDs remain separately attributable. Search groups only exact content digests. Import records and cursors commit with the batch; partial/error runs retain readable rows and never advance the cursor. Search/history/detail pages assemble generation, totals, hits, coverage and record data in one read transaction. |
| Sensitivity admission and projection defense | `internal/maat/knowledge/archive.go`, `internal/maat/knowledge/knowledge.go` | Closed source identifiers and bounded complete records/references/IDs are scanned before persistence. Withheld observations store only an opaque identity digest, approved source label or fixed placeholder, reason code and timestamp; they never rewrite admitted origins or delete admitted versions. A later safe reimport clears withholding and restores the current FTS projection. Detail and index rebuild rescan retained fields. |
| Legacy retained history and compatibility aliases | `internal/maat/knowledge/legacy.go`, `refresh.go`, `cmd/sirsi/seshat.go` | `latest.json` and every `ingestion_*.json` are read through the same admission, keyed by source-file digest for idempotence, and never modified. The hidden Seshat ingest/list/export aliases use Ma'at's canonical archive; ingestion is local-only, does not auto-export, and incomplete refreshes fail nonzero. Prune and legacy dual-store sync refuse. A corrupt archive does not fall back to legacy reads. |
| Search, detail, history, status, retry and recovery | `internal/maat/knowledge/archive_search.go`, `archive.go`, `cmd/sirsi/maatknowledge.go`, `internal/maat/decisions.go` | Bounded generation-bound cursors and consistent read snapshots; stable exact-content search; per-source latest coverage; item detail/history; withheld-reason listing; explicit `refresh --source claude`; safe FTS rebuild; owner decisions are transactionally bound to the reviewed revision+content digest, and new versions reopen current resolution; pending projection debt reconciles into the existing decision journal with stable evidence and lock-protected idempotent append. Schema initialization/readback stays on the already leased, verified sole SQLite connection. Reconciliation returns the normal typed `CommandResult` with exact projected/remaining evidence. |
| Existing native knowledge flow | Knowledge-only sections of `macapp/Sources/SirsiMenubar/MaatCasebookView.swift` | Search pages cap accumulated results at 100; detail loads source versions in explicit 10-item cursor pages capped at 100; source coverage/error recovery, withheld reasons, confirmation-gated owner annotations, and explicit projection-debt retry use the CLI/archive. Projection decoding retains coverage/debt/cursor/state; review captures exact detail revision, digest, state and trimmed note before scheduling, passes both version flags, and blocks duplicate in-flight submissions. Reconciliation accepts the typed `ok` receipt only when `remaining` is zero; a typed `partial` receipt reports its exact nonnegative `projected`/`remaining` counts as incomplete and keeps the explicit retry available. Malformed or contradictory receipts remain errors; partial debt is never presented as completion. `macapp/Tests/SirsiMenubarTests/MaatKnowledgeArchiveTests.swift` contains focused decoder/consent/receipt regressions. No Sagan Casebook/liveness section was changed. |

## Implemented in this slice vs. still open

The Darwin ABI boundary spans `archive_vfs_darwin_abi.go` and the private
`archive_vfs_darwin_copy_cgo.go` shim. It copies only generated pointer-free
structs/scalars, with a 256-byte maximum, and pins each Go-owned copy buffer
for the synchronous call. Go passes its buffer across cgo as a typed pointer;
only the shim converts SQLite's numeric ABI address to a C pointer. This is
needed because modernc SQLite can return addresses of generated Go globals,
not only C allocations. Darwin archive preparation fails closed when cgo is
disabled (`archive_vfs_darwin_nocgo.go`). No `//go:nocheckptr` or race/checkptr
suppression is used. The sentinel regression covers C-owned cells and the
generated `unix-posix` VFS global across forced GC and stack growth; these are
regression fixtures, not qualification results. The C `memcpy` itself is not
race-instrumented, so generated-global reads are restricted to init-stable
configuration/callback slots (not SQLite's mutable registry links), while
mutable SQLite-owned objects retain their existing serialized callback use.

The source now defines the owner-private archive boundary. On Darwin the
per-archive wrapper VFS maps SQLite's logical database, rollback-journal, and
temporary names through retained descriptors, delegating inode locking, data
I/O, and file sync to modernc SQLite v1.44.0's `unix-posix` VFS; it owns safe
`openat`/`unlinkat` operations and retained-directory sync. The implementation
does not use the failed `/dev/fd/<dirfd>/<leaf>` route or weaken SQLite's
persistent DELETE-journal protocol. Other Unix systems retain their existing
descriptor path. SQLite uses DELETE journaling with FULL synchronous commits
(plus macOS `fullfsync`); this relies on the filesystem and device honoring sync
requests. A process-interruption fixture checks SQLite rollback recovery, but
does not simulate power loss. A hostile same-UID process racing an
operation-boundary identity check remains outside the claim.
It also defines one local Claude transcript reader,
legacy snapshot admission, SQLite/FTS search, version/detail/history/status,
withheld-item explanation, explicit refresh, owner-review annotation, and
idempotent retry of its Casebook projection. The fixtures in
`internal/maat/knowledge/archive_test.go`,
`internal/maat/knowledge/archive_path_unix_test.go`,
Darwin-only `internal/maat/knowledge/archive_vfs_darwin_test.go` and
`internal/maat/knowledge/archive_lock_close_darwin_test.go`,
`internal/maat/decisions_test.go`, `cmd/sirsi/maatknowledge_test.go`, and
`internal/seshat/adapter_claude_archive_test.go` are authored but have not been
executed in this work. Existing CLI/MCP/dashboard readers use the same Ma'at
projection; their broader query/detail workflows are not claimed complete.
The cross-process lock-close fixture's exact test inventory is
`TestArchiveClosingOtherHandlePreservesCrossProcessWriteLock` plus its
environment-gated child `TestArchiveLockCloseChildHelper`; the child proves a
specific SQLite BUSY result before the held transaction commits and a successful
fresh-process write/readback afterward. Its owned process and pipe cleanup is
bounded and statically reviewed; execution and race qualification remain pending.

The current Darwin VFS corrective source adds
`TestArchiveDarwinVFSNullOutFlagsAndContainedFileControls`,
`TestArchiveDarwinVFSHasMovedAndWalAccessStayRooted`,
`TestArchiveDarwinVFSRefusesSubstitutedJournalDeletion`,
`TestArchiveDarwinVFSRefusesSecondAuthorityForRenamedDatabaseInode`,
`TestArchiveAuthorityRetirementRetryDoesNotRecloseRetiredDescriptor`, and
`TestArchiveSetCloseOnExecChecksAndPropagatesDescriptorFlags`. They cover null
`xOpen` output flags, contained file controls, HAS_MOVED after leaf
substitution, WAL/SHM absence versus presence, journal identity retention
through deletion, renamed-directory database-inode alias refusal, descriptor
retirement retry, and checked CLOEXEC failure paths. These fixtures are
authored but unexecuted; they do not qualify SQLite behavior or replace the
cross-process lock-close and abrupt-recovery runs.

Gemini/Chrome/Apple Notes and other local adapters, explicit consented online
Workspace ingestion, richer MCP/TUI/dashboard detail, cross-record conflict
grouping/review, and any additional native workflows remain open. Source-version
history and explicit owner resolutions are retained; no semantic conflict
classification or merge is inferred. Projection retry is source-defined but
still awaits the focused native qualification packet.
This slice makes no completeness, performance, classification-accuracy,
or installed-app claim. It adds no model, external service, dependency, or
automatic network reader.

The cumulative source review identified and this corrective child addresses
the reviewed archive integration gaps. The authored Go and Swift regressions
remain unexecuted; they are source intent, not qualification evidence. The
Darwin VFS-specific identity, file-control, sidecar, retirement, and WAL-refusal
fixtures are also authored but unexecuted. Original
adapters and richer CLI/MCP/TUI/dashboard workflows and conflict-resolution
requirements remain open.

Qualification has not been run for the corrective child. A future native run
must materialize a fresh private `GOMODCACHE` from already-present download
archives, verify against the exact `go.sum` with `GOPROXY=off`, and retain the
existing shared cache unchanged; no dependency or lockfile change is implied.
