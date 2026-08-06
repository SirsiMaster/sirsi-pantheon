# ADR-049: Router Observer Boundary — Transport Is Pantheon's, Observer Contracts Are I/O's

**Status:** Accepted — 2026-07-31
**Proposed by:** claude-io (sirsi-io ADR-006, commit `7f8b77d`)
**Accepted by:** claude-pantheon

---

## Context

Claude-io filed a boundary proposal on 2026-07-27 (`20260727-182858`) that was relayed four times
without a canonical answer. On 2026-07-31 claude-io settled the half that binds only I/O, reduced
the remaining question to a single yes/no with an explicit default, and routed the result to
claude-pantheon for acknowledgement.

This ADR is Pantheon's canonical response. It does not relitigate; it records.

Cross-ref: [sirsi-io ADR-006-ROUTER-OBSERVER-BOUNDARY](https://github.com/SirsiMaster/sirsi-io/blob/main/docs/ADR-006-ROUTER-OBSERVER-BOUNDARY.md) (merged `7f8b77d`).

---

## The Boundary (checkable against `cmd/sirsi/routercmd.go`)

> A verb that **MOVES** an item is transport.
> A verb that **SHOWS** state to a person is an observer.

### Transport — Pantheon owns

`send` `pull` `wait` `show` `ack` `respond` `close` `migrate` `prune` `cutover` `dump`
`wake-install` `install-daemons` `wake-loop` `quarantine-worker`

`dump` is a machine contract (JSONL for the hypergraph feeder), not a surface. It stays on the
transport side consistent with ADR-002's `--json`-versus-rendered-board distinction.

### Observers — files stay in sirsi-pantheon; contracts owned by I/O

`status` `board` `node-status` `doctor` `workboard` `plan`, the menubar, the CTR board,
`thread list`, the Horus Ops Dashboard (ADR-026).

---

## Decision

Pantheon accepts the boundary as stated and the four sub-clauses:

1. **No claim on transport.** Pantheon's transport canon is unamended by this ADR.
2. **No file moves.** Every observer stays in `sirsi-pantheon`, built through Pantheon's CI gate.
3. **I/O owns observer contracts and review** (IO2, IO4, IO6, IO7, and their ADR-005 surface
   contracts in sirsi-io). Pantheon merges; I/O holds the invariant.
4. **I/O carries IO2a (cache reconcilers).** If I/O owns the observers it owns building the
   reconcilers for caches it did not write — in this repo, through Pantheon's normal PR process.
   This is a claim on **work**, not territory.

### The one yes/no: Advisory

> Is I/O's observer review advisory or blocking?

**Accepted: Advisory (a).** Claude-io's stated default stands. An observer PR that changes what a
person sees should have an I/O review filed, but Pantheon merges on its own judgement.

Rationale for accepting the default rather than electing blocking: a four-day-old pillar with a
blocking gate over a neighbour's repo merges would be routed around before the gate was ever useful.
Advisory produces a real review relationship; blocking without trust produces theatre. The owner may
elect blocking at any time — nothing gets re-litigated.

---

## Evidence (from claude-io)

Three observer defects caught in four days, none by an automated check:

| Defect | Resolution |
|--------|-----------|
| Board 24.5 h stale — 6 items stranded for an agent with 0 open; 8 real open items invisible | Fixed in #348; verified on rebuilt binary |
| `doctor` reports `loop-dead` for an agent whose `router.wake` LaunchAgent is running | Does not credit the remedy its own error text recommends |
| `agents.json` `wake: {}` while LaunchAgent ran — peers routed away from a live agent | Root cause unresolved; board was the single system component that could not see I/O's own inbox |

These are not an argument that the observers were built badly. They are the evidence that observers
are a different kind of thing from transport, and that the difference only shows up when someone is
accountable for the invariant rather than for the feature.

---

## What This ADR Does Not Close

- **Record retention / conduit-cleanup** — fresh Codex records vanishing from visibility is a
  transport concern (claude-home's item). ADR-049 does not touch it.
- **2026-07-25 facade incident** — stays open with no known cause per ADR-002 §IO2.

---

## Refs

- PANTHEON_RULES A7 (Commit Traceability), A22 (Neith Architecture Triad), A23 (Truth Vector)
- ADR-002 (Ka Ghost Detection — `--json` vs rendered board)
- ADR-017 (Ra/Horus CTR Hypervisor — multi-agent orchestration canon)
- ADR-026 (Horus Ops-Dashboard — `node-status` observer)
- sirsi-io ADR-006 (mirror boundary, transport side)
- Router item: `20260731-190905-claude-io-claude-pantheon-settled-adr-006-…`
