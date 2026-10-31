test(core): make Apollo inventory fixture modes umask-independent

Set explicit modes on the generated signature directory, CodeResources file,
and Apollo profile in the two positive inventory fixtures. A private caller
umask can no longer turn these fixtures into unintended negative cases; product
mode validation and all existing assertions remain unchanged.

Refs: cmd/sirsi-package-inventory/main_test.go;
internal/packageinventory/inventory_test.go; scripts/artifact_modes.py;
scripts/PACKAGING.md; ADR-032.
