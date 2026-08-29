# SNE Resource Caretaker

Classification: `platform-foundation`

## Commercialization gate

- Buyer/User: Mac users and operators running local AI through Pantheon or Nexus.
- Pain: model launch can consume unified memory, force swap, and make the human's machine unresponsive.
- Primary workflow: select an admitted model and start SNE without destabilizing the Mac.
- Willingness to pay: reliable local inference and machine stewardship support Pantheon trust and Nexus retention.
- Trust boundary: reads local capacity, kernel pressure, swap, and process-level resource state; it does not inspect user content or terminate unrelated applications.
- Operational owner: Pantheon owns lifecycle, admission, memory pressure, rollback, and evidence; SNE owns computation.
- Done evidence: deterministic unit gates, real launch gate, visible caretaker decision, failure recovery, and immutable SNE package identity.

## Contract

Every SNE start, supervised restart, and reload must resample the machine immediately before child launch. Admission requires:

1. A manifest-bound measured model footprint.
2. Readable RAM capacity and swap state.
3. No memory-death spiral.
4. No critical memory pressure; warning pressure also yields when the profile protects foreground work.
5. Existing swap below the greater of 2 GiB or one-sixteenth of physical RAM.
6. Available memory sufficient for the model footprint plus Pantheon's node-scaled OS and safety reserve. Existing agent memory is already reflected in live available RAM and is not subtracted a second time.

Pantheon reports the measured decision. It refuses unsafe launch rather than silently killing applications or tuning unsupported kernel controls.

## Real M5 gate

The formal SNE v2 package passed a real Pantheon-supervised launch on 2026-08-17:

- Package root: `~/Library/Application Support/Sirsi/SNE/packages/SNE-2.0.0-12b-affine8-mtp-shared-wide-v2-darwin-arm64`
- Manifest SHA-256: `bfa45db88d0988292b3b48ae8b6e7486a2e6eaf5e9ff2e9d86bc3996696865a5`
- Required bytes: `17179869184`
- Available bytes: `28941336576`
- Protected reserve: `10200547328`
- Swap used: `922747` bytes
- Swap limit: `3221225472` bytes
- Pressure: `normal` (`bootstrap-snapshot`)
- Exact response content SHA-256: `271973d4979a0bdfa021f64bf7cc98f77eeffd101b3d1fbcaaf178c4b061b62f`
- Engine throughput: `106.713 tok/s`
- Delivered throughput: `96.042 tok/s`
- End-to-end latency: `196.511 ms`

The supervisor stopped cleanly after the request. These throughput values are a single real integration proof, not the repeated-run release statistic. The immutable SNE release contract remains authoritative for performance claims.
