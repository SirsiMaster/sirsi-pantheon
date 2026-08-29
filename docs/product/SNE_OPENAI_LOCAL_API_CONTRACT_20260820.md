# SNE OpenAI-Compatible Local API Contract

**Status:** implemented baseline, qualification pending  
**Scope:** Pantheon-governed local SNE serving only

Pantheon exposes `GET /v1/models` and `POST /v1/chat/completions` for local clients such as Nexus. The boundary is deliberately narrow and fail-closed:

- Only an exact active model/runtime tuple marked `release-supported` by the verified signed support matrix is discoverable or callable.
- Model discovery includes runtime, signed-catalog, device, execution-mode, precision, qualification, and support-status provenance.
- Pantheon validates the model identity and request-size boundary, then forwards the original JSON body byte-for-byte. OpenAI-compatible fields such as messages, stop sequences, structured-output format, token limits, sampling controls, and streaming options therefore remain owned by the admitted SNE runtime rather than being silently rewritten.
- Streaming responses are relayed incrementally with buffering disabled. Non-streaming JSON is preserved.
- Client cancellation propagates through the request context to the loopback-only upstream request.
- The upstream is fixed to a loopback address; proxy environment variables are disabled.
- Errors use an OpenAI-compatible envelope plus a privacy-safe `sne` extension with `no_fallback: true`, recovery guidance, support status, and the next required gate when applicable.
- Pantheon never falls back to Python, CPU inference, cloud inference, another model, another precision, or another framework.

This contract does not itself prove response quality, long-context behavior, queue fairness, sustained throughput, or release readiness. Those remain separate realistic-workload and qualification gates.
