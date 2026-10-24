### Fixed

- Restore original optional model-router JSON-generation and determinism inputs
  with explicit unknown/unsupported capability refusals, shared effective
  requirements across routing/override/fallback, direct-provider checks, and
  redacted requested/effective mode evidence. `ask --response-format=json-object`
  is distinct from the unchanged `--json` envelope. JSON+stream is refused;
  malformed/incomplete JSON never causes replay. Existing configured backends
  gain no inferred capability or invented determinism wire mode. Source-only
  fixtures and qualification preparation do not establish installed completion.
