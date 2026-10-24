### Corrective test-only source — router TestMain temporary roots

- Run router package setup/tests in a returning helper, then exit outside it,
  allowing both owned temporary-root cleanups to run after passing/failing tests.
- Register cleanup immediately after each creation, retain directory identity,
  refuse substituted paths and report cleanup debt. Preserve nonzero test exit
  codes; cleanup failure turns success into failure. No atomic same-UID race or
  abrupt-process-termination cleanup guarantee is claimed.
- Add actual-test-binary subprocess fixtures under private TMPDIR, plus inert
  setup/removal failure callbacks. Reaper/grant stubs, DB isolation and dispatch
  override stay unchanged; no dispatch fallback or runtime API change.
