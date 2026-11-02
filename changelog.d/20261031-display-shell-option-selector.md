### Fixed — select shell command text from the actual `-c` option

- Replace substring matching on shell options with a bounded argv selector.
  Long options such as `--norc` no longer stand in for `-c`; bundled `-lc` and
  `-ic`, option operands for `-o`/`-O` and Bash `--rcfile`/`--init-file`, the
  `--` boundary, and script-operand stopping are covered by pure harmless
  `printf` controls.
- Later positional argv values remain outside the selected command string.
  This is an argv option selector, not a general shell parser.

Refs: `docs/ADR-040-DO-NO-HARM-TO-THE-RUNNING-HOST.md`,
`docs/ADR-119-SLEEP-CUSTODY-PRODUCTION-CONTROLLER.md`,
independent review `independent-display-cbd5d7ca-source-review.md`.
