**Codex Apollo registration cleanup** (2026-09-28; PR #838). The canonical
router retires the stale `codex-inference` alias and binds the active Apollo
consumer to its M5 checkout as `codex-apollo`, with an explicit public name
and callsign. Historical task records remain unchanged.
