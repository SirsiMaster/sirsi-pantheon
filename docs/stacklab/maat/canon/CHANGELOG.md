# Changelog — Ma'at canon

## 2026-10-24

- Corrected the knowledge archive's crash-recovery path: SQLite now anchors its persistent DELETE rollback journal beneath the retained private-directory descriptor, uses and verifies FULL synchronous commits (plus macOS fullfsync), and includes an authored abrupt-process-recovery fixture. Filesystem/device sync behavior remains an explicit durability boundary; qualification has not been run.
- Added a source-bound first vertical slice for Ma'at's canonical private knowledge archive: stable source identities, immutable versions/provenance, sensitivity-filtered SQLite/FTS5, legacy snapshot admission, bounded search/detail/history/coverage, and the existing native knowledge route. Remaining adapters and surfaces stay explicitly open; no tests or installed-runtime claim are implied by this entry.
- Corrected five cumulative archive integration gaps: schema setup now uses the already verified sole SQLite connection; same-record historical provenance remains revision-bound; native knowledge projections decode coverage/cursors and exact content digests; owner review captures and submits the displayed revision/digest once; and projection reconciliation emits the typed CLI receipt the native surface consumes. Added bounded open, same-content provenance, native decoding/consent, and receipt regressions. Source-only; qualification remains pending.
- Replaced Darwin's unsupported `/dev/fd/<dirfd>/<leaf>` SQLite route with a per-archive non-default wrapper VFS over modernc SQLite v1.44.0 `unix-posix`. Safe retained-root opens, unlink/access/full-path checks, descriptor-shared authority retirement, rooted temporary files, and retained-directory sync preserve SQLite's DELETE-journal/FULL-sync and native inode-locking path; no default VFS or cwd mutation. Added Darwin-only shared-handle, temporary-file, and WAL-refusal fixtures. Source-only and unqualified; the new VFS and fixtures require Main's native normal/race/recovery qualification.
- Added a Darwin cross-process regression for closing one archive handle while another holds a write transaction: an already-open child must receive SQLite BUSY (5), then a fresh child must commit and the parent must read back the committed value. Test inventory: `TestArchiveClosingOtherHandlePreservesCrossProcessWriteLock` and `TestArchiveLockCloseChildHelper`; bounded child/parent contexts and deferred pipe/process/transaction cleanup were statically reviewed. The fixture is unexecuted and does not qualify the VFS.
- Corrected the Darwin archive VFS source findings: nullable `xOpen` output flags, rooted/refused SQLite file controls, retained journal identity through unlink, retry-safe per-descriptor retirement, database-inode alias authority, checked CLOEXEC, and absent-only WAL/SHM access probes. Added focused Darwin regression fixtures and updated the source/recipe map. Source-only and unqualified; Main's native normal/race/recovery packet remains required.
- Clarified the existing credential-preflight boundary: additional same-Team identity types are neutral metadata, only missing kinds from a completed observation receive recovery hints, incomplete output does not diagnose absence, and the native surfaces label notarization as not inspected while preserving the blocked aggregate gate.

## 2026-10-05

- Cataloged native Activity resolution: retained outcomes are classified as completed, review-required, or evidence-required, and every non-completed state has a Ma'at guided-review route while preserving the exact command as technical evidence.

## 2026-09-27

- Added the complete Stack Lab nine-record canon bundle.
- Bound the bundle to the existing Ma'at catalog, recipe, wing schema, and ADR sources.
