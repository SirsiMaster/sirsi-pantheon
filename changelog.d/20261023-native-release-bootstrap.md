### Fixed

- Native release builds use the installed Ma'at supervisor and CLI as one
  compatible bootstrap pair, not a future workspace supervisor that may require
  flags the installed CLI cannot parse. Missing supervision fails before either
  payload runs; registration, admission, renewal, bounded CPU/job settings and
  cleanup remain mandatory. A legacy installed pair can conservatively obtain a
  full build grant until upgraded; this is not proof of finite CPU/model sharing.
  No security, signing, foreign-job or ordinary lane admission rule is bypassed.
