# Completion entry fixture provenance

`agent_completion_gate.py` is an **unchanged, test-only retained copy** of the portfolio production helper read at `/Users/thekryptodragon/Development/tools/agent_completion_gate.py` on 2026-10-24.

SHA256: `a4953e61937d8bf03dfe9fbbcd6a0decb3fa44ad56fd4550fab0b0444cbf8ab9`.

`New` checks the embedded bytes against this exact digest before creating any validator fixture. Keeping the copy makes CI independent of a developer's external portfolio directory. It is not a second product validator and must not be edited as an independent policy implementation. Any future helper update requires explicit provenance and a new reviewed pin.

The complete proof scaffold is adapted from the existing `/Users/thekryptodragon/Development/tools/test_completion_final_status.py`, SHA256 `90d06b0cd24359a8be900d82f3efd9b2f230fc328a43b86716331a2746a7fe62`. That scaffold supplies all proof sections. These entry fixtures additionally supply a complete on-disk contract, five done-definition categories, a nonempty scope/canon file, and an evidence file. Unlike the scaffold's isolated function tests, no contract loader, proof loader, validation function, or policy is mocked. The actual helper's `main()` validates the real fixture contract and captured proof.

`capture_and_forward.py` is transport instrumentation only. It records argv mode and stdin bytes, replaces the proof pathname after capture, and forwards the **unchanged original argv and stdin bytes** to the retained helper via the same Python interpreter. It propagates that process's exit status. It does not parse proof semantics or turn refusals into success. Replacement of the pathname proves that completion validation consumes captured bytes, not a second pathname read.

The CLI/MCP tests invoke their actual production entries and shared Go gate. They use owned HOME, Git caller, registry, and explicit SQLite fixtures. Each case changes only final status, blockers, or verification status from the valid baseline (plus a malformed-JSON refusal control). Non-completed proof status is not a coordination `--blocked`/`--ack` request. No production code or gate policy is changed.

This qualifies the retained helper at the pinned revision through actual entry points when Main runs the tests. It does **not** claim whole CompassPoint completion, a current live canonical service, installed behavior, or future external-helper parity. Source preparation alone is not a passing test receipt.
