- Isolate submit requester and command tests from the shared linked-worktree
  registry by reusing the already-reviewed PR #1419 fixture. Keep real
  declaration, policy, refusal-exit and ledger-failure validation; do not
  restore or depend on another working session's registry.
- Test-only prerequisite for existing PR #1378. Production requester authority
  and the separate PR #1419 diagnostic work are unchanged.
- Require the undeclared-marker control to name the real declaration failure
  and owned registry path, rather than accepting a generic wrapper message.
- Publish the valid-response spool fixture through the existing atomic relay
  publisher. Keep the partial-body-close retention assertion and separate
  malformed-response fail-closed control unchanged.
