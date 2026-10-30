### Evidence
- **G3 harness: serve logs no longer record the host's scratch path.** `run.sh` now rewrites the absolute run directory to `<run>` in every `*.serve.log` once the test servers have stopped. The 2026-10-26 raw logs stay as recorded, because they are pinned by `SHA256SUMS`. This addresses Mercury's non-blocking note on #1222. Refs: ADR-062.
