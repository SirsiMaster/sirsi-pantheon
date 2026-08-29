# SNE Native Runtime Identity Correction

**Recorded:** 2026-08-21T15:31:21Z  
**Scope:** Pantheon supervised SNE launch identity  
**Claim boundary:** Integration correctness repair; no model, Metal, or performance run occurred.

**Owner-readable mirrors:** Desktop Markdown/HTML and [native Google Doc](https://docs.google.com/document/d/1b5aulkcj3dn0Tz03FVsBzitPGB2Or-TTBaWUPaQZYJA)

## Defect

Pantheon's signed runtime catalog intentionally stores two separate hashes:

- `runtime_sha256`: the `bin/sned` service executable;
- `native_runtime_sha256`: `lib/runtime/libsirsi_native_runtime.dylib`.

Pantheon verified both files against the correct catalog fields, but `resolveLaunch` then assigned `runtime_sha256` to `LaunchConfig.RuntimeSHA256`. The supervisor passed that value to `sned --runtime-sha256` and required readiness to report it. This confused service identity with native-runtime identity and repeated the same defect already repaired in the SNE qualification launchers.

## Correction

`resolveLaunch` now assigns `LaunchConfig.RuntimeSHA256` through `nativeRuntimeIdentityForLaunch`, which returns only `RuntimePackage.NativeRuntimeSHA256`. The service executable remains independently verified against `RuntimePackage.RuntimeSHA256` before launch.

The `LaunchConfig.RuntimeSHA256` declaration now documents that it is the native runtime dylib identity reported by `sned` readiness and must never contain the service executable identity.

## Prevention and evidence

`TestNativeRuntimeIdentityForLaunchNeverUsesServiceHash` constructs intentionally different service and native-runtime identities and proves that launch selects only the native identity.

Focused tests pass:

```text
ok github.com/SirsiMaster/sirsi-pantheon/internal/dashboard
ok github.com/SirsiMaster/sirsi-pantheon/internal/sne
```

Artifact identities:

| Artifact | SHA-256 |
|---|---|
| lifecycle resolution | `9d9d6a13b60f7fea15a3635e9e0ba2bfef2dc0c9c1aa6f5c4db3da0c90ab9564` |
| regression test | `3d29610ca8080504822be30fe8de226356b2e06a5b29cee3a2df7f64d6a23ad2` |
| supervisor identity contract | `44ef432c1974e8e0dbd8f11d8ae3426c3469f9540988484a934bbf6958bafbd6` |

## Catalog boundary

The current signed Pantheon catalog still points to the older pre-API4096 candidate identity. The API4096 research package has a distinct model ID, manifest hash, and `qualification.status=research`, so Pantheon correctly rejects it before admission. A successful API4096 receipt must be followed by copied product-package promotion and a newly signed Pantheon catalog/admission tuple; the existing signed catalog must not be edited prematurely.
