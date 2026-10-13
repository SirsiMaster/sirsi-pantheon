# Gemma resident worker

`sirsi-gemma-worker.sh` is a compatibility LaunchAgent entrypoint. It starts
the canonical native consumer:

```text
sirsi gemma worker
```

The native worker is the only owner of Gemma router work. It registers the
`gemma-pantheon` worker thread, exports its actual `SIRSI_THREAD_ID`, holds an
OS singleton, claims each item through the router's fenced lease API, reads and
acknowledges the full item, and heartbeats while native inference is active.

## Inference boundary

The worker uses only the selected **warm native SNE** endpoint. It does not
start a broker, cold-load a model, invoke Python, or fall back to a second
runtime. It verifies that endpoint before claiming any item. If it is absent,
the worker exits without consuming queue work.

Gemma produces nonbinding text drafts. It does not run tools, transfer work,
merge code, publish changes, or issue security/release verdicts. A routed reply
includes the original item identifier and a non-capability correlation digest;
it never exposes a lease token.

## Failure and recovery

Broker errors, empty output, truncated output, reply failure, and start failure
leave the request unresolved with retryable evidence. A lost or superseded lease
cancels generation and prevents publication. A successful response is followed
by token-fenced completion; the notification itself is intentionally marked
nonbinding because it is not atomic with that completion fence.

For operator recovery and evidence interpretation, see
[`docs/user-guides/gemma-worker-recovery.md`](../../docs/user-guides/gemma-worker-recovery.md).

## Verification

```text
scripts/gemma/worker-escalation.test.sh
go test ./cmd/sirsi -run TestGemmaWorker -count=1
```
