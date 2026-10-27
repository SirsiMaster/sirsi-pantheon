### Added
- **ADR-079 (PROPOSED): wake lanes are the interactive sessions.** This records the owner directive of 2026-10-26. It names the three defects that let Ra's wake loop start headless workers beside its live attended session:
  - the attended probe needs a watch loop that ADR-065 removed;
  - Ra's registry entry has no `attended` fence;
  - the consumer prompt defines a resident loop rather than a one-pass job.
- It also sets out a staged, Ra-first rollout with a pre-registered test. This is design only: no code or registry change is included.
