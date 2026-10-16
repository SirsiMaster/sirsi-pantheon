### Changed

- Apollo's installed native profile can admit one local whole-model rank through
  the same Ma'at authority and native backend. The M5 laptop profile places its
  31B Q8 model locally after matched measurements showed serial heterogeneous
  placement was the principal decode cost. Distributed profiles remain supported.
- Local profiles refuse coordinator, transport-edge, or raw-rank residue. The
  native SDK retains whole-model extent checks and owned worker teardown.

Refs: `cmd/sirsi/apollo_native.go`, `internal/apollo/maatauthority.go`,
Apollo Stack Lab `docs/apple-stack-lab/evidence/apollo31-placement-cost-20261015/RECEIPT.md`.
