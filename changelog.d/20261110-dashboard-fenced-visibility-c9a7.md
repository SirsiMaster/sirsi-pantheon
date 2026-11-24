- Keep legacy fleet task timestamps as metadata, not proof of work. `router fleet`
  now reuses the existing canonical fenced operational classification; recent
  claimed work and valid claims remain visible when automatic wake is unavailable.
  Preserve the board's watcher `armed`/`loop` contract and expose routing
  availability separately with consumption explicitly unknown. No autonomous
  answer grant, heartbeat daemon or consumer lifecycle change.

- Refs: ADR-057; existing router consumer-visibility work; reviewed source
  `c9a7d20d734df57d07030efa1c257364bc0cf32a`. Source integration atop Main
  `827ee701445089bd50f0227595b129104fe698e4`; combined qualification remains separate.
