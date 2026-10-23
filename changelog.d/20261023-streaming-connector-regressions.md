- Cover truncated OpenAI-compatible SSE through the real HTTP provider and engine
  connector: a model-bound partial delta followed by EOF must end in one error,
  never a completion or receipt. Expand non-finite streaming-request regressions
  to Temperature and TopP with NaN and both infinities, requiring request-validation
  refusal before HTTP transport. Fixtures use only owned loopback HTTP servers;
  production guards and model/service lifecycle remain unchanged.

Refs: PANTHEON_RULES.md A35; docs/QA_PLAN.md;
20260907-164834-sirsi-software-admin-codex-pantheon-ssa-review-streaming-transport-needs-terminal-and-identity-f.
