# Pantheon local engine selection

The `sirsi-gemma` MCP tools keep one workflow across three local engines: SNE,
MLX, and OMLX. The MCP tool names remain `gemma_chat` and `gemma_complete`;
their configuration is in `~/.config/sirsi/gemma.toml`. The Pantheon dashboard
and `sirsi engine prompt` use the separate process-environment contract below.

## SNE

```toml
engine = "sne"
sne_url = "http://127.0.0.1:11434/v1"
sne_model = "gemma-4-12b-it"
max_tokens = 1024
temperature = 0.7
```

## MLX

```toml
engine = "mlx"
model_id = "mlx-community/gemma-2-27b-it-bf16-4bit"
venv_path = "~/.venvs/mlx"
max_tokens = 1024
temperature = 0.7
```

## OMLX

Start the local OMLX OpenAI-compatible server for the desired model, then use:

```toml
engine = "omlx"
omlx_url = "http://127.0.0.1:8000/v1"
omlx_model = "gemma-4-12b-it"
max_tokens = 1024
temperature = 0.7
```

Pantheon does not silently fall back to another engine. A missing or unhealthy
selected endpoint leaves the tools visible but returns an actionable error. This
keeps backend changes observable while preserving the same chat and completion
contract for users.

## Working product operator path

The Pantheon-owned operator path is the local dashboard, started with:

```sh
sirsi dashboard
```

The dashboard's Engine view reads `/api/engine`. Choose SNE (or another
configured connector) with the `Use …` button; the selection is held by the
single engine router, not by the browser. The command bar then submits the
question through `/api/ask`, which keeps the live-diagnostics grounding and
renders findings from Pantheon-owned data. The canonical engine selection
controller completes the request through the selected connector and binds the
actual route to its receipt. If that controller is unavailable, `/api/ask`
reports the engine path as unavailable; Pantheon does not switch to a second
provider path.

The dashboard and CLI connectors are configured with process environment
variables, not the `gemma.toml` keys above. Configure each SNE route under its
own prefix: `SIRSI_SNE_PLAIN` for `sne-plain`, or `SIRSI_SNE_MTP` for
`sne-mtp`. For example, the plain route requires `<PREFIX>_ENDPOINT`,
`_MODEL`, `_ENGINE_VERSION`, `_MODEL_SHA256`, `_TOKENIZER_ID`,
`_TOKENIZER_SHA256`, `_PRECISION`, `_CACHE_NAMESPACE`, `_TOKEN`,
`_RUNTIME_SHA256`, `_NATIVE_RUNTIME_SHA256`, and `_MANIFEST_SHA256`. The MTP
route requires the same fields plus `_ASSISTANT_MODEL_ID`,
`_ASSISTANT_REVISION`, `_ASSISTANT_CHECKPOINT_SHA256`, and
`_ASSISTANT_PRECISION`. Set `SIRSI_ENGINE_PREFERRED` and
`SIRSI_ENGINE_PREFERRED_VARIANT` together to choose the initial dashboard
policy; for example, `sne` and `sne-plain`. The SNE readiness and status APIs
must both identify
`sne.openai-chat.v3`. Pantheon sends an explicit `execution_mode` for every
completion and accepts it only when the response echoes the requested mode and
the model/runtime/manifest identities. The endpoint is not probed while the
dashboard starts; the first submitted prompt performs session admission and
returns an actionable readiness error if SNE is not available.

In the dashboard, these SNE choices are labeled **Apollo (Plain)** for
`sne/sne-plain` and **Apollo Flash (Speculative)** for `sne/sne-mtp`. The
dashboard also shows the exact route ID, and command flags and receipts retain
the canonical identifiers. Apollo Flash remains unavailable unless the live
readiness and assistant-identity checks pass.

The Pantheon CLI exposes the same route contract for one-shot and streamed
operator requests:

```sh
sirsi engine status
sirsi engine prompt --engine sne --variant sne-plain --prompt "What should I address first?"
# Only with a v3 readiness contract and the exact configured assistant identity:
sirsi engine prompt --engine sne --variant sne-mtp --prompt "What should I address first?"
sirsi engine prompt --engine mlx --variant mlx-raw --prompt "Inspect this local task"
sirsi engine prompt --engine mlx --variant mlx-patched --prompt "Inspect this local task"
sirsi engine prompt --engine omlx --variant omlx-public --prompt "Inspect this local task"
# For a confirmed SSE-capable MLX endpoint, after explicitly enabling streaming:
SIRSI_MLX_RAW_STREAMING=true sirsi engine prompt --engine mlx --variant mlx-raw --stream --prompt "Summarize the current findings"
```

Use the variant configured for the selected engine. `engine prompt` selects the
route for that invocation, returns JSON containing the completion and
identity-bound receipt, and does not silently rewrite the persistent
environment policy. With `--stream`, stdout is newline-delimited JSON: the
first record binds the opened session and route; subsequent records carry
ordered events. A completion record carries the served model identity and
finish reason, and is accepted only when its model matches the model admitted
for the session; terminal receipts remain bound to that session. The stream
ends in a completion or terminal error event; cancellation is reported as an
error with a cancellation receipt, and provider, identity, and transport
failures remain explicit terminal errors. A connector-provided cancellation
receipt is preserved when available. Every
generation route must declare receipt support. Streaming additionally requires
declared streaming and cancellation support; Pantheon fails explicitly rather
than opening a session with weaker guarantees. OpenAI-compatible MLX and oMLX
routes advertise streaming only when their `_STREAMING` setting is explicitly
`true`; it defaults to disabled. A provider that does not declare streaming
cannot be selected for a streaming request.
Ctrl-C or SIGTERM cancels an in-flight prompt through the same request context;
streaming reports the resulting terminal cancellation event when the output
consumer remains available.

The `sne-plain` and `sne-mtp` variants are explicit execution modes. SNE
readiness must advertise the selected mode in `capabilities.execution_modes`;
the capability is not inferred from configuration or the endpoint. When MTP is
advertised, readiness must also provide the exact assistant identity
`{model_id, revision, checkpoint_sha256, precision}`. Configure the expected
assistant under the selected MTP prefix using `_ASSISTANT_MODEL_ID`,
`_ASSISTANT_REVISION`, `_ASSISTANT_CHECKPOINT_SHA256`, and
`_ASSISTANT_PRECISION`. Pantheon binds that tuple into the session/receipt
identity and rejects a missing or different readiness/response tuple. Until
the service provides the v3 contract and matching MTP identity, that route
fails closed; there is no automatic mode switch or downgrade.
The dashboard selection is changed during a running session through
the Engine view. Availability is proved when a prompt opens a session, not by
the selection snapshot alone.

Existing configurations that set `sne_url` without an `engine` key continue to
select SNE. Configurations without any engine or endpoint selection continue to
use MLX.
