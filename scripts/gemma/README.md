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

## Optional legacy model resolver (not the native worker)

`sirsi-gemma-model-resolver.sh` remains a separately callable compatibility
tool copied by `make install-gemma-worker`; that install target also restarts
the worker. Do not run the installer merely to inspect or test the resolver.
The native worker wrapper does not invoke this resolver. This corrective does
not change the native SNE endpoint, model recipe, registry grant or worker.

The legacy resolver still queries the HF model catalog, reads its own cache
metadata and can run its existing empirical broker probe on a cached non-WEDGE
path. Default-off downloads do **not** make this entire legacy tool offline or
side-effect-free; ordinary non-WEDGE probe/swap behavior is unchanged.

When neither selected nor fallback weights are cached, `WEDGE` preserves the
existing conf byte-for-byte or writes the accepted fallback-name sentinel when
conf is absent, prints that conf and exits **0**. A sentinel is not proof of
available weights. WEDGE never reaches the downloader or empirical probe,
including when an opt-in is supplied. Weight restoration is a separate explicit
operation, not a resolver recovery side effect.

HF downloads are off unless the invoking environment explicitly sets
`GEMMA_RESOLVER_PREFETCH=1` (only literal `1`, not `true`/`yes`). On a non-WEDGE
path this permits the existing background `hf download` for the **cached model
conf actually serves**. A cached fallback does not authorize downloading the
uncached selected candidate. The resolver rechecks that serving cache directory
before launching the downloader; missing tools/cache are logged as skipped.
`HF` can still select the executable; otherwise the existing venv `hf` then
`huggingface-cli` lookup is retained. Do not export the opt-in globally or add it
to an unattended job without deliberately reviewing that legacy operation.

StackLab records this optional dependency and its limits in the
[`Ra/Horus recipe`](../../contracts/stacklab/ra-horus-fabric-recipe-v1.json).
Source qualification, publication, installed-tool upgrade and broader Gemma
delivery remain separate; this does not close the cache or inference parent.

## Verification

```text
scripts/gemma/worker-escalation.test.sh
go test ./cmd/sirsi -run TestGemmaWorker -count=1
bash scripts/gemma/model-resolver-ranking.test.sh
bash scripts/gemma/model-resolver-cache.test.sh
python3 scripts/gemma/model-resolver-lifecycle.test.py
```

The lifecycle fixture runs the entire actual resolver under a private HOME and
command allowlist. Host/catalog/downloader/probe commands are inert doubles;
real Python evaluates only arithmetic/ranking. It joins fake background
downloads before asserting counts. The older extracted cache fixture remains a
conf-contract test, not evidence that downstream lifecycle work is suppressed.
