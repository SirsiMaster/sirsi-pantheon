# Pantheon Ra Go-Native Cutover — 2026-08-28

## Source boundary

This record covers the local integration of the reviewed immutable Ra source
lineage into the v0.23.14 candidate:

| Source commit | Purpose |
| --- | --- |
| `194ed0709523734997ef2b91c226734c4fb5e281` | Replaces the default CLI and recording pipeline Python executor with Go-owned fleet execution. |
| `3bfd49adfc5a43e708df2f2859ac743c0ddc1641` | Retains task and broadcast only behind an explicit developer-provider boundary. |
| `abc2663b01da7644184bb6114e4e928e294af5a1` | Rejects relative, symlink, non-regular, and non-executable provider paths. |

The integration commits on the local candidate are `5c837dbe`, `0c0df591`,
and `0b8c00b2`. They change only `cmd/sirsi/ra.go` and `internal/ra` source
and tests. They do not start a provider, create a Python environment, invoke a
shell, or change a service, Tailscale, security, SNE, or host state.

## Product behavior

- `sirsi ra health`, `test`, `lint`, and `nightly` use the Go-owned,
  direct-executable fleet runner.
- `sirsi ra task` and `broadcast` fail closed unless
  `SIRSI_RA_PROVIDER_EXECUTABLE` names an absolute, regular, non-symlink,
  executable file.
- An opted-in provider is invoked directly, never through a shell, Python, or
  a provider SDK. No configured provider is a truthful unavailable state.
- The recording pipeline serializes the Go-native results through its existing
  Seshat/Thoth recording path.

## Verification

| Command | Exit | Scope |
| --- | ---: | --- |
| `go test ./internal/ra` | 0 | Native fleet semantics, explicit provider, failure preservation, no Python/shell fleet steps, and injected recording path. |
| `go test ./cmd/sirsi` | 0 | CLI integration; completed in 38.985s. |
| `go test ./...` | 0 | Full Go source suite, including MCP, dashboard, SNE, TUI, and E2E. Only duplicate `-lobjc` linker warnings were emitted. |
| `git diff --check` | 0 | Source diff integrity before evidence commit. |

## Boundaries still open

This is source/test closure only. It does not prove a signed/notarized package,
GitHub release asset, Homebrew cask binding, clean installation/rollback, or
sustained M1/M5 resource and crash qualification. It is not a release claim.
