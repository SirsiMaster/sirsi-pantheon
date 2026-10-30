# Universal Task Ledger read model

`ledger` is a pure join layer. Persistence remains in `routerstore`/`dispatch`;
thread liveness remains in `router`. `BuildFrom` accepts those records and
produces the typed snapshot used by CLI and CTR surfaces.

Dependency rules are deliberately deterministic: missing and cyclic edges fail
closed; terminal dependencies release; pickup requires exact `current_item`
evidence. Keep new presentation logic out of this package and keep new
aggregation logic out of renderers.

Tests should construct item, task, and thread records directly and assert the
snapshot. Do not touch the live router database in package tests.

Fabric reads propagate their operation context through snapshot, node-status,
operational-agent, and runtime-classification stages. `FabricReadStage` records
only a fixed stage code and monotonic elapsed duration on the active producer
operation; deadline attribution describes the stage active when cancellation
was observed, not a measured share of the total acquisition budget. Keep public
errors payload-free and preserve the wrapped context cause.
