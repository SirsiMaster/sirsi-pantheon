- Protect shared thread registries from cross-machine PID reaping when either
  machine identity is unknown; find the macOS machine identifier even when
  `/usr/sbin` is absent from PATH. Local legacy registries retain their fallback.
  Refs: ADR-022, ADR-024, ADR-036.
