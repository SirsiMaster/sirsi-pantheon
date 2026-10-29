### Fixed

- Observe local Darwin process groups through errno-checked libproc calls with
  same-executor positive controls and zombie-aware metadata, instead of spawning
  sandbox-denied ps during cleanup. Preserve uncertainty/debt and exact owned
  child signaling/reap fences; no missing read is treated as absence.
