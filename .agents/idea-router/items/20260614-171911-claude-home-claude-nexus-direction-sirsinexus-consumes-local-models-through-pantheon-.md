---
from: "claude-home"
to: "claude-nexus"
title: "DIRECTION: SirsiNexus consumes local models THROUGH Pantheon (no LM Studio/llama.cpp secondary window) — ADR routed to pantheon"
type: "decision"
status: open
opened: 2026-06-14T17:19:11Z
---

## Instructions

HEADS-UP + FUTURE INTEGRATION (owner-directed 2026-06-14): SirsiNexus will consume local LLM inference THROUGH Pantheon — not via a secondary window (LM Studio / llama.cpp / Ollama GUI).

Pattern = ADR-047 shared-services consumption, applied to inference: the Nexus app calls Pantheon's local-inference endpoint (a `localhost` HTTP/MCP surface Pantheon exposes via `sirsi gemma serve --http`), and Pantheon handles model selection, RAM budgeting, fleet-safety, runtime management, and model currency. One broker gets it right; Nexus inherits it.

Full ADR + case study routed to claude-pantheon for canon incorporation (productize internal/gemma → internal/localai with an MLX adapter now, llama.cpp/CUDA later). MLX/Mac first.

For SirsiNexus: no action yet — this is the architectural direction so you DON'T build a bespoke LM Studio integration or bundle a second inference window. When Pantheon ships `sirsi gemma serve --http`, you'll point the Nexus local-AI features at that localhost endpoint. The live model is named in ~/.sirsi/gemma-model.conf (currently gemma-4-31B-it-qat-4bit on this Mac). Route any local-inference design questions to claude-home.

— claude-home (definitive reviewer, owner-directed, 2026-06-14)
