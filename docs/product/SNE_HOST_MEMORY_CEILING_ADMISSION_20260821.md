# SNE Host Memory Ceiling Admission

Date: 2026-08-21

## Incident

The M1 pilot carried a copied Pantheon SNE supervisor profile with a 40 GiB process memory ceiling even though the host has 16 GiB of physical RAM. The selected E2B NVFP4 model itself declared a measured 6 GiB requirement, so model admission could pass while the stale process ceiling remained physically impossible and operationally misleading.

The remote repair receipt is stored on the M1 at:

`/Users/sirsimasterdev/Library/Application Support/Sirsi/Pantheon/recovery-receipts/20260821T060235Z-m1-sne-policy-repair`

That reversible repair disabled the stale duplicate `ai.sirsi.pantheon-sne-e2b` launchd label, changed the active profile ceiling from 40 GiB to 12 GiB, retained the cataloged 6 GiB model requirement, and restarted only the governed active supervisor.

## Permanent prevention

Pantheon now validates a configured SNE process memory ceiling against measured host physical RAM at the actual supervisor launch boundary.

- `0` remains a supported optional/no-explicit-ceiling policy.
- A ceiling at or below physical RAM remains structurally valid.
- A ceiling above physical RAM fails closed before SNE starts.
- Live model admission remains separate and still considers measured model footprint, available RAM, dynamic reserve, lifecycle reserve, pressure, and swap.
- Pantheon does not silently clamp an invalid profile because that would hide configuration drift.

Stable diagnostic:

`memory_ceiling_exceeds_host_capacity`

Recovery guidance:

`Select a device-qualified SNE profile whose memory ceiling fits this Mac, then retry.`

## Qualification

Focused command:

```text
go test ./internal/sne -run 'Test(ResourceAdmission|HostMemoryCeiling)' -count=1
```

Result: PASS on 2026-08-21.

The focused tests prove rejection of the observed 40 GiB-on-16 GiB failure and acceptance of optional, 12 GiB, and exact-host 16 GiB policies. This is a product safety result, not a model performance claim.
