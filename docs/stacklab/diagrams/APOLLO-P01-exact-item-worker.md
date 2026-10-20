# APOLLO-P01 — Exact-item bounded Apollo worker

Owner: Pantheon. Recipe: `stacklab.recipe.apollo-2.1-bounded-tool-turn`.
Source boundary: `internal/routerstore/lease.go`, `cmd/sirsi/gemma_worker.go`.

```mermaid
flowchart TD
  A[Lead-owned gemma-pantheon worker_answers declaration] --> B[Worker reads declaration]
  B -->|absent| X[Refuse before claim]
  B -->|present| C[Register exact worker thread with answer_role]
  C --> D[Read back exact thread record]
  D -->|missing or false| X
  D -->|stamped| E[ClaimItem agent + exact --item]
  E -->|not open, wrong recipient, dependency/budget/breaker refusal| Y[No fallback claim]
  E -->|fenced lease| F[One observe-tier Apollo tool offer]
  F --> G[One provider tool call]
  G --> H[Typed registry read-only result]
  H --> I[One final answer with factual tool evidence]
  I --> J[Router verifies live lease and stamped answer role]
  J -->|accepted| K[Nonbinding draft reply and fenced completion]
  J -->|denied| L[Retain retryable unresolved evidence]
```

The declaration is not self-service: the worker does not alter the registry.
`ClaimItem` is selection-only; it retains the ordinary dispatch eligibility and
lease rules. The client requires a router service that implements `ClaimItem`;
older `ClaimNext` consumers continue to use their existing API.
