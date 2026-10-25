# G04 — bounded history and archive read surfaces

Source candidate only. Main owns compilation, runtime, causal negatives and
independent review. The original Ma’at scope is unchanged. This correction does
not close G01/G02 learning or the other original-scope audit gaps.

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
final64KiB bound. Cursor version4 binds the new phases; older cursors require a
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
