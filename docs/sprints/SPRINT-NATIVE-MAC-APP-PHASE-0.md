# Sprint Plan — Native Mac App, Phase 0 (Foundation)

**Sprint ID:** native-mac-app-phase-0
**Workstream:** pantheon-mac-native-cli-pivot
**ADR:** [ADR-018 — Native macOS App + CLI as Pantheon's Interactive Surfaces](../ADR-018-NATIVE-MAC-APP.md)
**Owner agent:** claude-pantheon (repo-scoped per Rule A26)
**Reviewer agent:** codex-pantheon
**User authorization required before:** any scaffold code under `cmd/sirsi-app/`, any TUI deletion, any change to `cmd/sirsi-menubar/` user-visible surfaces.

> **Per Rule 17 (Sprint Planning is Mandatory) and ADR-018 conditions: no code in this sprint. Phase 0 is foundation-only.**

## /goal

Phase 0 is complete when **all** of the following are true:

1. `docs/ADR-018-NATIVE-MAC-APP.md` exists, is committed, and is **user-approved** (router decision file with `status: approved-by-user`).
2. `docs/ADR-INDEX.md` is updated: ADR-018 listed; ADR-016 marked superseded.
3. The TUI is hidden from default entry points: the no-arg `sirsi` invocation does not launch the broken TUI by default in any binary built for v0.23.
4. `docs/CLI_COMPATIBILITY.md` lists every `sirsi` verb against macOS / Linux / Windows status — no implied Windows parity.
5. `internal/output/tui.go` carries a deprecation notice header referencing ADR-018 and the Mac-app replacement plan.
6. A router decision file at `.agents/idea-router/decisions/` records the Phase-0 completion and hands Phase 1 (reuse audit) back to user for go/no-go.

## Scope (this Phase 0 only)

**In scope**
- ADR-018 authoring and commit (done in this session, pending user approval).
- ADR-INDEX update.
- TUI default-entry-point change (small CLI flag-level edit in `cmd/sirsi/`, **not** TUI internals).
- Deprecation notice header in `internal/output/tui.go` (comment block only, no code change).
- `docs/CLI_COMPATIBILITY.md` authored from current verb registry — honest status per verb.
- Router decision file and pending item for user authorization.

**Out of scope (deferred to Phase 1+)**
- Any new file under `cmd/sirsi-app/`.
- Any Swift code.
- Any change to `cmd/sirsi-menubar/` user-visible surfaces.
- Reuse audit of `ios/Pantheon/` (Phase 1).
- Read-only inspection of `/Applications/Mole.app` bundle (Phase 1).
- Sparkle/codesigning research (Phase 2).
- Deletion of any `internal/output/tui*.go` file (deferred to Mac app v1.0).

## Tasks (Phase 0, ordered)

| # | Task | Files touched | Type | Gate |
|---|------|---------------|------|------|
| 1 | Author ADR-018 | `docs/ADR-018-NATIVE-MAC-APP.md` | docs | ✅ done in this work item |
| 2 | This sprint plan | `docs/sprints/SPRINT-NATIVE-MAC-APP-PHASE-0.md` | docs | ✅ done in this work item |
| 3 | Update ADR-INDEX (add ADR-018, mark ADR-016 superseded, bump "Next available" to 019) | `docs/ADR-INDEX.md` | docs | done in this work item |
| 4 | Author `docs/CLI_COMPATIBILITY.md` from current verb registry | `docs/CLI_COMPATIBILITY.md` | docs | Phase-0 commit |
| 5 | Add deprecation notice header to `internal/output/tui.go` | `internal/output/tui.go` (comment only) | docs-in-code | Phase-0 commit |
| 6 | Hide TUI from default `sirsi` entry: when invoked with no args, print helpful `sirsi --help` summary instead of launching TUI; preserve TUI behind `--experimental-tui` flag for internal salvage | `cmd/sirsi/main.go` or equivalent root cmd | code (minimal) | **user approval before commit** |
| 7 | Router decision file: `decisions/20260521-claude-pantheon-mac-native-cli-pivot-phase-0.md` with `status: ready-for-user` | `.agents/idea-router/decisions/` | router | Phase-0 commit |
| 8 | Update `state.json`: close `claude-pantheon` pending item; add `pending_for_user` flag for ADR-018 approval | `.agents/idea-router/state.json` | router | Phase-0 commit |

> Task 6 is the **only** code change in Phase 0 and is gated on explicit user approval before commit. All other tasks are docs/router.

## Tests / verification

- `go build ./cmd/sirsi/` must succeed after task 6.
- `sirsi` with no args must print help (or a friendly summary), not the broken TUI.
- `sirsi --experimental-tui` must still reach the existing TUI (no functional deletion).
- `go test ./...` must pass (no new tests required — task 6 is a default-route change).
- `golangci-lint run ./...` must pass.
- Manual UAT: user runs `sirsi` and confirms the broken TUI no longer appears by default.

## Risks

- **Task 6 risk:** changing the default route could be considered a user-facing behavior change before v0.23. Mitigation: gate on explicit user approval; document in CHANGELOG; preserve old behavior behind `--experimental-tui`.
- **ADR-016 supersession risk:** existing references to ADR-016 in build logs and Thoth memory will need to be marked as historical context rather than active doctrine. Phase 1 audit task includes a grep sweep for stale ADR-016 references.
- **No-code constraint enforcement:** Claude is parked on this thread until user approves ADR-018. Any drift into scaffold code violates the ADR's stated conditions and Codex's review.

## Confidence declarations (Rule A23)

- **Confidence in ADR-018 capturing the decision faithfully:** High. Mirrors Codex review verbatim on each conditioned point.
- **Confidence that Phase 0 is the right scope:** High. Codex review explicitly required ADR + Phase-1 audit before any code, and the proposal's /goal step 3 enforces it.
- **Confidence in task 6 (hide TUI from default):** Medium. The exact CLI surface change depends on `cmd/sirsi/`'s current root command structure — requires a short read before the edit. Will not proceed without user approval.

## ETA / check-back

- Claude pauses here pending user approval of ADR-018 (router decision: `ready-for-user`).
- On approval, Claude executes Phase-0 tasks 4–8 in this repo only (per Rule A26 repo segmentation), then opens Phase 1 (reuse audit) with a new router proposal for Codex review.
- If user objects to any clause in ADR-018, Claude revises and re-submits — no implementation begins until the ADR is approved as written.
