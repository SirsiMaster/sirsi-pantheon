- fix(maat): the loaded-regime own-lane traffic exemption in
  `CheckConflicts` now also requires `PID == 0`, closing the gap where an
  explicitly foreign PID on the reserved lane (Owner empty) was silently
  cleared instead of reported. Codex PR1027 round-2 review finding.
