- **fix(maat):** a granted reservation no longer exits as refused because the
  host-local decision journal failed. `maat reserve|extend|cede request|grant|
  counter|decline|withdraw` now exit 0 when the router transition committed,
  print a one-line stderr warning, and add an additive `journal_error` field to
  `--json` output. The journal mutation lock now waits up to 3 s (jittered
  flock retry) instead of failing instantly on a concurrent writer.
