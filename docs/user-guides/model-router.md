# Model Router

`sirsi ask` routes model work through a qualified local or configured remote
lane. It does not assume that all models have the same capabilities, and it
never sends a `local-only` request over the network.

The default is deliberately private and local:

```text
sirsi ask --route "summarize the measured machine state"
```

Use a remote lane only for content you explicitly classify as shareable:

```text
sirsi ask --route --task judgment --privacy shareable "assess this evidence"
```

Available routing flags:

- `--task generation|judgment|extraction|embedding`
- `--privacy local-only|shareable`
- `--lane local|remote` for an explicit per-request override
- `--min-context N` to require a qualified context window
- `--route` to continue through the model router even when deterministic
  machine checks already found an answer
- `--json` to include the route decision in machine-readable output

The local lane is the native SNE OpenAI-compatible service. Sirsi reads its
port from `~/.sirsi/gemma-server.port`; it does not hardcode a port. Readiness
requires a successful one-token completion. Because the current SNE release
does not yet publish `/v1/sirsi/capabilities`, its context window is treated as
unknown and a nonzero `--min-context` request fails closed instead of guessing.

Routing decisions are appended to
`~/.sirsi/model-router-decisions.jsonl` with mode `0600`. Each record includes
the declared inputs, selected lane, reason, provider/model provenance, outcome,
and any failure or fallback evidence.
