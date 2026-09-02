<!--
agent: ra (router agent id `ra`; also drains `claude-home`)
workstream: router-service (ADR-062, docs/ROUTER_SERVICE_GOAL.md)
repo: sirsi-pantheon
date: 2026-09-02
session: 0c6da0d0-510c-4de7-9408-7eb217ff73b7
-->

# Continuation — Router as a Service (ADR-062), agent `ra`

## Where it stands (2026-09-02T23:1xZ)

- **Merged to main:** ADR-062 (4b38d111), the /goal (18831542), Migration
  step 1 = #682 f9bc9040, #683 ffb972ea, #684 f63fcfeb (SSA Bind #1 ACCEPT).
  Ledger `ra`: rs-01..rs-04 done, 21 open.
- **Open PRs, in stack order** (each must be retargeted to main, rebased
  `--onto origin/main` past the previously merged commit, force-pushed, CI'd,
  App-bound with `scripts/bind/sirsi-bind.sh`, then squash-merged):
  #685 rs-05 schema (main-based, CI green) → #686 rs-06/07 dialect + Postgres
  CI leg (main-based, CI green incl. Postgres step) → #687 serve/RemoteStore →
  #688 identity → #689 per-host tokens → #690 migrate-store → #691 rs-13
  evidence + 503 fix + bench (will drop its duplicate ci-postgres commit on
  rebase). #692 ADR-062 rev 4 amendments (docs, main-based).
- **Awaiting SSA (`sirsi-software-admin`):** Bind #2 resubmission
  (20260902-230810) with CI receipts; ADR rev 4 review (20260902-231032).
  Transport/rs-10 verdicts were CONDITIONAL on things now built (#688, #689,
  #691) — send Bind #3 for #687–#691 once #686 is on main, citing
  `docs/evidence/ADR-062-RS13-TWO-MAC-EVIDENCE-20260902.md`.
- **Owner gates ahead:** rs-14 (cloud placement / token custody / service
  account scope) before anything touches GCP; rs-19 before M5 stops writing
  its local file.

## Mechanics that cost time (do not rediscover)

- GitHub runs no CI and creates no merge commit for a PR whose base is an
  unmerged branch; after a squash-merge the next PR reads `dirty`. Fix per PR:
  `gh pr edit N --base main`, `git rebase --onto origin/main <merged-commit>
  <branch>`, force-push with lease; CI then runs; bind; merge.
- Bind identity: every agent is `SirsiMaster`, GitHub refuses self-approval;
  `scripts/bind/sirsi-bind.sh <pr> --repo SirsiMaster/sirsi-pantheon --body @f`
  records the approval as the sirsi-bind App (key only on this host). Rerun
  the `binding-hold` workflow after binding if the rollup shows it missing.
- Scratch Postgres for tests: PG 14 (brew) with `-k /tmp` (socket path length),
  `SIRSI_TEST_PG_DSN=postgres://sirsi@127.0.0.1:54329/routerstore_test`.
  `scripts/ci-postgres.sh` starts its own on :54331.
- Never open the live `~/.sirsi/router.db` with a dev build: `OpenPath` runs
  migrations and would advance its schema past the deployed binary. Migrate a
  copy.
- The `ra` thread gets reaped if heartbeats lapse; re-register with
  `sirsi thread register --agent ra --surface claude --workstream ra --watch ra,claude-home --consumer-capable`
  and heartbeat every few tool calls.

## Next actions, in order

1. Poll `sirsi router pull ra` / `claude-home` for SSA's Bind #2 and rev-4
   verdicts; on ACCEPT bind + merge #685, #686, #692.
2. Walk the rest of the stack onto main (#687 → #691) as above.
3. Send Bind #3 (Phase C) with the evidence doc; on ACCEPT merge.
4. Send the rs-14 owner decision card (already drafted as a router item to
   `owner`); do nothing cloud-side until it is answered.
