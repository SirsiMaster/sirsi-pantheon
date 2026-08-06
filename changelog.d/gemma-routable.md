- fix(router): gemma was unreachable from the router in two independent ways —
  the worker polled identity `gemma`, which is absent from agents.json, while the
  registered identity `gemma-pantheon` carried `wake.mechanism: cli-spawn`, which
  the ADR-054 validator refuses. Both doors were shut, so the local model could
  never be given work and every task fell through to a paid API agent.
