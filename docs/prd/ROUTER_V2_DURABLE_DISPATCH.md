# PRD — Router v2: Durable Dispatch

**Owner thread:** gemma (decompose + draft) → repo thread implements → claude-home binds
**Status:** IN PROGRESS (2026-06-29) — Phase 1 built (2026-07-02, claude-fable, `feat/router-v2-increment`)
**Supersedes the bootstrap:** the file-based markdown pull-model router (still source of truth until cutover)
**Governs:** PANTHEON_RULES.md A26 (Idea Router Workstream), A27 (Heartbeat Loop)

### Phase checklist (for the next lane)
- [x] **Phase 1 — SQLite state store** (`internal/routerstore`): store (user_version-numbered migrations), item CRUD (atomic status-guarded close), agents, state, `Backfill`, `ExportMarkdown(dir)` (byte-fidelity recovery path, round-trip tested), `work.Item` field-fidelity enforced by reflection test (incl. wake_* delivery truth), `-race` tests, 85%+ coverage, lint clean. Wired to nothing (zero risk). *Done 2026-07-02; review blockers fixed same day.*
- [ ] **Phase 2 — Event-driven dispatch**: one dispatcher signals the addressed agent's wait channel (in-process) + per-agent notify FIFO/socket for cross-process; `router_wait` becomes a real blocking wait (< 250ms wake). *Depends on Phase 1.*
- [ ] **Phase 3 — MCP owns dispatch, CLI is a thin client**: move send/poll/get/close/register/heartbeat behind ONE facade over `internal/routerstore`; `sirsi router *` and the six `router_*` MCP handlers both call it (no duplicated logic). *Depends on Phase 1.*
- [ ] **Phase 4 — Migration + cutover + back-compat**: `sirsi router migrate` (adapt `work.Item` → `routerstore.Item`, call `Backfill`, report count-in==count-out); dual-read window (store first, legacy file fallback with `DEPRECATED` warning); stop writing files after a deprecation window; ADR + README/A26/A27 updates. *Depends on Phases 1–3.*

---

## 0. Why (the problem, in plain English)

The router works, but three things will hurt us as the fleet grows:

→ **Runtime state lives inside git.** Queue items are tracked `.md` files. Every send/close is potential commit-noise and merge-conflict surface. This is literally what polluted PR #95 and forced the #100 cleanup.

