### Fixed

- E2E TestMain now returns through its owned temporary-binary lifecycle before
  calling `os.Exit`, including build failures and nonzero test exits. Build/test
  nonzero exit status is preserved. Cleanup failure changes an otherwise
  successful exit to1 and reports the exact root and captured identity as debt,
  not a zero-residue claim.
- Recursive cleanup is confined to the retained root; a changed or unknown named
  root is refused, not passed to path-based `RemoveAll`. Empty-leaf removal is
  cooperative, not an atomic hostile-writer identity guarantee. No shared cache,
  pre-existing residue, production state or unrelated TestMain is cleaned.
- Added child-process fixtures for success, test failure, build failure and root
  substitution after both successful and failed test runs. Fixtures use fake
  build/run callbacks through the same lifecycle
  helper; normal outer TestMain still builds the real CLI. No runtime product or
  current-release behavior changes.
