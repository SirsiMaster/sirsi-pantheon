# Enrolled signing client

`sirsi-sign ARTIFACT [codesign|bin|app|pkg|dmg]` sends an artifact to an
already-enrolled signing Mac and verifies the returned Sirsi Developer ID
signature. It never enrolls a signer, imports a certificate or copies a private
key. Commercial Pantheon builds use this client through
`PANTHEON_SIGNING_EXECUTION=remote-service` and `PANTHEON_SIGN_CLIENT`.

## Transport contract

Every SSH operation, both SCP directions, and rsync's SSH transport use the same
settings: `StrictHostKeyChecking=yes`, `UpdateHostKeys=no`, `BatchMode=yes`,
password and keyboard interactive authentication disabled, and a ten-second
connection timeout. The client obtains eligible targets exclusively from Ra's
admitted machine registry with `sirsi router machines --signing-targets
--format=ssh`. The registry supplies a canonical SSH alias and its existing
`HostKeyAlias`; `~/.ssh/sirsi_fleet.conf` supplies the current endpoint and
`~/.ssh/sirsi_known_hosts` supplies the exact ED25519 pins. A DHCP, mDNS or
Tailnet endpoint change therefore does not change identity or learn a key.

`sirsi-sign --check` performs only this registry and enrolled-daemon reachability
check. It creates no inbox job, reads no signing credential, and does not sign,
notarize, install, or modify an artifact. Unknown or changed host keys and
unavailable authentication fail rather than opening a dialog or waiting for a
person.

An enrolled WatchPaths daemon can be idle with exit status zero: `not running`
alone does not mean the signer is broken. The client preflight checks enrollment;
actual artifact signing/notarization still requires the enrolled service to
complete its work. No screen unlock or account-security change is performed by
this client. A failed reachability/authentication check is not evidence that a
certificate is missing.

## Verification and limits

Run `go test -race ./tools/signing` on a host with zsh. Fixtures execute the actual
client with isolated fake transport and signature-verification programs. They
exercise file/app publication, waiting, readback, cleanup, unknown-key and
key-mismatch refusal, a foreign signature, endpoint-change alias continuity,
and an `accept-new` negative control. They contact no real
host, read no credentials, and establish transport behavior only—not actual
signing, notarization, installation or multi-machine inference success.

The existing result-signature checks and remote job protocol are unchanged.
This patch does not claim comprehensive timeout, crash cleanup, arbitrary-host
authorization, notarization or signer failover qualification.

## Commercialization profile

- Classification: platform-foundation; not an independently launchable product.
- User: Sirsi release operators and the enrolled native build service.
- Pain: an unattended release must not create authentication/trust prompts or
  silently accept a new host identity in a later transfer phase.
- Workflow: submit to an existing pinned signer, retrieve and verify the result.
- Commercial contribution: dependable distribution and support; standalone
  willingness to pay is UNKNOWN and not claimed.
- Trust boundary: artifact bytes cross the existing SSH boundary; signing keys
  remain on enrolled signing hosts; host trust is never expanded automatically.
- Operational owner: SSA installed-package custody, coordinating the canonical
  signing service and Pantheon release source owner.
- Done evidence: source/race/negative fixtures, exact review and release
  integration, followed separately by a real signed-package/readback receipt.
