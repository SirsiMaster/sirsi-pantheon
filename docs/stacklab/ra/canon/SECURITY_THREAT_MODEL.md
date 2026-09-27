# Security and threat model — Ra / Horus

Threats include duplicate claims, stale leases, forged worker identity, cross-lane routing, replayed results, wake storms, copied router stores, and status derived from a live PID alone. Controls are fenced leases, task/thread binding, idempotency, durable wake events, provider conformance, evidence-backed completion, and one canonical router store. Credentials and remote transport trust remain environment-owned.
