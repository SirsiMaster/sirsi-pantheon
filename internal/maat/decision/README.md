# `internal/maat/decision` — the live, drillable Ma'at decision ledger

Owner ask (via claude-io, 2026-09-26): *"Ma'at should show its grants and
refusals and other work in the Pantheon fabric somewhere... this should be
live and drillable to what it assessed, who it affected, what determination
was made and why."*

## What it is

An append-only JSONL file per host (`~/.sirsi/maat/decisions.jsonl`), one line
per Ma'at decision: a reservation grant/refuse/queue/release, a conflict
check, a guard verdict, a window-gate block, or a CI pause/resume. Every
record carries: `time`, `host`, `kind`, `requester`, `resource`, `assessed`
(what was assessed), `affected` (who it affected), `determination`, `why`, and
`evidence` (a link/id into the underlying system — a reservation id, a CI run
id, a repro path).

Read via `sirsi maat decisions` (list, filterable by `--kind`/`--host`/
`--since`) and `sirsi maat decisions show <id>` (full record; `<id>` is the
first 8 hex chars of the sha256 of the raw JSON line — stable and needs no
writer coordination).

## Design

- **Schema-tolerant reader, not a strict struct.** `Record` is a
  `map[string]any`. Other hosts' writers (m5go, the `maat-window-gate` hook,
  `maat-run-guard`) append to the same file; a fixed struct would silently
  drop any field name it didn't expect instead of displaying it. Unrecognized
  keys still print in `decisions show`.
- **Host-local by design, for now.** `internal/maat/schedule` already proved
  the pattern for a *cross-host* ledger (reservations live in the shared
  router store). This file deliberately does not do that yet: it matches the
  convention already in use by the other hosts' writers, and promoting it to
  a cross-host store is the follow-up Ra is being asked about (canonical
  ledger location). This package does not block on that answer — each host's
  decisions are self-contained and drillable today.
- **Best-effort append.** `reserve`/`release`/`conflict-check` in
  `cmd/sirsi/maatschedule.go` call `recordDecision` after they've already
  acted; a ledger-write failure never blocks or reverses a decision that
  already happened.

## Native writers in this repo

`sirsi maat reserve|release|conflict-check` write their own decision records
natively (closing the gap the ask named: "Ma'at's own `reserve` grants/
refusals should write the same record natively, instead of callers doing
it"). Kinds: `reservation grant`, `reservation queue`, `reservation refuse`,
`reservation release`, `conflict-check` (determination `clean`/`conflict`).

## Ceilings (deliberate)

- No cross-host aggregation yet (see above) — `sirsi maat decisions` on one
  Mac only sees that Mac's decisions.
- No retention/rotation. The file grows unbounded; add a prune pass if it
  ever matters in practice.
