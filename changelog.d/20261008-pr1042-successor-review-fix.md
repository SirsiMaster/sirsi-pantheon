- Closes codex-pantheon's successor CHANGES_REQUESTED on PR #1042 (head
  `0b1184c8`): `Board.Snapshot()` + `Board.Valid()` were two separate RLock
  acquisitions, so a `Poll` landing between them could pair a stale/invalid
  body with the next poll's valid flag and serve 200 with a fabricated
  payload (P1); the stream handler wrote the `200` status before checking
  validity, so an invalid producer still established an apparently-healthy
  stream instead of the documented 503 (P2). Adds `Board.SnapshotState()`
  (payload+version+valid under one lock) and routes `snapshot`/`slice`/
  `stream` through it exclusively; `stream` now checks validity before
  `WriteHeader` and emits an explicit `event: invalid` if an established
  stream's producer later fails.
