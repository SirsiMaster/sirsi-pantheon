### Empty Trash keeps a bounded local operation record

The review-bound Empty Trash apply path now persists the exact reviewed manifest before its first unlink, records each durable attempt and known result, and exposes bounded newest-first history plus exact operation detail through `sirsi anubis empty-trash history`. Existing Report v1 fields and explicit confirmation remain unchanged; unfinished or damaged records stay visible as unknown evidence. Receipts do not restore permanently deleted bytes or claim Trash namespace durability. The qualification status below records package-limited Go passes and an incomplete native wrapper result; independent review is pending.

History file opens now refuse blocking/special-file substitutions, and index lock contention fails before deletion with retry guidance. A partial crash-left index tail is isolated with a synced newline boundary before a later row is appended; failure to persist that boundary prevents deletion. Corrupt attempt indexes and terminal counts are surfaced as unknown history instead of panicking. Failure fixtures distinguish a partially written attempt (unknown outcome) from zero progress (not finalized).

Qualification record: Go normal/race passed for the durable-history package inventory on source 4629f327: 29 top-level tests and 93 passing events across two packages in each run (normal 40.016s; race 120.613s); Go blobs matched at 7960a495. At 7960a495, the strict-concurrency native build passed and XCTest reported 18 passes, but its wrapper failed on missing XML. Current successor 44685c22 contains only the Darwin journal lock-release/dead-wrapper cleanup and has not been rerun. Independent review, release qualification and installed-history verification remain pending.

The native history fixture propagates JSON encoding errors through XCTest's throwing test method.

Refs: `docs/stacklab/pantheon-pt/canon/TRASH_RECEIPT.md`, `contracts/stacklab/anubis-trash-receipt-v1.json`, `docs/ADR-033-REMEDIATION-CATALOG.md`
