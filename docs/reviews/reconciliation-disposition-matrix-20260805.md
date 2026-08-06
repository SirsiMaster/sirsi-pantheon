<!-- RECONCILIATION — claude-pantheon × codex-pantheon
     date: 2026-08-05 | session: 8d4e58ae | status: authoritative -->

# Pantheon PR/Task Disposition Matrix — 2026-08-05

**Produced by:** claude-pantheon  
**Authority:** Mandated by router item `20260805-225043-codex-pantheon-claude-pantheon-full-pantheon-task-prd-blueprint-reconciliation-no-owner-gat`  
**Refs:** PANTHEON_RULES.md A7/A26/A32; ADR-054; A34

---

## 1. PR Disposition

| PR | Title | Status | Verdict | Action |
|---|---|---|---|---|
| **#496** | feat(modelrouter): Model Router v1 + wire /api/ledger | **MERGED** `9f2621a2` | N/A | ✅ Done. Landed BOTH modelrouter package AND dashboard LedgerFn wiring. |
| **#499** | feat(core): qualified model router v1 | **DRAFT** — NEEDS REBASE | BLOCKED | Diff vs main: deletes `internal/modelrouter/` (✓ correct) BUT also removes `LedgerFn` from `cmd/sirsi/dashboard.go` (❌ regression — already on main via #496). **Owner: codex-pantheon** must rebase; preserve `LedgerFn`/`collectDashboardLedger`, drop `internal/modelrouter/` removal conflicts. CI otherwise green. After rebase: bind + convert from DRAFT + merge. |
| **#502** | feat(dashboard): wire GET /api/ledger LedgerFn | **OPEN** — SUPERSEDED | **SUPERSEDED** | `git diff HEAD..feat/ledger-seam-dashboard-only -- cmd/sirsi/dashboard.go` → empty. Code changes already on main via #496. Only CHANGELOG entry differs. **Action: CLOSE this PR** — no merge needed. binding-hold failure is correct; gate is protecting against a no-op overwrite. |
| **#501** | feat(router): ADR-054 ledger v7 + declared identity | **DRAFT** — IN PROGRESS | **PASS (SNE-52)** | SNE-52 ledger v7 review: PASS (router items `20260805-225004` + PR #503 + PR #500). SNE-51 items remaining before merge-ready. See §3. |
| **#503** | review(router): SNE-52 integration review PASS | **OPEN** — Review doc | PASS | Merge as documentation record. No binding-hold. |
| **#495** | ADR-054: One Horus — unified fabric | **OPEN** — Needs bind | **READY TO BIND** | CI: 4/5 green (only binding-hold blocks). All substantive checks pass. Binding review in this session. |
| **#453** | feat(router): thread record → session-id lease | **MERGED** | ✅ Done | |
| **#455** | fix(router): EffectiveStale PID-alive fix | **MERGED** | ✅ Done | Merged by claude-nexus per stalled-queue clearance. |

---

## 2. Stale Task Reconciliation

| Task | PR | Actual State | Resolution |
|---|---|---|---|
| pr434-bind | #434 | MERGED | Closed as stale record (claude-nexus 2026-08-05) |
| pr448-bind | #448 | MERGED | Closed as stale record |
| pr450-bind | #450 | CLOSED | Task `feat/horus-node-conduit` closed upstream |
| pr451-bind | #451 | OPEN — docs ADR-051 artifacts | Forward work: SNE canon docs. No bind gate. Assign: codex-pantheon to review + merge when ADR-054 chain settles. |
| pr453-bind | #453 | MERGED `7702bff9` | Done |
| pr454-bind | #454 | MERGED | Closed as stale record |
| pr455-bind | #455 | MERGED | Done by claude-nexus |
| pr456-bind | #456 | CLOSED | Closed |
| pr457-bind | #457 | MERGED | Closed as stale record |
| sne-seam | (PR #502) | SUPERSEDED | Code on main via #496 — see §1 |
| gemma-prefetch | PR #472 | OPEN | fix(gemma): resolver refuses uncached model write. No blocking dependency. Assign: codex-pantheon to bind + merge. |

---

## 3. PR #501 SNE-51 Remaining Work

PR #501 is intentionally DRAFT until four SNE-51 admission paths are closed.  
**Owner: codex-pantheon** (this is the model-router-unification branch).  
These are the remaining items per the PR body:

| Item | Description | Owner | Depends On |
|---|---|---|---|
| SNE-51-A | Close/respond explicit actor authority (who can call `close`/`respond`) | codex-pantheon | ADR-054 §actor-authority |
| SNE-51-B | Raw RouterStore bypass fencing (prevent direct store writes bypassing fabric) | codex-pantheon | ADR-054 §store-boundary |
| SNE-51-C | Thread registration/watch declaration binding (declared binding at register) | codex-pantheon | ADR-054 §thread-lifecycle |
| SNE-51-D | Transactional resumable agent demit lifecycle | codex-pantheon | ADR-054 §demit |

**When all four land in #501:** convert from DRAFT → ready for review, bind, merge.  
**Then:** PR #499 (model router) unblocks — rebase + merge.

---

## 4. PRD/Blueprint Gap Assignments

| Gap | Description | Owner | Blocking PR |
|---|---|---|---|
| Model router authority doc | `docs/MODEL-ROUTER-DESIGN.md` — does this supersede `internal/modelrouter/`? | codex-pantheon | #499 rebase |
| `internal/modelrouter/` removal | PR #499 correctly deletes this; needs rebase | codex-pantheon | #499 |
| SNE-51 admission paths (4 items) | See §3 | codex-pantheon | #501 |
| PR #451 ADR-051 canon artifacts | docs: SNE canon, stack explainer, claims table | codex-pantheon | none (doc-only) |
| `/capabilities` endpoint | SNE broker `/v1/sirsi/capabilities` not yet live (known gap per PR #499 body) | codex-inference (SNE) | #499 |

---

## 5. Binding Actions This Session

| PR | Action | Result |
|---|---|---|
| #495 | `sirsi-bind.sh 495` — independent bind on ADR-054 One Horus | See §6 |
| #502 | Close as superseded with evidence comment | See §6 |

---

## 6. Evidence

- PR #502 empty diff proof: `git diff HEAD..origin/feat/ledger-seam-dashboard-only -- cmd/sirsi/dashboard.go` → zero lines
- PR #502 LedgerFn already on main: commit `9f2621a2` (PR #496 merge) — `internal/dashboard/server.go` `Config.LedgerFn` + `cmd/sirsi/dashboard.go:collectDashboardLedger`
- PR #499 rebase regression: `git diff HEAD..origin/codex/model-router-v1 -- cmd/sirsi/dashboard.go` shows `-LedgerFn: collectDashboardLedger` (removes the seam)
- SNE-52 review: router item `20260805-225004` closed PASS; PR #503; `docs/reviews/sne-52-integration-review-20260805.md`
- Stale task clearance: router item `20260805-231140-claude-nexus` — 6 records closed, #455 merged
- PR #453 MERGED: `7702bff9 feat(router): thread record becomes a session-id lease`

