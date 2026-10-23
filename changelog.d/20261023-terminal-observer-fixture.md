### Fixed
- Run the terminal wake observer lifecycle assertion against an initialized, retained canonical SQLite store, with retryable terminal transitions and coverage that stale heartbeat snapshots cannot overwrite committed terminal state. Legacy JSON snapshots remain unfenced compatibility data; this fixture does not change their behavior.
