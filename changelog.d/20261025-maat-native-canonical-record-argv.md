### Fixed

- Send the canonical `--record-request` flag from the native diagnostic adapter.
  Make its inert command fixture use an independent literal flag table; preserve
  existing invocation-count and once-consent assertions. Add literal argv checks
  for all four request operations and bind native interop to actual Go-accepted
  argv. Source-only correction; no test/build/installed execution claimed.