→ **There is no real "wake" — everything polls.** `router_wait` (PR #103) is an honest 1-second poll loop in a long-poll costume. N agents each spinning a poll loop is the wrong primitive: wasteful and laggy.

→ **The MCP tools mirror the CLI instead of owning dispatch.** `router_submit/poll/get/list/notify/wait` shadow the `sirsi router` verbs. Two parallel implementations *will* drift.

**End state:** runtime state in SQLite *outside* git → event-driven dispatch (deliver wakes exactly the addressed agent) → MCP server owns dispatch, CLI is a thin client → the pluggable-brain conductor drives it from the open Max session.

## 1. /goal (explicit completion condition)

Router v2 is DONE when ALL hold:
1. A new item addressed to a live, waiting agent wakes it in **< 250ms** (no 1s poll floor), proven by a test.
2. **Zero** router runtime files are tracked in git; `git status` after 100 sends/closes shows a clean tree.
3. `sirsi router send/pull/show/close/status/node-status` and the six MCP `router_*` tools both call **one** internal dispatch package — no duplicated send/close logic (proven by both paths sharing the same functions, verified by test).
4. A one-shot importer migrates every existing `items/*.md` into the store with zero data loss (count-in == count-out, spot-checked bodies match).
5. Full `go build ./... && go vet ./... && go test ./...` green; Ma'at pre-push gate green.
6. Back-compat: legacy file items still readable during a deprecation window (dual-read), with a logged warning, so no in-flight work is stranded at cutover.

## 2. Phases (build sequence — each is independently shippable as its own PR)

### Phase 1 — SQLite state store (foundation)
- **New package** `internal/routerstore` (SQLite via `modernc.org/sqlite`, CGO-free — honors Rule A3 static-binary mandate). DB at `~/.sirsi/router.db` (outside any repo).
- **Schema (as built — mirrors `internal/work.Item` field-for-field, incl. wake-delivery truth):** `items(id TEXT PK, from_agent, to_agent, title, type, status, opened, closed, instructions, result, wake_status, wake_attempted_at, wake_adapter, wake_error)`; index on `(to_agent, status)`; `agents(id, registered_at, last_seen, pid)`; `state(key, value)` for the old state.json keys. Versioned via the SQLite `user_version` pragma + numbered migrations so future column adds work against existing DBs. A reflection test (`TestFieldFidelityWithWorkItem`) fails the build if `work.Item` grows a field the schema doesn't carry.
- **CRUD API:** `Send`, `Inbox(agent)`, `Get(id)`, `CloseItem(id, result)` (atomic `status='open'` guard — concurrent double-close yields exactly one winner), `RegisterAgent`, `Heartbeat`, plus `ExportMarkdown(dir)` for audit/debug — it emits the file router's exact frontmatter+body bytes, round-trip proven (file → `Backfill` → `ExportMarkdown` → byte-identical).
- **Tests:** table-driven CRUD; concurrent-write safety (WAL mode + a single writer goroutine or `sync.Mutex` per Rule A21); inbox filtering.
- **Acceptance:** store passes tests in isolation; nothing wired to it yet (zero risk to live router).

### Phase 2 — Event-driven dispatch (kill the poll loop)
- **One dispatcher** owns delivery. On `Send`, signal the addressed agent's wait channel directly (in-process) and, for cross-process agents, write a single byte to a per-agent notify FIFO/socket under `~/.sirsi/notify/<agent>` (or `fsnotify` on a tiny spool dir — pick one in Phase 2 design; default to FIFO for simplicity, no new deps).
- **`router_wait` becomes a real blocking wait** on that signal with the timeout as a ceiling, not a poll interval. Falls back to a long (e.g. 30s) safety re-check so a missed signal never strands a waiter.
- **Tests:** deliver-wakes-waiter in < 250ms; timeout still returns cleanly; missed-signal fallback still delivers.
- **Acceptance:** `/goal` #1 met against the new store.

### Phase 3 — MCP owns dispatch; CLI becomes thin client
- Move all send/poll/get/close/register/heartbeat logic into `internal/routerstore` (or a `internal/dispatch` facade over it).
- `sirsi router *` cobra commands call the facade directly. The `router_*` MCP handlers call the **same** facade. Delete the duplicated logic.
- **Tests:** one test exercises a send via the CLI facade and reads it back via the MCP handler (and vice-versa) — proves single source of truth (`/goal` #3).
- **Acceptance:** no behavioral regression in existing router/mcp tests.

### Phase 4 — Migration + cutover + back-compat
- **One-shot importer** `sirsi router migrate`: reads every `items/*.md`, inserts into the store, verifies count + spot-checks bodies, reports a diff. Idempotent (skip already-imported ids).
- **Dual-read window:** `Inbox`/`Get` read store first, then fall back to legacy files (logged `DEPRECATED` warning) so in-flight file items aren't stranded.
- Stop *writing* files; keep reading legacy for one deprecation window, then remove.
- Update `.agents/idea-router/README.md` + PANTHEON_RULES A26/A27 references; add an ADR (`docs/adr/ADR-0XX-router-v2-durable-dispatch.md`) capturing the decision + rejected alternatives (Rule A22 triad if it's an architecture doc).
- **Acceptance:** `/goal` #2, #4, #5, #6 all met.

## 3. Key decision points (Rule A22 matrix)

| Question | Options | Recommendation |
|---|---|---|
| Store engine | SQLite (`modernc.org`, CGO-free) vs BoltDB vs flat JSON | **SQLite modernc** — queryable, CGO-free static binary, WAL concurrency |
| Wake mechanism | per-agent FIFO/socket vs fsnotify spool vs keep polling | **FIFO/socket** in-process channel + FIFO for cross-process; no new heavy deps |
| Cutover | hard switch vs dual-read window | **Dual-read window** — never strand in-flight items |
| Single source of truth | CLI calls MCP vs both call a shared internal facade | **Shared internal facade** — no network hop, no drift |

## 4. Risks / guardrails
- DB corruption → WAL + single-writer; `ExportMarkdown` gives a human-readable audit trail and recovery path.
- Concurrency races on injectable fns → Rule A21 (RWMutex accessors).
- Static-binary mandate (A3) → CGO-free driver only.
- Do-no-harm (Rule 14/A12) → each phase ships behind tests with the live file router untouched until Phase 4 cutover; additive-only until then.

## 5. Gemma's job (assigned)
Decompose Phase 1 into a concrete file/function checklist + draft the `internal/routerstore` schema and CRUD signatures + draft the Phase-1 table-driven tests. Gemma drafts (zero-token); the repo thread implements + self-reviews; claude-home binds. Do NOT attempt agentic multi-file builds — produce the spec + code drafts as text artifacts routed back to claude-home.
