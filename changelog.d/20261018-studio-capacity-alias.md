## Unreleased — Studio capacity alias compatibility

Bare studio/m5-studio scheduler names share admission, read, conflict, cede and
capacity semantics without rewriting reservation identities or host bindings.
Historical alias capacities are read conservatively; explicit setters update
both. Shared-engine and atomic concurrent admission regression controls added.
Refs: docs/runbooks/maat-atomic-reservations.md, docs/runbooks/maat-studio-alias.md.
