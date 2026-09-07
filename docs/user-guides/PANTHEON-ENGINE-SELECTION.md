# Pantheon local engine selection

Pantheon's `sirsi-gemma` tools keep one user workflow across four local
engines: SNE, SNE Native v2, MLX, and OMLX. The MCP tool names remain `gemma_chat` and
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

## SNE Native v2

The recovered SNE Native v2 release candidate is selected through its local
OpenAI-compatible service. Pantheon does not start, qualify, or manage that
runtime; SNE owns its lifecycle and qualification.

```toml
engine = "sne-native-v2"
sne_native_v2_url = "http://127.0.0.1:11434/v1"
sne_native_v2_model = "gemma-4-12b-it-affine8-sne-v1"
max_tokens = 1024
temperature = 0.7
```

The native-v2 choice is a candidate integration, not a performance claim. A
missing or unhealthy endpoint stays a visible failure; Pantheon never switches
to MLX, OMLX, or another SNE implementation on its behalf.

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
