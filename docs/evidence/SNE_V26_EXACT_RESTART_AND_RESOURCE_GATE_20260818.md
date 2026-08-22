# SNE v26 Exact Restart and Resource Gate

Date: 2026-08-18

## Result

Pantheon admitted the copied SNE v26 package, forced a full supervised process
replacement, and admitted the replacement only after the complete package,
identity, capacity, and resource contracts passed again.

- first PID: `94803`
- replacement PID: `94885`
- artifact set SHA-256: `c6c315c9d26e7b353b65c9544c611f19eafb59b9a0f917aaed464b6582571a87`
- verified artifacts: `8`
- verified bytes: `12,935,160,119`
- runtime SHA-256: `fc979e2031fc2459b6ecca96d3d6d85cafa8d898d7e3bcda90bda1e98f2c0787`
- manifest SHA-256: `81e9332f7b516b99ddb1c4a23896733fee67a487e70bfded10091fe526463904`
- cache topology: `paged-ring-4096`
- serving cache capacity: `4096`
- prefix-session maximum: `2`
- required bytes: `17,179,869,184`
- available bytes: `32,381,779,968`
- reserve bytes: `10,200,547,328`
- swap used bytes: `518,122,373`
- swap limit bytes: `3,221,225,472`
- pressure: `normal`
- pressure source: `bootstrap-snapshot`

## Executable gate

`internal/sne/real_lifecycle_test.go` runs the gate when
`SNE_REAL_EXACT_RESTART=1`. It requires a distinct replacement PID and exact
post-restart readiness identity. `VerifyRuntimePackageBoundary` independently
checks governed hashes, package-contained symlinks, Mach-O imports, and rpaths.

## Claim boundary

This proves lifecycle and package durability for a candidate. It does not
promote v26, establish generation performance, or authorize a public speed
claim.
