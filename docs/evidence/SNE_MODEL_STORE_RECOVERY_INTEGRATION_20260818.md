# SNE Model-Store Recovery Integration

Date: 2026-08-18  
Classification: Pantheon platform-foundation evidence  
Status: **PROVEN for startup recovery admission and fail-closed behavior**

## Result

Pantheon runs SNE model-store removal recovery before exposing model
installation or lifecycle launch. As of 2026-08-20, production defaults resolve
the helper from the receipt-bound installed SNE package at
`~/Library/Application Support/Sirsi/SNE/bin/sne-model-store-recover`.
The earlier `~/.local/bin` side installation is historical and is no longer a
production dependency.

If recovery fails or the helper is unavailable:

- install availability is empty;
- installation is rejected with the recovery cause;
- lifecycle state is `failed`;
- runtime selections are unavailable; and
- model launch is rejected.

The same engine boundary is used by Pantheon install and lifecycle managers;
the UI does not reimplement recovery logic.

## Historical standalone helper identities

These hashes record the 2026-08-18 standalone integration evidence. They are
not current package identity. Each assembled SNE package builds and signs all
three helpers together, and its package-local `SHA256SUMS` is authoritative.

- `sne-model-checkout`:
  `06fcc1b9769003e87e74c47bd68092924e981b55fb276d49b6835a5dc10b970c`
- `sne-model-remove`:
  `4cec03aaea8d9411cdc09cef37c3aab2d21cbc53b1bc432aa9bc08bebfd44fe8`
- `sne-model-store-recover`:
  `283ee71f7c317cdba4cf17f393d5f98ab14db1f63f784f15d412d67ba4b051c2`

## Real store result

The installed recovery helper ran against
`~/Library/Application Support/Sirsi/SNE/model-store` and returned:

```json
{"result":{"removals_recovered":0,"objects_removed":0,"objects_retained":0},"type":"result"}
```

This proves the current store had no interrupted removal pending. No installed
model was removed.

## Verification

- Machine receipt: `docs/evidence/SNE_MODEL_STORE_RECOVERY_INTEGRATION_20260818.json`.
- Machine-receipt SHA-256:
  `cd42f805790848da39f6390e43994be41c9449257a27c0dd2cb8e404044f0d7c`.
- Focused SNE install/lifecycle tests: pass.
- Complete `go test ./internal/dashboard -count=1`: pass.
- Real default signed-catalog gate: pass.
- Current catalog: `sne-gemma4-v2`.
- Current catalog entries: `12`.
- Current catalog version:
  `aca14182c98fdab493491322d14d49ed84a6da861b718a03164bffe513101019`.
- Two signed versions are retained, so rollback is available.

The previous real-default test hard-coded the superseded v1 catalog identity,
11 entries, and one retained version. It now verifies the signed current
catalog's identity, entry count, version digest shape, retained-version count,
and rollback state without freezing stale release constants.

## Claim boundary

This proves production-default recovery invocation, clean-store success,
fail-closed behavior, and dashboard regression safety. It does not yet prove a
real process restart occurring midway through removal, a real installed-model
removal through Pantheon UI, upgrade/rollback interaction with shared objects,
or clean-host behavior on M1 and M5.

The 2026-08-20 source gate additionally proves that package assembly,
verification, Doctor, update, and rollback all require checkout, recovery, and
removal together. Signed clean-host lifecycle evidence remains open.

## Human-access linkage

- Canonical source: this document and its adjacent machine JSON receipt.
- Desktop mirror: `~/Desktop/Sirsi - Owner Reading Room/Pantheon/`.
- Sirsi Google Workspace mirror: pending; repository source remains authority.
## Contract-wide verification correction

The initial contract-wide run was executed inside a restricted shell that
denied loopback sockets, Unix sockets, hardware probes, and `launchctl`. Those
failures are classified as environment-rejected, not product failures. The same
contract was rerun with normal host permissions and passed:

- `go test ./...`
- `go vet ./...`
- `go test ./internal/dashboard ./internal/sne ./internal/snemodels -count=1`

Machine receipt:
`docs/evidence/SNE_MODEL_STORE_RECOVERY_VERIFICATION_20260818.json`.
