### Fixed — continue shell option selection across plus-options

- Treat valid plus-prefixed short options as options instead of script operands,
  so a later minus-prefixed `-c` still selects its command string. Consume
  `+o`/`+O` option-name operands, including attached values; `+c` does not
  select command text. Existing `--` and script-operand boundaries remain.
- Extend only pure selector controls with harmless `printf` argv values. No
  shell payload is executed, and the recognizer remains a bounded argv selector
  rather than a general shell parser.

Refs: `docs/ADR-040-DO-NO-HARM-TO-THE-RUNNING-HOST.md`,
`docs/ADR-119-SLEEP-CUSTODY-PRODUCTION-CONTROLLER.md`,
independent review `independent-display-2ce95b6f-source-review.md`.
