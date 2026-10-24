### Fixed — native application Quit menu

- Install a standard AppKit application menu before the programmatic event loop,
  with Quit Pantheon and Command-Q targeting NSApplication.terminate.
- Keep window-close/menu-bar companion behavior, branding, layout and typed
  engine operations unchanged. No new preferences, settings or activation path.
- Add factory menu/shortcut/target tests, fake-target action invocation and launch
  wiring regression coverage. No real app termination or event posting in tests.

Refs: existing native124 empty-app-menu/Command-Q defect; ADR-030; AGENTS.md A35.
Source checkpoint only; main owns admitted build/tests and installed qualification.
