# Pantheon v0.23.14 Binary Footprint — 2026-08-28

## Scope

This is a local, unsigned, non-packaged binary measurement from the isolated
v0.23.14 source candidate. It neither creates a release artifact nor proves
signing, notarization, installation, resource residency, or crash behavior.

## Exact build

```text
go build -trimpath \
  -ldflags "-s -w -X github.com/SirsiMaster/sirsi-pantheon/internal/version.Version=0.23.14-beta" \
  -o /private/tmp/pantheon-v2314-sirsi-stripped ./cmd/sirsi
```

The command exited `0` (with the existing duplicate `-lobjc` linker warning).
`/private/tmp/pantheon-v2314-sirsi-stripped version --json` exited `0` and
reported `version: 0.23.14-beta`.

| Binary | Size | SHA-256 | Interpretation |
| --- | ---: | --- | --- |
| Unstripped stamped development build | 27 MiB | `ae1518ef757631ce1dd4bb9b9131871f0ebe4a425e0dd1d912e4eda3f60f2aa9` | Above the historical 25 MiB warning threshold; not a release-size measure. |
| Stripped stamped release-mode build | 18 MiB | `8ca5c75e96ef9aab1d24548d6bc0e1057c17362d3330b02f83b8f111b5c8d8fe` | Below the 25 MiB warning threshold. |

`otool -L` for the stripped executable listed only macOS system frameworks
(Foundation, Metal, CoreGraphics, CoreML, Security, CoreFoundation, System,
and related system libraries); it did not load Python.

## Ra default-path probe

The stamped temporary CLI ran:

```text
sirsi ra task pantheon "non-mutating proof"
```

Ra reported `Executor: native Go fleet` and `external Ra provider is not
configured`, then exited `1` as designed. This is a fail-closed behavioral
check: no provider, shell, Python process, task dispatch, or service was
started.

## Boundary

The stripped binary is a promising size measurement, not a signed asset. The
release remains gated on Developer ID signing, notarization/stapling, exact
published bytes, cask update, clean install/rollback, and sustained M1/M5
resource/crash qualification.
