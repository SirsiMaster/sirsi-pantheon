### Fixed

- NEXT corrective source, not a v0.24.124 release gate: preserve bulk thread-store
  API compatibility while adding an AST guard against fence-silent bulk upserts
  in router production code, with deliberate regression fixtures for both APIs.
- Centralize lifecycle timestamp encoding as UTC with nine fractional digits;
  add pure fixtures for offsets, exact width, subsecond ordering and record use.
  Lifecycle CAS behavior, pruning, storage schema and service controls are unchanged.
