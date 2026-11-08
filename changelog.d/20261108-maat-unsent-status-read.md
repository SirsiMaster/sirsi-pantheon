- **Ma’at unsent status-read recovery (source candidate):** return a specific
  no-queue error for newly submitted reads proven never sent, and permit the
  running supervisor to retry only its own exact reservation read within the
  previously proven deadline. Denials, prelaunch failures and uncertain send
  outcomes remain fail-closed. Bounded mock/private-spool and narrow race
  controls passed; native transition, mixed-version qualification and the
  separate legacy-read/mutation-custody holds remain open. No installed repair
  is asserted. Refs: `docs/stacklab/MAAT_READ_RELAY_COMPATIBILITY_DRAFT_20261107.md`.
