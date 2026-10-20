- Bind the native release build to its actual runner executor and a maintained
  canonical Ma'at CPU grant before compiling, signing or notarizing. Keep Go
  and Swift at the declared single-core cooperative build mask; retain failed
  admission and cleanup receipts instead of treating the legacy rail-lock hook
  as a grant. Default Swift job arguments are valid under macOS Bash 3.2 with
  nounset, without an empty-array expansion. Refs: scripts/maat/README.md.
