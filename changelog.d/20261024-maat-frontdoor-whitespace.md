### Fixed

- Ma’at’s Claude admission classifier accepts terminal shell whitespace on
  foreground supervised runs, registration and source reads. Previously a
  trailing newline was classified as a forbidden extra command. Interior
  commands, background runs, expansions and direct unadmitted workloads remain
  checked; parser regression tests never execute their command text.
- Source/control reads may suppress stderr to the fixed `/dev/null` device.
  Arbitrary file targets, append/descriptor duplication, supervised-run
  redirection and direct workloads remain refused. This separately addresses
  the exact CompassPoint source-read payload, which contained no newline.
- Null-stderr recognition retains shell quote/escape and IO-number adjacency,
  so executable `2 > ...` cannot be mistaken for descriptor `2> ...`.
