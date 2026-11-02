### Empty Trash keeps a bounded local operation record

The review-bound Empty Trash apply path now persists the exact reviewed manifest before its first unlink, records each durable attempt and known result, and exposes bounded newest-first history plus exact operation detail through `sirsi anubis empty-trash history`. Existing Report v1 fields and explicit confirmation remain unchanged; unfinished or damaged records stay visible as unknown evidence. Receipts do not restore permanently deleted bytes or claim Trash namespace durability. Source and failure-injection fixtures are unqualified pending coordinated Go and native runs.

History file opens now refuse blocking/special-file substitutions, and index lock contention fails before deletion with retry guidance. A partial crash-left index tail is isolated with a synced newline boundary before a later row is appended; failure to persist that boundary prevents deletion.

Refs: `docs/stacklab/pantheon-pt/canon/TRASH_RECEIPT.md`, `contracts/stacklab/anubis-trash-receipt-v1.json`, `docs/ADR-033-REMEDIATION-CATALOG.md`
