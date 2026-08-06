# Observer Contract — `sirsi router workboard` (the Work Board)

> Inherits all invariants from [`router-observers-common.md`](router-observers-common.md).
>
> **Surface**: `cmd/sirsi/workboardcmd.go` · `internal/router/workboard.go`
>
> **Purpose**: "what needs me right now?" — every agent's open packages, its
> peers, and the fabric's pace at a glance.

---

## 1. Data Sources

| Source | What it provides | On failure |
|--------|-----------------|-----------|
| `AllItems(routerRoot)` | open + closed items corpus (90-day retention) | propagate error; surface aborts |
| `LoadThreadRegistry(routerRoot)` | live surfaces per agent (for `Live` + `Surfaces`) | best-effort; liveness omitted, packages + pace remain |

The board is **computed on read, stored nowhere**.  `WorkBoard.GeneratedAt` is
the canonical timestamp of this computation; consumers derive item age from
the item's own `opened` timestamp and may derive board freshness from
`GeneratedAt`.

---

## 2. Empty State — Positive Claim vs. Failure

The board's job is a **positive claim**: "nothing needs you right now."  If a
failure can produce that claim, the failure is invisible precisely when it
matters.

**Rule**: when `ComputeWorkBoard` succeeds (no error returned) and the result
contains zero agents/packages, the surface MUST display an explicit positive
confirmation — e.g. `— none — fabric healthy` — rather than silently rendering
nothing.

A failure (`ComputeWorkBoard` returns a non-nil error) MUST propagate as an
explicit error to the caller; it must never be silently collapsed into an empty
board.

This rule closes D-BOARD-3 (claude-io review 2026-07-31): the previous
implementation rendered empty and failed identically, making a read failure
invisible at the moment an empty board is most likely to be misread as "all
clear."

---

## 3. Cache / Reconciler

The board is a **live-read-with-fallback reconciler** — the simplest kind.
`LoadThreadRegistry` failure is absorbed (liveness data falls back to "all
offline"); `AllItems` failure is fatal (packages are the board's primary data).

Naming this a reconciler — rather than "none" — makes the fallback path's
unbounded divergence visible: if the thread registry is unreadable for an
extended period, the board accurately shows all agents as offline while still
showing their open packages.  The divergence is bounded by the next successful
registry read, not by a TTL.

---

## 4. Compliance Ratchet

This contract applies **now** (`ComputeWorkBoard` + `workboardcmd.go` are
actively developed surfaces).  The empty-state fix (§2) is the only
outstanding obligation; all other invariants in the common contract are
already met.
