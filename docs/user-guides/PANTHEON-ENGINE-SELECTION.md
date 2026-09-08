# Pantheon local engine selection

Pantheon's `sirsi-gemma` tools now keep one user workflow across three local
engines: SNE, MLX, and OMLX. The MCP tool names remain `gemma_chat` and
`gemma_complete`; only `~/.config/sirsi/gemma.toml` changes.

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

## Friday demo operator path

The Pantheon-owned operator path is the local dashboard, started with:

```sh
sirsi dashboard
```

The dashboard's Engine view reads `/api/engine`. Choose SNE (or another
configured connector) with the `Use …` button; the selection is held by the
single engine router, not by the browser. The command bar then submits the
question through `/api/ask`, which keeps the live-diagnostics grounding and
renders findings from Pantheon-owned data. When the canonical engine selection
controller is configured, that request is completed through the selected
connector and its receipt records the route. Deployments without the
controller retain the legacy loopback SNE bridge as an explicit compatibility
fallback.

The dashboard connector is configured with the process environment, not the
`gemma.toml` keys above. A minimally identity-bound SNE setup supplies
`SIRSI_SNE_ENDPOINT`, `SIRSI_SNE_MODEL`, `SIRSI_SNE_ENGINE_VERSION`,
`SIRSI_SNE_MODEL_SHA256`, `SIRSI_SNE_TOKENIZER_ID`,
`SIRSI_SNE_TOKENIZER_SHA256`, `SIRSI_SNE_PRECISION`, and
`SIRSI_SNE_CACHE_NAMESPACE`; `SIRSI_ENGINE_PREFERRED=sne` selects it. The
endpoint is not probed while the dashboard starts; the first submitted prompt
performs session admission and returns an actionable readiness error if SNE is
not available.

There is no separate `sirsi engine select` CLI today. Connector selection is
configured with the `SIRSI_*` environment contract above and changed during a
running dashboard session through the Engine view. Do not start a model
workload merely to inspect the selection snapshot; availability is proved only
when an operator submits a prompt.

Existing configurations that set `sne_url` without an `engine` key continue to
select SNE. Configurations without any engine or endpoint selection continue to
use MLX.
