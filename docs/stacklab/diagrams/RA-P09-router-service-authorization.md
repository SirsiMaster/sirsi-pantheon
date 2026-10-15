# RA-P09 — Router service authorization (host token, thread binding, audience log)

Owner: Ra. Source: `internal/routerstore/serve.go`, `server.call` (lines 343-498). Revision: main `b29778fc`.

## Logical view
```mermaid
flowchart TD
  CALL[Gated call to /v1/call/Method] --> HOST{hostForBearer: resolve bearer to host}
  HOST -->|DB failure| E503[503 — outage, not a credential failure]
  HOST -->|ok| AUTH[authenticate: session, signature, nonce, runtime]
  AUTH -->|fail| E401A[401]
  AUTH -->|ok| BIND{sameIdentity: token host == session host?}
  BIND -->|no| E401B[401 — stolen session on wrong host's token]
  BIND -->|yes| ROR{ruleOfRa: thread bound, registered, live heartbeat}
  ROR -->|unregistered, enforce mode| E401C[401 ErrUnregistered]
  ROR -->|log mode| LOGONLY[log would_refuse, continue]
  ROR -->|ok| AUDIT[RecordAudience: verdict written BEFORE mutation]
  AUDIT -->|log write fails| E503B[503 — mutation audit can't see must not happen]
  AUDIT -->|ok| THAUTH[threadAuthority: register/rewrite/resume/delete scoped to caller's own host]
  THAUTH --> RUN[Run the Store method]
```

## Data view
```mermaid
flowchart LR
  REQ[incoming call] --> TOKEN[(host_tokens table)]
  REQ --> SESSION[(session: host, runtime hash, agent)]
  SESSION --> THREAD[(thread registry: ThreadID, live state, heartbeat)]
  REQ --> AUDIENCE[(audience_log: verdict allowed/would_refuse/refused)]
  AUDIENCE -->|written first| MUTATION[Store method mutation]
```

## Failure and recovery
- Every refusal branch names its failure mode distinctly (503 for infrastructure outage vs. 401 for a genuine credential/binding problem) so an outage is never misread as an attack, and vice versa.
- `sameIdentity` is the cross-check that stops a session secret stolen from host A from riding host B's legitimate token — host binding, not just session validity, is required.
- `RuleOfRa` has three modes (`off`/`log`/`enforce`); `enforce` is the only mode that actually refuses on `would_refuse` — operators can run `log` to observe before cutting over.
- The audience log write happens *before* the mutation runs, and a failed audit write refuses the call rather than letting an unaudited mutation through — audit-then-act, not act-then-audit.
