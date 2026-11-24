# Nonroot service image inputs

Router and board context builders explicitly set the generated binary to 0755
and only the copied public registry, machine-list and changelog assets to 0644.
Private context directories and unrelated files are not widened. Both static
builds require `-buildvcs=true` instead of permitting automatic omission.

Inert-compiler fixtures exercise both actual context functions under umask 077,
including private-file preservation and build argv. They are not real compiler,
container startup, deployment or installed-runtime qualification.

Refs: docs/router-service/RUNBOOK.md, docs/user-guides/router-board.md, ADR-062
Diagrams: unchanged — packaging and provenance only
ADR: not required — correct existing distroless nonroot packaging contract
