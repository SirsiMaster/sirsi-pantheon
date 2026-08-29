# SNE Process-Scoped Lifecycle Gate

Date: 2026-08-16  
Owner: Pantheon SNE supervisor  
Status: accepted candidate integration evidence

## Contract

Native MLX ownership is process-scoped. SNE advertises no in-process model lifecycle. `load`, `unload`, and `restart` are supervised operations owned by Pantheon:

- unload terminates the admitted SNE child while preserving the supervisor parent context;
- load starts a fresh admitted child and waits for readiness;
- reload replaces the running child with a fresh admitted child;
- all SNE lifecycle endpoints return HTTP 409 `restart_required` rather than attempting native close/reopen.

## Real gate

The opt-in real integration test launched the surviving candidate6 package:

`/Users/thekryptodragon/Development/sirsi-native-rebuild/artifacts/productization/packages-runtime-api-v0-v7/SNE-1.1.0-runtime-api-v0-candidate7-darwin-arm64`

It executed initial serving, supervised unload/load, and supervised reload against the pinned Gemma 4 12B affine-8 target and MTP assistant.

| Process stage | PID |
|---|---:|
| Initial load | `52111` |
| Load after supervised unload | `52118` |
| Supervised reload | `52125` |

All three processes returned official rendered content SHA-256:

`fda564ba3f7a0f028106d468420f674898ed99ac5bf2765ac9586206e39d73c5`

The test passed in `57.07 s`. Helper-process tests also prove lifecycle serialization and three distinct starts without requiring Metal.

## Claim boundary

This proves Pantheon-owned process lifecycle correctness for the candidate package. It is not a throughput, physical-bandwidth, cache, occupancy, signing, notarization, clean-Mac, or GA claim.


## Sirsi Google Workspace mirror

Native reading copy: [SNE Pantheon Process-Scoped Lifecycle Gate - 2026-08-16](https://docs.google.com/document/d/1CwEzyFXuIhr-Uhd-kiQR_kur9sZU9RzXVbEpY6LLjhM/edit?usp=drivesdk)  
Workspace document ID: `1CwEzyFXuIhr-Uhd-kiQR_kur9sZU9RzXVbEpY6LLjhM`  
Synchronization status: current as of 2026-08-16. Repository source remains authoritative.

## Candidate7 refresh

The real elevated gate was rerun against packaged candidate7 after the official thinking-disabled Gemma 4 prompt correction. Pantheon preserved process-scoped lifecycle semantics and exact final-answer content across all three processes.
