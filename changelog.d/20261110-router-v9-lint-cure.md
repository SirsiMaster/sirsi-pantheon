### UNRELEASED router v9 lint correction — NOT PUBLISHED

- Format the delivery-history label test and use the compiler-enforced conversion
  between the two identical queue metadata structs. The public queue still has
  only agent/open; unmatched future fields make the conversion fail compilation.
  No assertion, test selection, privacy field, runtime behavior or lint waiver
  changes.
- Retain frozen qualification source `5cb30cbe` and its v9 first lint failure.
  This correction is source-only; normal/race qualification remains unrun.

Refs: ADR-057, ADR-062; router-consumer-visibility-mismatch-20261106.
