# SNE E2B API-v2 Pantheon M1 Admission

Date: 2026-08-21  
Classification: platform-foundation, candidate identity admission

## Commercialization gate

- Buyer/user: a Mac owner who expects Pantheon to select, launch, protect, and
  recover an exact local model without assembling runtime components manually.
- Pain: package, supervisor, profile, catalog, and memory-policy version skew can
  make a valid engine unlaunchable or silently bind the wrong runtime.
- Primary workflow: select the copied E2B successor, admit its exact signed
  identity, launch it through Pantheon, receive a local completion, tear it down,
  and restore the immutable installed rollback.
- Value: reduces installation/support risk and makes local inference governable.
- Trust boundary: loopback API, private mode-0600 capability, local model bytes,
  no cloud/CPU/Python fallback, no prompt or model content in support evidence.
- Operational owner: Pantheon owns catalog admission, resource care, lifecycle,
  readiness, recovery, and rollback; SNE owns inference.
- Done evidence for this slice: signed candidate catalog, exact package/runtime
  identities, M1 supervised launch, HTTP 200 exact response, and r5 restoration.

## Accepted result

Pantheon supervised the copied E2B NVFP4 API-v2 compatibility package on the M1
under the new catalog entry `e2b-nvfp4-api-v2-compat-v6-m1`. The secured local
endpoint returned HTTP 200 and visible content exactly equal to `M1-READY`.
After teardown, the immutable r5 LaunchAgent returned ready on port 8477.

This is identity-policy and bounded-functional evidence. Correctness-policy,
performance-policy, clean100, Nexus, release, signing/notarization of the full
successor tuple, and public performance claims remain open.

## Exact identities

- Pantheon supervisor: `76ef093aef732ecd601058a0879faecf900f59ded9ad1df6118a3bcf8d3a0de8`
- SNE service: `3c27e78431c7b7ba2535266377e97a6546d94f0afaf594b3478029921463084f`
- Native compatibility shim: `e22d6ae3f92c65a65a3a313946375b5bf4e8669c49f33604df6b1010aa4a8f4e`
- Service version: `1.2.2`
- Candidate admission registry: `12546e024e98c7d2c0b0f2fdad675fe516ad01c31929ab8b32b1ed3367a28d4e`
- Candidate readiness registry: `ea28f7284a3bfeb805f8f8d0ba4c899711f4e74740f9b4db50fbd800b022bcd2`
- Candidate runtime catalog: `bb212c70ea1a2b54cc8a2e31703238e60a36dcd702c29b851f1a6ec1d654c4d9`
- Detached catalog signature: `7f1dcfaa3fbb0861a764f0126482a4874fad2e0a16c1564f36848b0a9927702c`
- M1 profile: `8ee702b5d8e79b7124bb5002880e7a70ff9f266df161b0995f90fb3513bbb107`

The runtime catalog signature verifies against
`configs/sne/runtime-catalog-ed25519.pub`. The private key was never copied,
printed, or included in evidence.

## Defects found and repaired

1. The installed M1 supervisor predated readiness policy and dual-runtime
   identity flags. A copied current supervisor was used; installed bytes were
   not replaced.
2. The installed M1 profile lacked queue discipline, queue capacity, and request
   timeout fields required by the current serving contract. A copied current
   M1 profile was created.
3. An old readiness registry happened to have the same entry count but different
   IDs. Pantheon rejected it. Candidate readiness is now generated from the
   admission registry's exact catalog-entry set, with all unproven gates open.
4. Pantheon's nominally dynamic 8 GiB reserve floor consumed half of a 16 GiB
   node. The floor now remains 8 GiB on 32+ GiB nodes but is capped at 25% of a
   smaller node. Swap, pressure, measured availability, model requirement, and
   foreground-yield gates remain enforced.
5. Booting out the old installed supervisor left its 2.75 GB `sned` child
   resident. The isolated gate explicitly reaped only that known child before
   admission. Durable lifecycle packaging must ensure current supervisor and
   child advance together.
6. `LaunchConfig` always passed `--version`, but the supervisor CLI never
   populated `ServiceVersion`. The CLI now requires explicit `-service-version`
   for a launch and binds it into the child configuration.

## Evidence

Directory: `docs/evidence/sne-e2b-api-v2-compat-v6-pantheon-m1-20260821`

- Receipt: `0e7883dbdb14604541b861d91b586c9b91d6a00bbe57ab5f7bd5256739ec2a1c`
- Response: `8ff31c48adc94b6e9224ad36946e366f6efde73b39b2207b0bd69e23cac8c901`
- Request: `8cc8b59e05e97a0535458887824dcbb831dd0ccc8e359442d741daee45742876`
- Readiness: `141f71a56a0c80556d999083877e4dc9c18cb55ca3777c16999a3c989791b2dd`
- Supervisor log: `703a124327fc032ba08acaaebafc0f6d3fc3818eb8f4b4f401766e51e389fccc`

Focused `internal/guard` and `internal/sne` Go suites pass. No performance result
is admitted because the M1 session was not a clean AC performance campaign.

