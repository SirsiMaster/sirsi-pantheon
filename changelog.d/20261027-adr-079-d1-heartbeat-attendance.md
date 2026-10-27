### Fixed

- ADR-079 D1: `sirsi thread attended <agent>` (and the wake hold and worker deferral that share it) now credits a heartbeat-fresh interactive `claude`/`codex` session as attended. It previously also required a watch-loop process that ADR-065 retired, so no Claude lane could ever read as attended and a headless worker answered beside the live session. Stale, suspended, closed, worker and resident records still never count.
