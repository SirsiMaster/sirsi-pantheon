# Pantheon prefix-cache pressure control-plane candidate — 2026-08-28

**Status:** partial, source-proven control-plane candidate; not a release claim.

**Base:** `ede68456f40065e7331a74dd51ef337e10bfc7aa`
(`docs: record M1 supervised recovery proof`), tree
`f9844052388cfeb58c957afd76cd7922772c5cbc`.

## Attributed changes

Pantheon now constructs a fresh, host-bound, five-minute pressure observation
that is wire-compatible with SNE's `PressureObservation`.  The observation has
a cryptographically random request ID, measured RAM/swap fields, pressure
source, and a SHA-256 over canonical Go JSON.  A decision transport value is
accepted only when it names the exact observation SHA.  Missing, stale,
cross-host, altered, or unbound inputs are rejected before any SNE mutation.

Pantheon does not calculate prefix-cache limits, execute cache eviction, claim
replay protection, or create execution/retention receipts.  Those remain SNE
consumer responsibilities.

When an SNE launch is denied by measured resource admission, the lifecycle
state exposes the observation receipt as `prefix_cache_pressure`.  Receipt
creation is observation-only: it neither starts a model nor changes
operator-disabled or quarantine state.

| Path | SHA-256 |
| --- | --- |
| `internal/sne/resource_pressure_receipt.go` | `139078f1a2cecc1a0eddd72d7ef00d4a4d9d077013f38f5b7f94aeea2eba66c4` |
| `internal/sne/resource_pressure_receipt_test.go` | `56478d740ac7d3c999f7bf97b6e143f70f417b1a8a39987671200e2c737018ee` |
| `internal/dashboard/sne_lifecycle.go` | `fe4422723f638317f585fedcc9690fc1be138d7c9dc871a8eb9bc47c9c213ed2` |
| `internal/dashboard/sne_lifecycle_test.go` | `796262211e72b11e74d896fc975ce42f05a904cbafab2f18824b6c6e33019a32` |

## Consumer contract read

Read-only consumer contract: 
`/Users/thekryptodragon/Development/sirsi-native-rebuild/internal/prefixcache/pressure_policy.go`
(SHA-256 `de055b674ce8a025d5fb5682bce562c5a3509407709c1cd8493e56478f83e390`).

The candidate matches the consumer observation JSON keys and its five-minute
freshness maximum.  `pressure-<hex>` request IDs conform to the consumer's
`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$` identity grammar.

## Verification

All commands used an isolated Go build cache and made no live-service change.

```text
GOCACHE=/private/tmp/pantheon-prefix-pressure-gocache go test ./internal/sne -run '^TestPrefixCachePressureReceipt' -count=1
exit 0

GOCACHE=/private/tmp/pantheon-prefix-pressure-gocache go test ./internal/dashboard -run '^TestSNELifecyclePublishesActionableResourceAdmission$' -count=1
exit 0

git diff --check
exit 0
```

## Remaining product acceptance

This candidate does **not** yet provide the required owner-visible authorization
receipt, SNE decision/execution transport, terminal completed/failed/interrupted
receipt projection, retention receipt projection, or the authoritative Swift,
menu bar, CLI/TUI, and dashboard surfaces.  It must not be described as a
prefix-cache mutation capability or as release-ready until those paths are
implemented and verified from an immutable attributed commit.
