# Pantheon local engine selection

Pantheon's `sirsi-gemma` tools keep one user workflow across three local
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

Existing configurations that set `sne_url` without an `engine` key continue to
select SNE. Configurations without any engine or endpoint selection continue to
use MLX.
