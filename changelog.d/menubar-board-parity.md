- feat(board): `sirsi board-serve` — the Go router board, replacing the
  out-of-repo Python server.py. Same UI, same URL, verified field-identical
  side by side before cutover.
- fix(menubar): reads `board-serve --once --shape fleet` — a PROJECTION of the
  board's own payload, not a parallel aggregation. It previously called
  `router fleet --json`, which counts blocked separately while the board treats
  it as a subset of active, so the two disagreed by construction.
- chore: retire the 9119 Horus dashboard (duplicated the menubar) and the
  token-burning auth probe in sirsi-router-board.sh, which launched a real
  Claude session per agent per refresh and caused the "gtimeout wants to access
  data from other apps" prompt.
