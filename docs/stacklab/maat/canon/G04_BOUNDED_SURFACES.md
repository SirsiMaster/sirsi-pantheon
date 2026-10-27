# G04 — bounded history and archive read surfaces

Current source status (2026-10-27): normal merged PR1240 at
`e26e324be107ab541ec8d4d1400392e1ee55fc26` includes the independently reviewed
G04 engine/native corrections. Focused engine normal/race and causal controls,
and native focused tests/control were executed; see the qualification addendum
below. This supersedes the old authored-only status, not the original scope.
G01/G02 learning, skipped interop/accessibility and installed141 acceptance stay
separate; no whole-Ma'at or JEV-relative performance claim is made.

Base: Mercury `5f907f5761a23bc9ad3fecd2158decfea9f1a613`. Mercury's G1–G5
collectors, native views and browser assets remain inherited, without replacing
that branch with an older donor.

## Replaceable boundaries

| Consumer | Inputs and owner | Outputs / boundary |
|---|---|---|
| MCP `maat_casebook` | optional query/kind/status/limit plus scope/evidence/cursor; `internal/mcp/maat_reads.go` validates transport before journal factory | existing root cases/summary/graph plus history and journal_integrity; canonical `casebook.SearchHistory` filters before window and resolves links on the same snapshot |
| Horus GET `/api/maat/casebook` | same bounded optional GET arguments (`q` remains compatible); request context; `internal/dashboard/maat_reads.go`, existing `cmd/sirsi/dashboard.go` producer | explicit partial/no provisional cases, complete empty or typed unavailable/snapshot_changed; GET only |
| Horus classic Ma’at UI | `internal/dashboard/ui/maat-history.js`, mounted/disposed by `pages.go` | search, scope/status, Continue scan, Load older, Retry and Restart; generation + AbortController owns current query/cursor commits; existing System One explanations and prescribed recovery remain text |
| Activity Ma’at TUI | `internal/tui/maat_history.go`, command registry and captured absolute current CLI through `runner.go` | `/` search, `o` scope, `v` status, `n` continue/older/retry, `u` restart; bounded canonical CLI JSON, generation/cancel on leaving; no PATH fallback |
| MCP `maat_knowledge` | existing default search plus mode=detail/history, canonical id/revision, bounded limit/cursor, source/classification/resolution/history filters | canonical sensitivity-filtered `LoadPageContext`, `Detail`, `HistoryPage`; no source parser/cache or ingestion/write/repair action |
| Horus GET `/api/maat/knowledge` | same typed archive read request; existing dashboard producer | default items/total/withheld retained; canonical detail/history and cursor read paths; actual Detail/provenance and History/Load older/Retry controls |

The core journal and archive remain the sole query and privacy authorities.
Transport cursor checks establish typed encoding only; canonical readers validate
snapshot, query, offsets and retained identities. Partial journal responses expose
no provisional cases. Page summary counts are page-local; count_scope labels query,
after-cursor or incomplete scanned candidates. Only total_complete + total is a
whole-query journal total. Archive totals retain their existing query-wide meaning;
source ingestion coverage is separately shown, never inferred from an empty page.
Alternate legacy injected Recent-only transports retain their original root shapes
but explicitly report historical coverage unavailable and cannot continue. They
are not used by the production file-journal producer. MCP's existing handler API
has no caller context: reads have a finite two-second context; this slice does not
claim transport-disconnect cancellation. Dashboard contexts and TUI cancellation
reach the actual canonical read/CLI runner.

## Authored, unrun verification

- `internal/mcp/maat_history_test.go`: real Go-encoded 2,012-row private journals,
  oldest match, >4MiB partial scan without provisional output, exact continuation,
  after-cursor counts, changed query/snapshot refusal and malformed-before-read.
- `internal/dashboard/maat_history_test.go`: same actual HTTP route with 2,012
  canonical records, old review + acceptance link before limit, cancellation and
  typed unavailable/snapshot refusal, archive mode dispatch/refusal.
- `cmd/sirsi/dashboard_maat_history_test.go`: actual configured journal and canonical
  archive producer paths under an explicitly private fixture HOME.
- `internal/mcp/maat_knowledge_reads_test.go`: canonical archive import fixture,
  search/detail/history paging, immutable version content, provenance, sensitivity
  projection and query/generation cursor refusal. No live source reader is used.
- `internal/tui/maat_history_test.go`: actual canonical runner argv, delayed result
  ownership, cancellation, partial wire refusal and absolute-engine refusal.
  Existing TUI resolution assertions remain; their canned wire now includes the
  canonical history fields it previously omitted.
- `internal/dashboard/maat_history_ui_test.cjs`: shipped controller and DOM callbacks,
  late success/failure, dispose/cancel, continuation identity, no provisional cases,
  retained System One explanations and actual Continue/Load older/Retry/Detail/History
  actions. This is an inert browser DOM seam, not native AX or installed UI evidence.
- `internal/dashboard/testdata/maat-g04-negative-controls.json`: exact production-only
  mutations for Main to compile/run against the causal tests. None executed by author.

