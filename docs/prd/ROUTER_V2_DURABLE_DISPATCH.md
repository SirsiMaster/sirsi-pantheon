# PRD — Router v2: Durable Dispatch

**Owner thread:** gemma (decompose + draft) → repo thread implements → claude-home binds
**Status:** IN PROGRESS (2026-06-29) — Phase 1 built (2026-07-02); Phase 2 contract BOUND (2026-07-04, codex APPROVE — §2b); Phase 2 BUILT (2026-07-04); Phase 3 BUILT (2026-07-05 — one `internal/dispatch` facade wired into CLI + MCP; worker re-arm now gates on the owner-verified end-to-end §2b bar + Phase-4 migration stance)
**Supersedes the bootstrap:** the file-based markdown pull-model router (still source of truth until cutover)
**Governs:** PANTHEON_RULES.md A26 (Idea Router Workstream), A27 (Heartbeat Loop)

### Phase checklist (for the next lane)
- [x] **Phase 1 — SQLite state store** (`internal/routerstore`): store (user_version-numbered migrations), item CRUD (atomic status-guarded close), agents, state, `Backfill`, `ExportMarkdown(dir)` (byte-fidelity recovery path, round-trip tested), `work.Item` field-fidelity enforced by reflection test (incl. wake_* delivery truth), `-race` tests, 85%+ coverage, lint clean. Wired to nothing (zero risk). *Done 2026-07-02; review blockers fixed same day.*
- [x] **Phase 2 — Event-driven dispatch + §2b Dispatch Contract** (`internal/routerstore`: lease.go, facade.go, breaker.go, dispatch.go + migration v2): fenced-lease lifecycle (`ClaimNext`/`RenewLease`/`StartWork`/`Complete`/`Fail`/`Block`, token-checked incl. owner closes; audited `ForceOwner`), send facade (idempotency key + per-sender quotas; over-quota UPDATES a throttle singleton), keyed-singleton escalations (partial unique indexes make duplicates a DB impossibility), circuit breakers per failure domain (one operator item), budgets, counters aggregate, `Wait` real blocking wait (<250ms wake proven by test; 30s safety re-check) + per-agent notify FIFO (`~/.sirsi/notify/<agent>`, no new deps). Acceptance bar: safety tests reproduce BOTH incidents (duplicate-claim race; stuck-item ⇒ one terminal+one escalation; 500-send flood ⇒ quota rows + 2 singletons; restart mid-lease; expired token cannot complete newer-leased work) — all green with `-race`. *Done 2026-07-04. Worker stays OFF until Phase 3 wires every live path through the facade.*
- [x] **Phase 3 — one dispatch facade** (`internal/dispatch`): `sirsi router send/close/pull/show` AND the MCP `router_submit/poll/get/wait` handlers call ONE facade — store-first guarded writes (idempotency/quotas/breakers BEFORE dispatch; no store row, no dispatch) + byte-identical items/*.md audit dual-write (§2b axiom 8); reads stay on the canonical files until Phase-4 cutover. `router_wait` is now a REAL blocking wait over the store (<250ms wake for facade sends; bounded 5s re-check catches legacy file-only writers). The MCP path's pre-ADR-024 fossil (proposals//reviews//decisions/ + state.json inboxes) is retired from the write path; legacy ids stay readable. Cross-path tests prove /goal #3 (CLI-send→MCP-read and MCP-send→CLI-read, same id/file/row). Deliberately held for Phase 4: register/heartbeat (the mature thread registry is not duplicated — Rule 0). *Done 2026-07-05.*
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

## 2b. Phase 2 Dispatch Contract (BINDING — codex-SME APPROVED 2026-07-04)

Provenance: two runaway incidents (2026-07-03: 19,195 agentic sessions, 0 closed;
2026-07-04: 11,564 escalation-item flood) → adversarial design bounce
claude-pantheon ⇄ codex (round 1 relayed via claude-home: FAIL-as-stated;
round 2 direct, session 019f2f6c-207f: **APPROVE**). This section is the
converged contract; Phase 2 implements it verbatim. The claude build-worker
stays OFF until the acceptance bar below passes.

**The law: the routerstore is the ONLY executable dispatch authority.**
Any second path around the lock (file writes, raw `router send`, interactive
pulls, sidecar workers) merely relocates the failure mode.

1. **Lifecycle** — `open → claimed → working → blocked | dead_letter | completed`.
   Terminal states are terminal. "Give up" = `dead_letter` with owner/action
   metadata — never an item left open forever.
2. **Fenced leases** — `ClaimNext` / `RenewLease` / `Complete` / `Fail` /
   `Block` are atomic (BEGIN IMMEDIATE, WAL, busy_timeout) and token-checked:
   claim returns a lease token; EVERY lifecycle mutation rejects missing,
   expired, or mismatched tokens — including owner-session closes.
   `--force-owner` is human-only, audited, and requires an explicit reason.
3. **Claim is the only door to execution** — `sirsi router claim <id>` is the
   sole transition into executable ownership. Pull stays read-only
   (observation ≠ execution). Claim-on-pull rejected (punishes read UX).
4. **One send facade** — `router send` + MCP send route through a single
   facade enforcing an idempotency key `(from, to, type, subject_key,
   source_item_id, time_bucket)` and per-sender quotas. Over-quota UPDATES a
   singleton throttle item; it never appends.
5. **Escalations are keyed singletons** — update-in-place or deduped on
   `(source_item, failure_class)`, bounded (compacted counters + first/last
   seen), never timestamp-keyed new items.
6. **Circuit breakers by failure domain** — per sender, per target, per error
   class, and global; a tripped breaker pauses dispatch and writes ONE bounded
   operator item. N distinct failures ≠ N escalations.
7. **Budgets & backpressure** — max concurrent claims per target (initial: 2),
   max new items per sender per window, max retries per item, max total
   active work.
8. **Files are non-authoritative by definition** (Phase 2 migration stance):
   item content dual-writes to `items/*.md` as the human audit view, but
   store commit is primary — no store row, no dispatch; a stale or mutated
   file cannot change lifecycle; sweep flags inert/orphan files. Full file
   cutover remains Phase 4.
9. **GC + observability** — retention rules for expired leases, attempts,
   throttle buckets, closed items; counters (claims, lease expiries, retries,
   rate-limit drops, dead letters, breaker state) surfaced in `node-status`
   as ONE aggregate. The next incident must be one red number, not 11,564 files.
10. **Executor policy** — the tiered Brain (rules → gemma → claude) is an
    executor policy ABOVE this contract, never a bypass: tier selection happens
    after claim (or in a non-mutating budgeted classifier), and any tier that
    emits items uses the same facade. The gemma FYI-close tier may run pre-Phase-2
    only while bounded + idempotent, and must adopt the facade when it lands.

**Acceptance bar (before any build-worker re-arms):**
- `ClaimNext`/`RenewLease`/`Complete`/`Fail`/`Block` with token fencing, tested.
- Send facade enforces idempotency + quotas before insert.
- Escalation = update-in-place / keyed singleton, tested.
- Breaker pauses dispatch on systemic failure with one operator item.
- File-router executable writes disabled (dual-read-only during migration).
- Safety tests reproduce BOTH incidents and pass: duplicate-claim race;
  stuck-item ⇒ at most one terminal/escalation record; sender flood rejected;
  restart mid-lease; expired worker cannot complete newer-leased work.

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
