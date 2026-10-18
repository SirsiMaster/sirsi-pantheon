# Security and threat model — Ra / Horus

| Threat | Control | Failure signal |
|---|---|---|
| split-brain local ledger | `Resolve` fails closed; service is canonical | local regular `router.db` |
| forged or stale identity | host token, session, runtime hash, nonce, thread binding | 4xx/auth receipt |
| duplicate claim/replay | database fencing, idempotency keys, lease expiry | existing id / `ErrLeaseInvalid` |
| body loss or shell substitution | required non-empty body and `@file` path | send refusal |
| wake storm or blind spawn | bounded wake event leases, progress gate, session-mode strategy | terminal wake failure |
| token spread to sandboxes | per-host relay, refusal list, secret-free logs | relay refusal/audit |
| overbroad worker network access / SSH substitution | runtime proxy exact enrolled-IP rules; no broad private binding/upstream/UDP; strict existing pins, public-key-only auth and POSIX-literal argv; no direct fallback | outside-allowlist denial, missing proxy/trust/auth refusal; destination executor/admission absent |
| copied/forked router DB | origin/service identity and doctor check | source/registry drift |
| false liveness | PID/heartbeat separated from consumer/read-ack state | stale/unacknowledged lane |
| release drift | stamped source, binary/image hash, traffic and rollback receipt | release check failure |

Credentials and remote transport trust remain environment-owned. No Stack Lab
document authorizes exporting tokens, copying live databases, weakening SSH/TLS,
or adding a second router service.
