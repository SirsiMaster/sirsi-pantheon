### Fixed — select shell command text using shell-specific option arity

- Keep Bash optional `-o`/`+o`/`-O`/`+O` names from consuming a following
  option when the name is omitted. Treat Zsh `-O`/`+O` as no-operand flags,
  while Zsh `-o`/`+o` consumes its named option. Consume Zsh's separate
  `--emulate MODE` operand before scanning its following invocation options.
- Retain actual minus-prefixed `-c` selection, bundled short options, separate
  Bash rcfile/init-file operands, Zsh emulation modes, and `--`/script boundaries.
  The matrix uses harmless `printf` argv values and does not claim general
  shell parsing or shell execution qualification.

Refs: `docs/ADR-040-DO-NO-HARM-TO-THE-RUNNING-HOST.md`,
`docs/ADR-119-SLEEP-CUSTODY-PRODUCTION-CONTROLLER.md`,
independent review `independent-display-15fbf097-consolidated-selector-source-review.md`.
