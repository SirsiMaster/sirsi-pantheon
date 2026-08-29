# Sirsi Local-AI Task Registry Reconciliation

**Date:** 2026-08-22
**Rule:** no blanket close; every transition requires exact evidence or named
supersession into the active 36-item zero-open inventory.

## Fenced false-open closures

Four `codex-inference` rows whose own records already asserted completion were
claimed by exact ID and completed through fenced leases:

| Task | Evidence |
|---|---|
| `sne-44-pantheon-shape-crash` | `sirsi-inference/docs/evidence/SNE-44-PANTHEON-LONG-PROMPT-P0-20260805.md` |
| `sne-52-ledger-schema-v7-build` | `sirsi-inference/docs/owner/CODEX-INFERENCE-WORK-BOARD.md` |
| `sne-37-grant-terms` | `sirsi-inference/docs/adr/ADR-003-anubis-sne-seam.md` |
| `sne-authority-window` | active zero-open closure charter |

No partial `5/7` framework gate, failed memory test, active pipeline, or
unproven model task was closed.

## Deduplication map

| Historical registry family | Active inventory authority |
|---|---|
| SNE-41/SNE-44 legacy broker and source-bind rows | 11, 14, 17, 28, 31, 32 |
| SNE-27/Core ML/ANE and AppleStack E/F/G rows | 15, 24, 25, 26 |
| QMV/QMM/ws3d/down-family rows | 12, 14, 20, 23 |
| SNE-43 durable prefix/cache rows | 16, 17, 21, 31, 32 |
| SNE-31 multi-Mac pipeline rows | 30 |
| AppleStack manifest and comparator rows | 19, 20, 22, 32 |
| Installer experience | 3, 4, 27, 33 |
| Standing monitor/supervision rows | operational duties, not product-completion evidence |

Historical rows may be marked superseded only after a task-specific check that
its still-valid obligation appears in the mapped active item. A superseded
implementation result does not prove the successor item complete.

## Exact retired-lineage rows

The following legacy SNE v1/broker records are superseded by the sole active
SNE v2 lineage. Their surviving requirements are explicitly carried by the
inventory items shown; closing these rows removes duplicate implementation
lineage and does not close the successor obligation.

| Task ID | Successor inventory items |
|---|---|
| `sne-41` | 11, 14, 18, 31, 32 |
| `sne-41-live-attachment` | 11, 17, 28, 31 |
| `sne-41-gpu-validation` | 11, 14, 19, 20 |
| `sne-41-stage1-review` | 11, 14, 32 |
| `sne-41-quiet-window` | 11, 20, 21 |
| `sne-32-second-model-family` | 11, 22 |
| `sne-44-broker-parity` | 11, 17, 28 |
| `sne-production-cutover-contract` | 11, 17, 28, 31 |
| `sne-44-source-bind` | 11, 31, 32 |
| `sne-49-release-dogfood-reconciliation` | 4, 11, 31, 32, 33 |
| `sne-broker-memory-balloon` | 6, 17, 20, 21 |
| `inference-permanent-work-loop` | active native goal and closure inventory |

The memory-balloon row is a **failed historical result**, not a repaired result:
the retired broker reached 34.9 GB active memory and 78% swap. Its closure means
the defective broker is no longer an active candidate; SNE v2 admission,
fresh/soak testing, memory pressure, and Pantheon swap care must prevent the
same class under inventory items 6, 17, 20, and 21.

## Excluded records

Generic router-fabric defects, unrelated customer-product work, investor/legal
decisions, and unavailable-account actions are not silently absorbed into the
local-AI goal. Pantheon defects that affect local-AI delivery remain mapped to
items 3, 5, 7, 8, 9, or 10 as applicable.
