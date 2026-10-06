### Fixed

- **The release train treats the M5 as best-effort and reports it honestly.** A failed M5 step used to be silently ignored while the train still printed "RELEASED ... on M1 and M5, loops restarted" (v0.24.93: the `m5` hostname stopped resolving mid-run). The M5 upgrade and loop restart now try the hostname, then its LAN address (`M5_FALLBACK`), never stop the release, and the final line says what actually happened: upgraded and restarted, reached but not on the version, or unreachable with the commands to run when it is back.
