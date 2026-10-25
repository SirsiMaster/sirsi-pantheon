### Fixed

- Bind the non-executing release source preflight to the current unified
  app-update dispatch and typed retained-preflight, durable attempt, Installer
  handoff and installed-readback recovery owner instead of removed help prose.
  Add causal missing-control fixtures; source presence does not prove installed
  update/rollback or call ordering. Complete the strict source-screen JSON
  fixture with its required empty host_binding while preserving source/host
  separation and all required-key refusal cases.
