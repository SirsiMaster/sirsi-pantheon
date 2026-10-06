# Commercialization gate entries

Each entry follows the portfolio-law format used in `docs/prd/RELEASE_V1_STAR_GRADE.md`. An entry records closure honestly: a dimension that is not closed says so. Classification stays `pilot` until every dimension is closed.

## Ra platform foundation: the router (ADR-062, ADR-063)

Recorded 2026-10-05 by Ra. Evidence: `docs/evidence/ADR-062-ROUTER-READINESS-AUDIT-20261005.md`.

- **Buyer/User:** Teams that run several AI agents across more than one machine and need work handed between them without loss.
- **Pain:** Agents duplicate work, drop it when a session ends, or wait on a human to relay it. Nothing records who claimed what or what proved it done.
- **Primary workflow:** One agent sends an item. The recipient wakes without a human, claims it exactly once, works it and closes it with evidence. The owner sees the whole fleet on one board.
- **Willingness to pay:** Not validated. No customer conversation or pricing test is recorded in this repo. Intended as paid fleet orchestration (Ra) beside the free CLI.
- **Trust boundary:** Lanes never hold the service token; a per-host relay does. Every call is bound to a registered session. Router items carry work, not code or secrets. Zero telemetry (Rule A11) applies.
- **Operational owner:** Ra (router architect), reviewer sirsi-software-admin, owner Cylton/Sirsi.
- **Done evidence:** the G1 to G12 table in the audit above. Today 7 of 12 are met (G11 on merge of this entry), 3 partial, 2 open.
- **Classification:** `pilot`.

### Closure by dimension

| Dimension | Status | What is missing |
|---|---|---|
| Product | Open | Hosted service is single-tenant (one project, one operator). No "run your own" install path has been rehearsed by someone outside the team. |
| Design | Partial | Operator dashboard redesigned and shipped (v0.24.69). First-run experience for a new user is unwritten beyond the user guide. |
| Technical | Partial | Exactly-once under load (G3), client-side TLS pin enforcement (G6) and a third-machine rehearsal (G9) are not evidenced. |
| Operational | Partial | Runbook exists (this change). Cloud Run rollback and token rotation are documented, not rehearsed. Not self-sustaining: unknown failure classes still need a hand repair. |
| Narrative | Open | The public README does not mention the router. No launch copy. |
