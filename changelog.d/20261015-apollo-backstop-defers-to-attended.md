- **fix(router):** the codex-apollo background consumer now defers to a live attended session. Its prompt first runs
  `sirsi thread attended`; when attended, the worker is a mail-only backstop (no ledger-task claims, never `apollo-mm-*` /
  `apollo-multimachine-*`, release any such lease it holds). Keeps the consumer (so mail is never stranded when the
  attended session is away) while stopping the duplicate execution reported by Apollo on 2026-10-15. This is prompt-level
  intent over the existing attended check, not mechanical enforcement; a dispatch-gate hold for attended lanes is tracked as
  `ra-attended-defer-gate`.
