# Managed command display guard

This source-only adapter reads one bounded hook envelope and checks string
`command` and `cmd` fields regardless of tool name. It refuses recognized direct
display-power operations and never executes the supplied payload. Input is
limited to 64 KiB; malformed or trailing JSON and read failures are refused.
Valid non-command payloads pass through this narrow command check.

The matching recognizer handles selected direct commands, common execution
wrappers, and command substitutions with a shared recursion bound. It is not a
general shell parser or sandbox. Denial is independent of hook metadata and
cannot be bypassed through `sirsi agent safe-run --force` or ignored health
checks. Read-only commands remain outside the deny set.

This source correction does not install or activate a hook, modify system
settings, or add authority. `scripts/stacklab/managed-requirements.toml` is a
source-only matcher fixture; it does not prove an installed route invokes this
adapter. Coordinate-only GUI actions and unmanaged command surfaces remain
outside its coverage. ADR-119's typed sleep transaction remains the separate
authority for governed sleep; this guard does not create an issuer, grant,
custody store, or native actuator.

## Source and fixture checks

The adapter fixtures cover bounded input, malformed envelopes, both command
fields, tool-name independence, and refusal before a fake dispatch boundary.
Parser fixtures cover direct and wrapped invocations, argument positions,
empty quoted arguments, quoted substitution syntax, and depth exhaustion. The
`SafeRun` fixture uses only an owned harmless executable and verifies that
force and ignored-check options do not bypass a command-safety denial. None of
these fixtures executes a native display or power command.

This source-level correction does not qualify an installed hook, a full shell
containment boundary, GUI-action policy, or the combined display lifecycle.