Positive fixtures explicitly create 0700 private roots and 0600 journals; these
permissions do not depend on a permissive process umask. No user journal or live
archive is used. History pages replace bounded views; they never accumulate an
unbounded client-side history. Existing strict Recent mutation callers are untouched.

Upgrade independently by retaining public root shapes, typed failure vs empty,
canonical filters/privacy and snapshot cursor semantics. Replace presentation or
transport adapters without changing journal/SQLite policy, schema or write authority.
Qualification should run normal/race for affected Go packages and focused dashboard
CLI producers, existing CLI compilation/vet/lint, inert JS controller tests, then
production-only compiled semantic negatives with the original positive source
restored. No source qualification, installed behavior, performance, release, or
full original-scope completion follows from this document.

## Corrective source after Main's 2ee terminal failure

Main's owned run `/private/tmp/pantheon-maat-g04-2ee401-qualification-c8kex8ni`
failed both original 80-continuation MCP tests and the existing TUI return-hint
assertion. Neither test was a single-call query. The cursor advanced, but the
core's text/status `NeedsState` path performed a full relationship scan for
each candidate, including unrelated candidates, before MatchState. MCP did not
lose the cursor. The original two 80-call tests remain byte-identical.

`internal/maat/history_query_batch.go` now groups at most48 candidate references
for a shared canonical relationship pass. Partially collected batches survive
request boundaries instead of flushing a small batch on each work deadline.
Each candidate is still re-read and qualified with its complete linked state
before top-K insertion. Final selected-page relationships are independently
materialized using the existing snapshot-bound path. No query/owner-status
predicate is moved after the window, and no relationship or integrity failure
is treated as absent. Per-call byte/row/time budgets, maximum record/page/cursor
sizes and cancellation remain unchanged. Counts include qualification reads.

This is bounded amortization, not a linear-complexity or empirical performance
claim: full relationship passes still repeat across batches. No new database,
policy owner or persisted authority/index is introduced. The batch reference
arrays and selected links are bounded; the strict cursor encoder remains the
final64KiB bound. Cursor version5 binds the frozen prefix and new phases; older cursors require a
fresh query. MCP/GET transport syntax recognizes the new version and phases.

Authored `TestHistoryQualificationSharesLinkPassBeforeWindow` checks a fixed
charged-record work ceiling and actual late owner-acceptance text qualification
under a smaller row budget. Cancellation/malformed-batch/old-version fixtures
preserve no provisional output. Existing same-snapshot updater/link tests remain
required. The TUI return hint is on its own short line before optional controls;
the bidirectional `m` registry hint now says `view` rather than naming the wrong
destination while already viewing Ma’at.

Core architecture/source review by Astra and Main runtime qualification remain
required. No test, build, lint, service, model or user-data action was run by the
author. The original failed packet and frozen2ee source remain unchanged.

## G04-C3 exact transport membership corrective

The inherited joined-string checks admitted composite argument keys such as
`query kind` / `mode query`, and composite enum strings such as
`conversation|search` / `open|reviewed`. Both transport adapters now build an
exact set of the existing literal keys and use exact switches for the existing
classification/resolution values. Empty/default and supported literal behavior
are preserved; no new source/privacy or archive policy is introduced.

New `internal/mcp/maat_membership_test.go` and
`internal/dashboard/maat_membership_test.go` cover composite, prefix, suffix and
substring keys/values through the actual handlers. Private-home/journal/producer
counters must remain zero for malformed requests. Companion tests retain all
supported enum literals, defaults and HTTP query aliases. These are authored,
unrun fixtures; existing fixture files and original80-call tests are unchanged.
Core batch logic and legacy private-directory mode policy are untouched.
Independent review and Main normal/race qualification are required for C3.

## Independent whole-core fixture-root correction

Main's whole `internal/maat` run exposed legacy test-owned temporary store roots
that were not explicitly0700 under child umask022. This child extracts only the
`privateMemoryStoreRoot` helper, the OpenStore fixture-root substitutions and
`TestStorePrivateFixtureDoesNotRelaxPublicRootRefusal` from donor
`855037c6ca04f33191f7026ad0ac2413c88c0383`. The helper changes permissions only on
its fresh test-owned directory. The new negative requires an explicit0755 caller
root to remain refused and unchanged. No production file/mode policy, skipped
case, test selection or old assertion is changed. G04's existing
`ProjectFailureMemoryPreflight` API calls remain unchanged; donor855's retained
receipt API adaptations are deliberately excluded. Later G02 composition may
deduplicate the identical helper. Full internal/maat normal/race under022 remains
Main-owned qualification; no author execution or legacy failure relabeling.

Literal archive search source correction: see KNOWLEDGE_ARCHIVE_V1.md. The
observed fresh `archive-drift` HTTP503 is caused by exposing raw FTS grammar,
not a cursor or permission failure. Core/HTTP/MCP/Cobra shared-fixture regressions
now require literal punctuation, operator words and complete-empty no-match;
malformed punctuation-only text must refuse before private readers and show
static actionable guidance. Source-only until Main's native/browser execution.

## G04 frozen-prefix continuation

