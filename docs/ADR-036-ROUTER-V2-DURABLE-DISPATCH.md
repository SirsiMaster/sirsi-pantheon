# ADR-036: Router v2 — Durable Dispatch (SQLite authority, one facade, event wake)

## Status
**Accepted** — 2026-07-07 · claude-pantheon; the Phase-2 contract within it was adversarially bounced with codex-SME (round 1 FAIL-as-stated, round 2 APPROVE) and is canonized as ADR-035's axioms. Custodian: 𓁢 the Router. Governs PANTHEON_RULES A26/A27's dispatch substrate. PRD: `docs/prd/ROUTER_V2_DURABLE_DISPATCH.md` (Phases 1–4 built 2026-07-02 → 2026-07-07).

## Context

The bootstrap router was markdown files in git: every send/close was commit-noise and merge-conflict surface; "waiting" was a 1-second poll loop in a long-poll costume; and the CLI and MCP surfaces had drifted into two write implementations — the MCP path still writing to directories retired by ADR-024. Then the 2026-07-03/04 runaway-executor incident (19,195 sessions / 11,564-item flood / 1.3 TB — case study `docs/case-studies/2026-07-04-runaway-executor.md`) proved the deeper point: a dispatch layer whose authority can be bypassed by any file write cannot bound anything.

## Decision

Router v2, in four shipped phases:

1. **Durable store as dispatch authority** (`internal/routerstore`, SQLite via CGO-free `modernc.org/sqlite` at `~/.sirsi/router.db` — outside every git tree). Schema mirrors `work.Item` field-for-field, enforced by a reflection test; versioned migrations via `user_version`.
2. **The §2b Dispatch Contract** (ADR-035): fenced lease lifecycle (`open → claimed → working → blocked | dead_letter | completed`), idempotency keys and per-sender quotas as **partial-unique-index database invariants**, keyed-singleton escalations, circuit breakers, budgets, and dispatch counters — with safety tests that reproduce both incidents.
3. **One dispatch facade** (`internal/dispatch`): `sirsi router *` and the MCP `router_*` handlers call the same package. Writes commit store-first (no store row, no dispatch) then dual-write the `items/*.md` audit view byte-identically; `router_wait` is a real blocking wait (<250ms event wake, bounded 5s legacy re-check).
4. **Migration + dual-read window**: `sirsi router migrate` imports every existing file item with verification evidence (count-in == count-out, spot-checked bodies, idempotent); facade reads are the union of file items and store rows by id, so neither a legacy writer nor a failed audit write can hide work.

**The cutover — mechanism shipped, flip is an owner-visible deploy step.** Runtime files are now untracked from git (#196). The cutover *mechanism* is built behind one flag `SIRSI_ROUTER_STORE_WAKE` (default off, so a binary ships identical-to-before): wake rides the store via `sirsi router wait` (#198); with the flag on, `Send` stops writing `items/<id>.md` and `Show/Pull/Status/Close` read/close from the store. Flipping it on strands nothing *only after* the `router wait` verb is in the running binary and every live watcher is re-armed — so the flip is a deliberate, live-verified deploy step, not a side effect of a build PR. See ADR-037 for the end-state (daemon-owned control plane) and the ship-complete completion-proof.

## Alternatives Considered

1. **Keep the file router and harden the workers** — Rejected: the incident proved bypassable authority relocates failure instead of removing it (ADR-035 axiom 1).
2. **BoltDB / flat JSON state** — Rejected: no queryability for inbox/lease/quota invariants; SQLite's partial unique indexes make idempotency and singletons *database* facts rather than application promises.
3. **CLI calls MCP over the wire (or vice versa)** — Rejected: a network hop inside one host to avoid a shared package; the shared facade removes drift without a protocol.
4. **Hard cutover in Phase 4** — Rejected: stranding in-flight file items or surprising legacy readers violates do-no-harm (Rule 14/A12); dual-read + deprecation window instead.

## Consequences

- **Positive:** dispatch state is durable, queryable, and outside git; floods and duplicates are refused at every surface identically; waiting agents wake on events; the next incident is one red counter in `node-status`, not a directory of files. The worker re-arm gate (ADR-035 axiom 6) finally has the substrate its acceptance bar names.
- **Negative:** two representations exist during the window (store + files), reconciled by dual-write/dual-read and regenerable via `ExportMarkdown`; the cutover debt is explicit and owner-gated.
- **Enforcement:** Ma'at treats a new send/close path outside `internal/dispatch` as a governance failure. Regression suites: `internal/routerstore` (contract + incident reproductions), `internal/dispatch` (facade, dual-read, migration zero-loss), `internal/mcp` (cross-path single-source-of-truth).
