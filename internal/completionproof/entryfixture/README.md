# Synthetic completion entry fixtures

This package supplies data and owned temporary Git/SQLite state to the actual
CLI/MCP close tests. Product policy is embedded from
`../validator/agent_completion_gate.py`, never supplied by this package.

The retained `agent_completion_gate.py` (historical SHA-256
`a4953e61937d8bf03dfe9fbbcd6a0decb3fa44ad56fd4550fab0b0444cbf8ab9`)
and `capture_and_forward.py` are historical test-source records only. Neither
is embedded, executed or installed by these fixtures now. They are not the
canonical helper, not approved replacement source, and not release authority.

The fixture configures a forged external helper that would create a marker and
accept every proof if selected. Actual negative cases must still refuse and
leave the durable row/lease unchanged; the completed positive must transition
through the real entry. `AssertValidator` retains its old API name for caller
compatibility but asserts only no forged-helper execution and unchanged proof
bytes. It no longer claims an instrumented stdin receipt. Exact captured-stdin
causality and standalone parity use the actual production validator in the
parent package's tests.

Cases include non-completed/malformed status, blockers, failed verification,
malformed JSON, mismatched item and omitted/changed/duplicated required command
identities. The complete scaffold is synthetic declaration data, not actual
qualification of an application or proof that any listed command ran.

ADR-062 resolver boundaries remain unchanged: private HOME, cleared inherited
service selectors, explicit owned SQLite path via `routerstore.Resolve`, and
backend/path assertions. No live ledger, registration, service, model or remote
consumer is needed. Tests are authored, not executed by source preparation.
