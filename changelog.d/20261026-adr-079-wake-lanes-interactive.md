### Added
- **ADR-079 (PROPOSED, rev 2): wake lanes are the interactive sessions.** This records the owner directive of 2026-10-26. It names the three defects observed on Ra's lane that let its wake loop start headless workers beside its live attended session:
  - the attended probe needs a live watcher process (thread-, predecessor-, or agent-keyed), and none was running for Ra's lane — per-lane watcher retirement under ADR-065 is gated separately and not a fleet-wide fact, so this is Ra's observed case, not a claim about every Claude session;
  - Ra's registry entry has no `attended` fence;
  - the consumer prompt defines a resident loop rather than a one-pass job.
- It also sets out a staged, Ra-first rollout with a pre-registered test, including a close-session negative fixture for the `ConsumerCapable`/`armed` path that D1–D3 do not by themselves resolve. This is design only: no code or registry change is included.
