# Changelog — Pantheon PT canon

## 2026-10-09 — v0.24.99 release candidate

- Revises ADR-072 (universal thread naming) C2/C4/C5 and the phase order per
  SSA round-1
- Closes codex-pantheon's successor CHANGES_REQUESTED on PR #1042 (head
  `0b1184c8`): `Bo

## 2026-10-09 — v0.24.98 release candidate

- `relieve --memory` no longer raises an admin-password dialog when run unattended.

## 2026-10-08 — v0.24.97 release candidate

- Wake loops share one CPU-headroom probe per host.
- Refuses implicit production use of `~/.sirsi/router.db`; service resolution is
  fail-cl
- Makes Ma'at's test ledger binding explicit so tests cannot inherit the live
  router aut
- Publishes the complete Ra router architecture, release contract, traceability
  matrix, 
- Adds a hermetic one-authority negative control and preserves the existing
  routerstore,
- Classification: platform-foundation/pilot. Fresh cloud readback, third-machine
  rehears
- `knownfail.ciRunsExactPath` now parses the CI workflow as real YAML and
  requires the g
- `docs/sprints/SPRINT-ADR065-ROUTER-INFORMER-PHASE1.md` — the Rule-17 sprint
  plan SSA's
- `.agents/completion.contract.json` — `canon_documents[4].path` (portfolio
  manifesto) c
- `docs/sprints/SPRINT-ADR065-ROUTER-INFORMER-PHASE1.md` — third-round
  correction per SS
- `docs/sprints/SPRINT-ADR065-ROUTER-INFORMER-PHASE1.md` — second-round
  correction per S
- `.agents/completion.contract.json` — added (new file); sirsi-pantheon had
  no completio
- Router: refuse the retired implicit local ledger
- `claude-pantheon` shares the reserved consumer slot.

## 2026-10-06 — v0.24.96 release candidate

- `sirsi router task-lease-audit [task-id]` — a read-only audit of task-lease
  ownership 
- The router client can enforce the published TLS key pin (opt-in).
- The TLS pin now binds the per-host relay, the process that holds the host token.
- Ma'at consults the known-failure catalog on every failing gate step, in the pre-push hook, in CI and in the release train.
- Sending to a lane now validates against the pinned registry.
- `claude-m5-compasspoint` is a declared router lane.
- CI + pre-push gate refuse a non-release/* branch that edits `CHANGELOG.md`
  directly; t

## 2026-10-06 — v0.24.95 release candidate

- `sirsi router snapshot`: the router snapshot as JSON, from the same producer as the Horus dashboard.
- The release train clears the abandoned branch a stopped run leaves behind, and refuses to touch real history.
- The Ma'at pre-push gate queues behind another gate on the same Mac, and shows the linter's real error.
- The Horus dashboard is redesigned
- `TestSenderFloodRejected` no longer fails at the top of the hour.
- `/api/router` sends an empty list, not `null`, for a release section with no entries.

## 2026-10-06 — v0.24.94 release candidate

- Reviewer lanes get a reserved consumer slot.
- The release train treats the M5 as best-effort and reports it honestly.
- The changelog now says what is unreleased, and the release script cuts all of it.
- `/api/router` carries what a richer dashboard needs, additively.

## 2026-10-05 — v0.24.93 release candidate

- Router: add the task retry-ceiling operator verb the breaker verbs already established the shape for
- Development candidate: bounded mobile desktop recovery transplanted onto current release
- feat(routerstore): migration gate — a build with uncommitted changes may not
  apply a s
- feat(menubar): decode node-status's `outbox[]` (ADR-069, PR #931) into
  `RBOutbox` and 
- test(menubar): `OutboxReachabilityTests` — the new `macapp` test target
  proving the de
- feat(board): `sirsi board-serve` — the Go router board, replacing the
  out-of-repo Pyth
- fix(menubar): reads `board-serve --once --shape fleet` — a PROJECTION of the
  board's o
- chore: retire the 9119 Horus dashboard (duplicated the menubar) and the
  token-burning 
- Add `sirsi maat submit --kind tag|release|release-edit|merge --repo OWNER/REPO --ref REF
- Router: `setThreadConsumerCapable` no longer silently drops its write — two independent gaps closed, not one
- fix(board,menubar): rename the "touched" column to "last ledger update". It
  measures t
- fix(supervision): `supervision.Escalates()` had no caller — lanes no wake could
  reach 
- fix(dashboard): `fleet.go` hardcoded `Routable: true`, making `UNROUTABLE`
  unreachable
- fix(routerstore): recognize schema v8–v14, already deployed to the live store
  by an ou
- Hooks: the SessionStart inbox counter now warns loudly on a schema drift instead of silently reporting a healthy empty inbox
- fix(router): gemma was unreachable from the router in two independent ways —
  the worke
