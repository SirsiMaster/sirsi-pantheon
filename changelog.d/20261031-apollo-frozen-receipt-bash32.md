fix(core): read frozen Apollo digest receipts safely in Bash 3.2

Both release builders now capture and check the receipt parser's exit status
before extracting the exact Info.plist and Apollo profile digest pair. The
existing two-key schema, lowercase SHA-256 validation, and disabled `absent`
profile value remain required; malformed, incomplete, unreadable, or
unparseable receipts stop before signing or PKG construction.
The inert Bash 3.2 regression exercises both complete reader blocks with valid
enabled/disabled receipts and space-containing paths, plus malformed and
parser-failure refusals.

Refs: scripts/PACKAGING.md; scripts/build-dmg.sh; scripts/build-pkg.sh;
scripts/test_artifact_modes.py; internal/maat/releasepreflight_test.go;
docs/evidence/APOLLO_NATIVE_PACKAGE_COMPOSITION_20261015.md; ADR-032.
