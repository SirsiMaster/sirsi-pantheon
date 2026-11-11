- Default fabric network transport to the existing strict-pinned direct path only
  when all six recognized runtime proxy values are absent or empty. Keep explicit
  SOCKS mode and configured-proxy refusal unchanged; never retry through direct
  transport after a proxy failure. Add private plan-only causal fixtures; runtime
  qualification and installed adoption remain separate.

- Refs: `scripts/maat/fabric-run.sh`, `scripts/maat/fabric-run.test.py`.
