# Host readiness observer

Source of `ai.sirsi.host-readiness-watch`. Before this change it existed only as
installed copies under `~/Library/Application Support/Sirsi/HostReadiness/resources/`
(SHA 085045, 093848).

- `host-readiness-watch.zsh`: the launchd entry point (every 60s). Observation
  only: it never relaunches Tailscale, unlocks a session or restarts a service.
- `unattended-availability-monitor.py`: probes SSH (22) and Screen Sharing
  (5900) on `--host`, classifies the result, and publishes a receipt.

## Where readers find the latest state

`current.json` in the state root. It is rewritten atomically on every run, so it
is always the latest observation.

## Retention

`--history changes` (what the watch script passes) writes a run receipt under
`receipts/<run-id>/` only when the admission state or the GUI lock state differs
from `current.json`. A missing or unreadable `current.json` counts as a change.
`--history all` (the default) keeps the old behavior: one directory per run,
about 1,440 a day. On the M1 that reached 96,740 directories (378 MB) between
2026-08-28 and 2026-11-10.

Existing receipt directories are not touched. Deleting history is a separate,
explicit decision.

## Tests

```bash
python3 scripts/hostreadiness/test_monitor.py
```
