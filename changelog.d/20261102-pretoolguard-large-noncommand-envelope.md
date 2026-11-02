Allow complete managed pre-tool hook envelopes up to a local 8 MiB cap while preserving cap+1 rejection, full JSON framing, and command/cmd checks for every tool name. This is a local resource policy, not an emitter maximum.

Refs: `cmd/sirsi-pretoolguard/README.md`, `docs/ADR-119-SLEEP-CUSTODY-PRODUCTION-CONTROLLER.md`
