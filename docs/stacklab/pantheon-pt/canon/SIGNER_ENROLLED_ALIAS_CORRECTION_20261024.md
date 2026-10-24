# Enrolled signing aliases — corrective release input

Classification: platform-foundation. Owner: SSA, coordinating Mercury, Ra and
Pantheon Main. Governing mechanism: ADR-074 and tools/signing/README.md; no new
signing architecture or credential flow.

Mercury160821 reports M5 DHCP .155 no longer identifies its signing host.
The installed signing client still hard-codes that address. Existing local SSH
configuration resolves sirsi-m5 to user thekryptodragon and sirsi-m1 to user
sirsimasterdev with StrictHostKeyChecking true and matching pinned host aliases.
This configuration observation does not independently prove remote reachability.

Plan: replace only the default ordered endpoints with those enrolled aliases;
retain non-empty explicit override behavior and strict noninteractive options.
Qualify unset/empty/default/override order against fake denied transport and
restore the old DHCP line as a causal negative. Retain the existing all-phase
file/app, signature-refusal and accept-new negative fixtures. Fixture HOME,
ZDOTDIR and zsh startup are isolated; no real signer/network/credentials used.
Run focused normal/race/vet and normal protected source publication. Independent
review and exact135 release-preparation composition follow; no edit to Main's
running frozen qualification checkout or silently refreshed provenance pins.

Done means this source correction is normally published and included in a
qualified release; source tests alone are not live signing or host adoption.
Actual signing must later use the existing enrolled service and read back the
same signed artifact. No key/trust, Tailscale, account, firewall, login, security
or service changes belong to this repair. Rollback is the preceding client or
an explicit existing pinned SIRSI_SIGN_HOSTS route, not a new credential.

Repo source is authority after origin publication. Stack Lab recipe/catalogue
registration is required with final source identity. Desktop and native Workspace
reading copies remain pending until release integration; no three-home claim.
