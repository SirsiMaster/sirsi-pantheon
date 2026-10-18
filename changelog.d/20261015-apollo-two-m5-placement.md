### Changed

- Use the measured Studio-to-M5 placement for the local fleet's31B Q8 profile.
  Moving the first eight layers from M1 to Studio improved matched distributed
  decode from9.55 to11.88tok/s while retaining an approximately8-second native
  first-token result. The one-M5 comparison had faster steady decode but an
  observed75-second installed first-token delay, so it is not the default chat
  placement under these host conditions. No native or client code changes.

Refs: `cmd/sirsi/apollo_native.go`; Apollo Stack Lab
`docs/apple-stack-lab/evidence/apollo31-placement-cost-20261015/RECEIPT.md`.
