### Corrected

- Route the actual router MCP work-board handler's durable item and thread reads through its instance-bound store and the existing shared projection/loaders. Preserve public board shape, pace calculations, best-effort peer loading and legacy file-mode behavior.
- Keep post-registration heartbeat/close on the registry mode selected for the connection: explicit StoreWake=0 remains the existing JSON compatibility lifecycle with exact owned-target fencing, not forced durable resolution or authenticated RPC isolation. Add actual board-handler audience assertions and real owned JSON registration/heartbeat/EOF-close/sibling-preservation fixture source. No tests, build, runtime cutover or installed completion is claimed.
