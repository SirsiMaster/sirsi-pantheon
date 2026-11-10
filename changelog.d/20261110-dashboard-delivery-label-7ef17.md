- Label an `UNROUTABLE` lane with currently available routing and unresolved
  historical wake failures as `DELIVERY NEEDS REVIEW`. This display label does
  not change the canonical enum, routing gates, failure counts or history;
  unavailable or unqualified routes keep their existing labels.

- Refs: ADR-057; existing router consumer-visibility work; reviewed source
  `7ef17b7774bcfa5aade6a15e71d5b687fc29d65e`. Original full-ledger normal/race
  evidence remains bound to that source; this composition is not runtime-qualified.
