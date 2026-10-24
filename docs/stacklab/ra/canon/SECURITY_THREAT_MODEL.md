# Security and threat model — Ra / Horus

| Threat | Control | Failure signal |
|---|---|---|
| split-brain local ledger | `Resolve` fails closed; service is canonical | local regular `router.db` |
| Background poller races selected relay refresh | cooperative shared/exclusive retained-directory admission, finite exclusive wait, unchanged stable queue/instance/children guards, authorization before lock and readback after release | legacy producer adoption missing, timeout, unreadable/nonempty queues or changed pins refuse before retirement; advisory locks do not fence old clients or authorize deleting requests |
| Maintenance contention hides an untrusted spool client | existing trust preflight precedes shared admission acquisition | actionable trust refusal occurs without publishing even while exclusive admission is held; deterministic regression fixture |
| forged or stale identity | host token, session, runtime hash, nonce, thread binding | 4xx/auth receipt |
| duplicate claim/replay | database fencing, idempotency keys, lease expiry | existing id / `ErrLeaseInvalid` |
| body loss or shell substitution | required non-empty body and `@file` path | send refusal |
| wake storm or blind spawn | bounded wake event leases, progress gate, session-mode strategy | terminal wake failure |
| token spread to sandboxes | per-host relay, refusal list, secret-free logs | relay refusal/audit |
| overbroad worker network access / SSH substitution | runtime proxy exact enrolled-IP rules; no broad private binding/upstream/UDP; strict existing pins, public-key-only auth and POSIX-literal argv; no direct fallback | outside-allowlist denial, missing proxy/trust/auth refusal; destination executor/admission absent |
| copied/forked router DB | origin/service identity and doctor check | source/registry drift |
| false liveness | PID/heartbeat separated from consumer/read-ack state | stale/unacknowledged lane |
| registration CAS contention or concurrent identity substitution | bounded fresh-load/rederive retry pinned to the selected agent/host/machine/surface/session/PID/start, with terminal/suspended refusal and foreign-write preservation | explicit exhausted fence or changed/retired binding; no silent success, sibling adoption or stale-delta replay |
| release drift | stamped source, binary/image hash, traffic and rollback receipt | release check failure |

Credentials and remote transport trust remain environment-owned. No Stack Lab
document authorizes exporting tokens, copying live databases, weakening SSH/TLS,
or adding a second router service.
