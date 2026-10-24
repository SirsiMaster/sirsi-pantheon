### Fixed

- Ma’at’s Claude admission classifier accepts terminal shell whitespace on
  foreground supervised runs, registration and source reads. Previously a
  trailing newline was classified as a forbidden extra command. Interior
  commands, background runs, expansions and direct unadmitted workloads remain
  checked; parser regression tests never execute their command text.
