# Provider and Model Router

`provider.Provider` is the transport seam. `OpenAICompat` implements the
existing SNE and configured remote transports. `Decide` is the pure, typed
policy engine, while `ModelRouter.Run` qualifies lanes, executes the decision,
applies an allowed fallback, and writes an observable decision record.

Policy is capability-qualified, not comparative. A zero capability value means
unknown and cannot satisfy a declared minimum. Privacy is a hard boundary: an
explicit remote override cannot override `local-only`. Local SNE readiness must
use `ProbeCompletion`; `/health`, a socket, a process, or `/models` alone is not
an inference proof.

Add transports by implementing `Provider`. Keep probe behavior injectable via
the provider instance, expose only capabilities established by an authoritative
wire contract or conformance test, and add table-driven policy plus execution
tests. Do not add credentials: the remote adapter continues to use the existing
orchestrator configuration and environment variables.
