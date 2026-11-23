### Fixed
- `WithdrawTask` no longer wipes a live lease: withdrawal is refused with `ErrLeaseHeld` (holder and expiry named) while any unexpired lease exists, enforced in the UPDATE predicate so a claim landing mid-withdraw cannot race it. The wire API cannot identify the caller as the holder, so any live lease refuses; expired or absent leases still withdraw. `ErrLeaseHeld` crosses the wire as a named sentinel.

Refs: `internal/routerstore/receipts.go`, `internal/routerstore/lease.go`, `internal/routerstore/remote.go`
