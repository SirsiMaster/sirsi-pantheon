# Managed command display guard

This source-only adapter reads one bounded hook envelope and checks string
`command` and `cmd` fields regardless of tool name. It refuses recognized direct
display-power operations and never executes the supplied payload. Input is
limited to 8 MiB by a local resource policy; this is not a claim about the
hook emitter's maximum. Complete JSON framing is required, and malformed,
trailing, read-failed, or over-limit envelopes are refused. Valid non-command
payloads within the bound pass through this narrow command check.

This assumes the emitter supplies command text as string `command` or `cmd`
members of an object-valued `tool_input`. Other shapes are not inspected as
commands, and JSON duplicate keys follow Go's last-value behavior. Inputs over
8 MiB are refused regardless of tool type; larger payloads are not supported by
this policy. Shell control flow, variable expansion, and other forms outside
the recognizer's direct-command coverage can evade this narrow check.

The adapter buffers the bounded envelope and JSON decoding may make additional
bounded copies while checking `command` and `cmd`. The cap therefore bounds the
input and its parsing work, while peak memory can be several times the envelope
size. Both command fields are inspected regardless of `tool_name`; large MCP
payloads do not receive a tool-name exemption.

The matching recognizer handles selected direct commands, common execution
wrappers, and command substitutions with a shared recursion bound. It is not a
general shell parser or sandbox. Denial is independent of hook metadata and
cannot be bypassed through `sirsi agent safe-run --force` or ignored health
checks. Read-only commands remain outside the deny set.

This source correction does not install or activate a hook, modify system
settings, or add authority. `scripts/stacklab/managed-requirements.toml` is a
source-only matcher fixture; it does not prove an installed route invokes this
adapter. Coordinate-only GUI actions and unmanaged command surfaces remain
outside its coverage. ADR-119's typed sleep transaction remains the separate
authority for governed sleep; this guard does not create an issuer, grant,
custody store, or native actuator.

## Source and fixture checks

The adapter fixtures cover bounded input, malformed envelopes, both command
fields, tool-name independence, and refusal before a fake dispatch boundary.
Parser fixtures cover direct and wrapped invocations, argument positions,
empty quoted arguments, quoted substitution syntax, and depth exhaustion. The
`SafeRun` fixture uses only an owned harmless executable and verifies that
force and ignored-check options do not bypass a command-safety denial. None of
these fixtures executes a native display or power command.

This source-level correction does not qualify an installed hook, a full shell
containment boundary, GUI-action policy, or the combined display lifecycle.

## Standalone artifact staging (source candidate)

`bash scripts/build-pretoolguard-artifact.sh <full-commit> <output-directory>`
builds unsigned Darwin arm64 guard and installer executables in a new retained
directory. The installer uses cgo for the libSystem ACL bridge; a no-cgo build
does not qualify as an operational installer. It
requires native Darwin arm64, an exact checkout with no tracked or nonignored
untracked changes, and an existing absolute output directory. Ignored files and
the Go toolchain/environment are not attested by this check; use an independently
qualified build environment. Run it only under the host's applicable admission and safe
window. The output hash identifies the unsigned bytes; signing changes those
bytes. This staging helper neither signs nor installs the result.

The enrolled `tools/signing/sirsi-sign` supports `bin`, but this is not a
declaration that its current queue, cleanup, identity, or notarization custody
is qualified for this guard. Before using it, the signing custodian must resolve
those conditions and retain the returned receipt and signed-byte hash. Verify
the Developer ID signature and applicable notarization independently. A signed
artifact alone is not authorization to change any installed hook.

The source candidate `scripts/sign-pretoolguard-artifact.sh` connects a staged
binary to the repository client and M1 route, using the expected unsigned digest
and independently selected code identifier. It refuses pre-existing output,
verifies the returned signature and compares retained copies after signature
removal. This normalization-based equality check is untested and needs review;
it is not yet a qualified provenance receipt.
The helper accepts either the guard or installer filename, each signed and
checked separately. Distribution and release binding remain to be qualified.
The existing client removes a successfully consumed remote outbox job; this
wrapper does not change that behavior. Do not invoke it until the custodian has
established acceptable receipt and queue retention. It does not notarize or
publish the executable.

The separate source candidate `scripts/package-pretoolguard-artifact.sh` builds
a two-executable disk image and uses the same explicitly selected signer; it checks
the returned image's team signature and stapled ticket, and retains hashes and
the signer receipt. It does not publish or install. The candidate now mounts the
returned image read-only to check both payload hashes and signatures; that new
code has not been tested or independently reviewed. The existing two-asset
publisher intentionally remains unchanged.

The three observed M5 targets have different custody, including a root-owned
target. No live replacement or rollback is provided by this staging helper.
Installation remains on hold until a reviewed per-target installer establishes
exact target identity, retained metadata-preserving backup, safe atomic
replacement and tested rollback, and the required host-specific admission and
measurement-safe window. Do not use a blanket copy or infer installed behavior
from this build.

### Custody and source-candidate limits

Each fresh build, signing, or package directory retains an `INCOMPLETE` marker
until its local checks finish; only then is it renamed to `ACCEPTED`. Treat a
missing marker, `INCOMPLETE`, a failed or interrupted invocation, or uncertain
output as unaccepted and preserve the directory for investigation. The marker
name is authoritative; its text may have been updated just before the rename.
`ACCEPTED` means only that this candidate's local checks completed, not that an
artifact is independently qualified, published, or installed. An interrupted
signer call may have created remote work or consumed its outbox even when no
local receipt or accepted marker exists; reconcile with the custodian before
any retry. No marker provides crash durability or protection against a writer
with access to the staging directory.

The exact client path and `sirsi-m1` string are configuration checks, not a
client-integrity, SSH host-key, route, physical-host, or signer-custody
attestation. A changed client, alias configuration, environment, intermediate
path, or concurrent writer may alter behavior. Independently qualify and pin
the client, transport, host identity, signer, receipt and staging custody before
use. The scripts do not prove those properties or eliminate time-of-check/time-
of-use races. Signature inspection requires a reported secure timestamp on
the binary and image and a reported hardened-runtime flag on the binary; this
is based on `codesign` textual output and is not an independent timestamp or
policy attestation. The checks and output format require macOS qualification.

The returned image is checked for the two expected regular, single-link payloads
with mode 0755, their expected digests, and signatures. Both are required to have
no declared entitlements. Its mount is detached on
the normal path and attempted on failure after a successful attach. A failed
detach leaves the package unaccepted and requires manual inspection; an
interruption during attach, forced termination, or a crash can leave a mount
without cleanup. Inspect the retained mount path and system mount state before
recovery; do not blindly detach an unrelated mount. No force-detach is used.

Nine source-contract and mocked entitlement-helper fixtures passed on M5 under
the agent's own released CPU admission
`20261103T025418Z-m5-sirsi-software-admin-d18738d0735780e68b16a944b18e7ac8`,
terminal `247e1a6e0e01492eb2f23c51c0445993`. They neither execute `codesign`
nor build, sign, notarize, mount, publish, or inspect actual artifacts. The
entitlement helper accepts empty successful `codesign` output as no declared
entitlements; that interpretation needs native qualification. Its output size
check occurs after subprocess output capture, not during capture.
