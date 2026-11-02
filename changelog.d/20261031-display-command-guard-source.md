### Fixed — restore the reviewed display-command source guard

- Restore the bounded direct-command recognizer, deny-only hook adapter, and
  source-only managed command matcher selected by the independent review.
- Preserve `agentguard.SafeRun`'s existing resource preflight while making the
  display-command denial non-bypassable by `Force` or ignored health checks.
- Keep shell command operands distinct from positional arguments, retain empty
  quoted arguments, and share a fixed parser-depth budget across substitutions
  and nested wrappers. Add inert parser, adapter, and matcher fixtures.
- No hook installation or activation, system-setting change, native display or
  power command execution, new issuer, custody store, or authority change is included.
  The source fixtures do not establish installed-route or GUI-action coverage.

Refs: `docs/ADR-040-DO-NO-HARM-TO-THE-RUNNING-HOST.md`,
`docs/ADR-119-SLEEP-CUSTODY-PRODUCTION-CONTROLLER.md`, original
`display-parser-cure-0312`, independent review of
`e6bcb1941de732f9de162f204a694e4fc3c295b7`.
