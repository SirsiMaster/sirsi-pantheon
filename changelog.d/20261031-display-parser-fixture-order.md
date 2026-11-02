### Corrected — match substitution fixture traversal order

- Update only the expected order in the two inert command-substitution lexer
  fixtures. The parser emits each substitution's invocation before the outer
  invocation, matching its recursive recognizer traversal. The first focused
  applied run exposed a fixture ordering mismatch; it did not identify a
  production defect. All empty-argument and other parser assertions remain.

Refs: `docs/ADR-119-SLEEP-CUSTODY-PRODUCTION-CONTROLLER.md`,
`display-parser-cure-0312`, source checkpoint `cbd5d7ca61ba8056dd1589415a2b6c691798b58e`.
