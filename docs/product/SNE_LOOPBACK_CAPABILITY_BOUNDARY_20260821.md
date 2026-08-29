# SNE Loopback Capability Boundary

Date: 2026-08-21

## Decision

Pantheon is the sole local control boundary for SNE inference, lifecycle, catalog mutation, model installation/removal, support-bundle export, and governed recovery. A listener bound to `127.0.0.1` is necessary but insufficient.

Pantheon now applies two independent controls:

1. Every SNE, OpenAI-compatible, and recovery route validates that the HTTP `Host` authority is canonical loopback (`localhost`, `127.0.0.0/8`, or IPv6 loopback) with a valid numeric TCP port when present. This blocks DNS rebinding before a route handler runs.
2. Inference and state-changing routes support a constant-time bearer capability check. The capability is generated from 256 bits of system randomness, stored as a regular non-symlink file with mode `0600` under a mode-`0700` Pantheon application-support directory, and reused across restart.

Pantheon Menubar enables the capability and launches Nexus with a URL-fragment handoff. If durable capability storage fails, Pantheon uses an unexported random in-memory credential so protected operations fail closed rather than reverting to unauthenticated operation.

Authenticated `POST /api/sne/access/rotate` atomically replaces the private token file and the in-memory credential. The response is non-cacheable and returns the replacement only to the authenticated caller; the prior credential is rejected immediately.

The user-facing CLI exposes the same contract without accidental disclosure:

- `sirsi sne access status` reports readiness and path but never the token;
- `sirsi sne access token --reveal` requires explicit disclosure intent for OpenAI-compatible clients;
- `sirsi sne access rotate --confirm` rotates through the running Pantheon server, verifies the durable replacement, and explains that existing sessions were revoked;
- `sirsi sne open` launches Nexus through the fragment handoff.

## Protected operations

- `/api/sne/chat`
- `/v1/chat/completions`
- SNE start, stop, install, remove, catalog install/rollback/remove
- SNE support-bundle export
- Pantheon recovery restart and resume

Read-only readiness, lifecycle, catalog-update discovery, diagnostics, model discovery, and recovery status remain loopback-host bounded so diagnostics can explain a pairing failure.

## Evidence

Focused Go tests prove:

- hostile `Host` plus matching hostile `Origin` is rejected before the handler;
- malformed, remote, and ambiguous authorities are rejected;
- canonical IPv4, IPv6, and localhost authorities are accepted;
- missing credentials return `401` and invalid credentials return `403`;
- the exact bearer credential reaches the configured handler;
- browser preflight remains available;
- token creation is durable and mode-bounded;
- symlink, world-readable, and malformed token files fail closed;
- typed Pantheon diagnostics exclude capability headers, token fields, and token filenames;
- the real policy-v8 support exporter excludes a capability canary supplied both as a private Application Support file and an environment value;
- registered production routes, not only helper functions, enforce the boundary;
- existing SNE chat, lifecycle control, recovery, OpenAI proxy, and model-removal suites remain green.

## Claim boundary and remaining gates

This closes DNS rebinding and unauthorized callers that do not possess the local capability. It does not claim isolation from a malicious process already running as the same macOS user with permission to read that user's private application-support files. Stronger same-user isolation requires a native signed-client/Keychain or audit-token boundary.

Before GA:

- exercise the proven rotation/revocation API against a live Nexus session;
- test hostile origins, missing/invalid credentials, preflight, and reconnect end to end;
- decide and document the signed-native-client/Keychain boundary for stronger same-user protection;
- include the token store in upgrade/uninstall policy without ever including its secret in diagnostics or benchmark packets;
- mirror this canon into the native Sirsi Workspace.