The file-journal history cursor binds a fixed byte length `N`, SHA-256 of
`[0,N)`, the resolved parent descriptor, the no-follow journal leaf descriptor,
and their owner/group/mode/link/object identities. Appends beyond `N` remain
outside that query and do not invalidate it; a fresh query anchors the longer
file. Truncation, a changed prefix, a replaced leaf/parent, symlink or hard-link
substitution, and metadata-authority changes refuse continuation. The strict
mutation readers and their snapshot/repair contracts remain unchanged.

Each pass is a forward stream from byte zero through the same hash tee used by
the bounded phase parser or selected-record capture. Results and cursor state
stay tentative until the full anchored prefix digest and final retained/named
identities verify. A request may run multiple verified phase passes for a small
query, but all passes share the caller context, the original cumulative byte,
row and parser-work budgets, and the overall two-second ceiling. The existing
300 ms default and two-second maximum are parser-work budgets; full-prefix
verification is separately reported as `verification_bytes` and never charged
to `scanned_bytes`/the 4–8 MiB parser byte budget. The two-second overall
verification deadline is capped by any earlier caller deadline. An interrupted
unverified pass cannot publish its tentative phase. A request with prior
verified progress can return that cursor; a verification deadline with no
progress returns a timeout instead of a successful unchanged cursor. Caller
cancellation and digest mismatch remain errors. Physical EOF before the
anchored boundary is latched as snapshot change, even if the retained inode is
restored before the pass drains.

Backward link, candidate-finalization and materialization phases run in a new
forward pass when cumulative request budget remains. The 48-reference batch,
record/page/cursor bounds and atomic materialization allowance remain in force.
The cost is explicitly O(K × H) for K complete prefix passes over H anchored
bytes, plus the initial anchor; `verification_bytes` reports the actual sum.
This is not a linear-time or empirical performance claim, and there is no
arbitrary journal-size cutoff, cache, second database, lock, write or auth
change. A prefix that cannot be fully verified within the two-second deadline
does not advance.

The strict retained-parent and frozen-prefix reader is currently Unix-scoped.
Windows retains the prior bounded snapshot reader, including its append
invalidation and cancellation continuation behavior; this change makes no
Windows frozen-prefix or strict no-follow guarantee.

Authored source-only adversarial coverage includes append-between-phase and
older-page behavior, a fresh query after append, same-inode prefix mutation
with restored mtime, same-UID leaf replacement, symlink/hard-link/truncation
refusal, cancellation, cursor-version refusal and a private 23,563,095-byte
prefix fixture. These fixtures are not run/performance evidence. Main owns
normal/race qualification, focused interleaving/rollback negatives and actual
performance measurements.

### Qualification recorded 2026-10-27

Engine source `6191ba73` passed 46 focused normal and 46 race cases across the
core, casebook, MCP, HTTP and CLI producer. Four compiled negative controls
failed their intended assertions. A private 23,563,095-byte synthetic journal
required 190 calls for two first-page completions in 5,105.7 ms total, with
51.94 ms p95 and 57.61 ms maximum per call. This is a measured fixture result,
not a cold-cache, Windows, installed-UI or comparative product benchmark.

Native source `a39dcd54` passed 150 selected tests, with zero failures and three
explicit skips; actual Go-artifact interoperation and external accessibility
proof remain separate. A compiled private one-read mutation failed all eight
intended continuation assertions without hanging. Native browsing follows only
validated advancing partial cursors, for at most 128 reads or eight seconds per
burst. Cancel and bounded pause retain accepted progress; older pages remain
explicit. Source and raw-log hashes are bound by Main qualification receipt
`3d7e8a1f56700955fc8383699159705c95b5e9ce350ebe7151569ab01158d6ac`.


## 2026-10-27 source qualification and remaining acceptance

- Engine source6191: 46 focused normal and46 race results, four compiled causal
  controls. Receipt SHA256 `34536c093e3f34c809f7e15d19f6a472e1b89b0d51ab79e4a0fa713f8c971c96`.
- Native sourcea39:153 selected results,150 pass and3 explicit skips, zero fail;
  compiled one-read mutation fails the intended assertions. Receipt SHA256
  `3d7e8a1f56700955fc8383699159705c95b5e9ce350ebe7151569ab01158d6ac`.
- Main/independent review verified the normal merged composition preserves the
  accepted G04 source and the separate selected relay fixes. Final141 whole
  composition CI/package/install and actual producer-to-Swift/accessibility proof
  are not supplied by these component receipts.
- Private2012-row workload measured parser/verification separately. It does not
  prove cold-cache behavior, real-journal UI latency or superiority to JEV.

Exact retained local receipts are under the qualification machine's
`~/.sirsi/evidence/pantheon-freeze-corrective-20261023.1rud4wmz/` as
`Main-6191ba73-g04-qualified-engine-and-performance-20261027.json` and
`Main-a39dcd54-g04-native-qualified-20261027.json`. Hashes, not that machine,
identify evidence. Full product acceptance is composed in the PT wing's
`CERTIFICATION.md`; these receipts do not close unrelated original tasks.