- fix(router): the wake loop now logs a bounded tail of a failed consumer's
  output. `dis
- fix(routerstore): read-compatibility with a store newer than the binary. The
  write gua
- fix(dashboard): 8734 is now served by the SAME process and handler as 9119.
  It was a s
- fix(board): emit `ledger` alongside `board` in the router-board payload.
  index.html re
- Made CTR thread registration, heartbeat/current-item, close, suspend, and resume SQLite-
- Router: restore the ledger header items/tasks split that PR #668 silently reverted
- A lane can no longer disarm another lane's wake loop.
- Process diagrams for the router wing (SL-DIAGRAM-001).
- Router readiness audit (G1 to G12) and the docs it was missing.
- Exactly-once claim test over 1,000 contended rounds
- Per-deploy audit receipt.
- The release train assembles `changelog.d/` before it cuts a version.
- `cmd/sirsi/threadcmd.go` `thread register` derived the router filesystem root directly f
- Fixed: `SpoolOutboxHealth` used `filepath.Glob`, which silently swallows directory-read 
- `router.NodeStatus` (`sirsi router node-status --json`, GET /api/node-status) gains `out
- Forward correction to the previous append-error propagation fix: PR #929's `appendCedeDe
- Document Pantheon's Ma'at failure-memory contract and eight-domain operational preflight
- liveness-watch re-alarmed on a menubar the owner had just quarantined, and a live agent auto-relaunched it
- fix(router): stray-reap salvage is inscribed only after the save persists

## 2026-10-02 — v0.24.66 release candidate

- Records the lease/session-identity fixes (per-thread session cache, dispatch-contract agent id), the ps-free thread anchor, the pre-push window gate, the bind router-rejection check, the gemma status default port, who-is-on live activity, the ADR-070 revision and the doctor name-conformance report from the exact tested mainline.

## 2026-10-02 — v0.24.67 release candidate

- Records the lease/session-identity fixes (per-thread session cache, dispatch-contract agent id), the ps-free thread anchor, the pre-push window gate, the bind router-rejection check, the gemma status default port, who-is-on live activity, the ADR-070 revision and the doctor name-conformance report from the exact tested mainline.

## 2026-10-05 — v0.24.68 release candidate

- Records the lease/session-identity fixes (per-thread session cache, dispatch-contract agent id), the ps-free thread anchor, the pre-push window gate, the bind router-rejection check, the gemma status default port, who-is-on live activity, the ADR-070 revision and the doctor name-conformance report from the exact tested mainline.

## 2026-10-05 — v0.24.69 release candidate

- Records the lease/session-identity fixes (per-thread session cache, dispatch-contract agent id), the ps-free thread anchor, the pre-push window gate, the bind router-rejection check, the gemma status default port, who-is-on live activity, the ADR-070 revision and the doctor name-conformance report from the exact tested mainline.

## 2026-10-05 — v0.24.90 release candidate

- Records the lease/session-identity fixes (per-thread session cache, dispatch-contract agent id), the ps-free thread anchor, the pre-push window gate, the bind router-rejection check, the gemma status default port, who-is-on live activity, the ADR-070 revision and the doctor name-conformance report from the exact tested mainline.

## 2026-10-05 — v0.24.91 release candidate

- Records the lease/session-identity fixes (per-thread session cache, dispatch-contract agent id), the ps-free thread anchor, the pre-push window gate, the bind router-rejection check, the gemma status default port, who-is-on live activity, the ADR-070 revision and the doctor name-conformance report from the exact tested mainline.

## 2026-10-05 — v0.24.92 release candidate

- Records the lease/session-identity fixes (per-thread session cache, dispatch-contract agent id), the ps-free thread anchor, the pre-push window gate, the bind router-rejection check, the gemma status default port, who-is-on live activity, the ADR-070 revision and the doctor name-conformance report from the exact tested mainline.

## 2026-10-01 — v0.24.65 release candidate

- Records the PR947/PR949/PR950 lineage from the exact tested mainline: lease
  ownership across session remint and threadless invocation, and a fail-closed
  PostgreSQL CI leg.

## 2026-10-01 — v0.24.64 release candidate

- Records the PR944 router-completion release lineage from the exact tested
  mainline, including token-fenced completion, authenticated dual-backend
  coverage, truthful lane-state observability, and reversible router closure.
- The tag and published artifacts become the canonical starting point for the
  next Pantheon build; signing, notarization, cask, and installed-host receipts
  are recorded only after their corresponding release jobs complete.

## 2026-09-28 — v0.24.23 release candidate

- Records the exact tested mainline Stack Lab doctor roster-provenance change
  from PR #843 as the next commercial patch release candidate.
- Release scope remains CLI/Go and canon provenance; macOS signing,
  notarization, Homebrew cask publication, and installed-host qualification
  remain release-workflow evidence rather than source-only claims.

## 2026-09-27

- Added the complete Stack Lab nine-record canon bundle.
- Separated source/static claims from build, package, credential, signing, notarization, remote, cask, and installed-host proof.

## 2026-09-28

- Added the independently cataloged development-versus-commercial release artifact recipe. Commercial artifact names now require the Developer ID/notary/stapling route; ad-hoc package work is explicitly `-dev` only.
