# SNE Package-Bound Support Privacy Verification

Date: 2026-08-21

## Product outcome

Pantheon no longer treats a successfully generated ZIP as sufficient evidence
that an SNE support bundle is safe to download. The exact installed SNE package
must provide both its exporter and its privacy verifier. Pantheon runs the
verifier against the newly exported archive before reading or returning bytes.

Successful downloads carry:

`X-Sirsi-Support-Privacy-Verified: true`

The operator interface reports **Exported privacy-verified SNE support bundle**.
If the package lacks its verifier, Pantheon fails closed with an actionable
repair/update message and returns no archive.

## Ownership boundary

- SNE owns archive composition, required evidence, checksums, and prohibited
  customer, network, credential, model, tokenizer, prompt, and output content.
- Pantheon owns consent, bounded execution, same-origin authorization,
  verification enforcement, download delivery, and actionable recovery.
- Pantheon does not recreate or weaken SNE's privacy policy.

## Source changes

- `internal/dashboard/sne_install.go`
- `internal/dashboard/sne_diagnostics.go`
- `internal/dashboard/sne_diagnostics_test.go`
- `internal/dashboard/pages.go`

## Verification

Focused package-bound support tests:

`go test ./internal/dashboard -run 'TestSNESupportBundle' -count=1`

Broader diagnostics and support tests:

`go test ./internal/dashboard -run 'TestSNE.*(Diagnostics|SupportBundle)' -count=1`

Both pass. The negative fixture provides a valid exporter while omitting only
the verifier, proving the verifier-specific fail-closed path.

The focused suite also renders the real served Pantheon dashboard and requires
the consent prompt, Enter/Space keyboard activation, privacy-verified success
wording, and visible failure message. This is a product-surface contract, not a
static source-string check against an unused template.

## Real package integration

An opt-in integration test invokes Pantheon's production Go wrapper against the
exact copied SNE support-privacy candidate rather than a mock. The wrapper ran
the package's exporter and verifier under its 15-second bound and returned a
valid ZIP.

`SNE_TEST_PACKAGE_ROOT=<exact-candidate-root> go test ./internal/dashboard -run 'TestSNESupportBundleRealPackagedIntegration' -count=1`

The command passed in 2.062 seconds. Ordinary test runs skip this external
package fixture unless the exact root is explicitly supplied.

## Combined M1 clean-host integration

Pantheon's compiled dashboard integration binary and the exact copied SNE
candidate were transferred to a separate M1 running macOS 26.6.2.

- SNE archive SHA-256: `5899e19f1fdfbb035cc42b53124160f38710572e9a38dd3c499463c3dea491fc`
- Pantheon test binary SHA-256: `177ac4e1c8a7d92773c8d90066d5c288e3449215bfa0a0d1cb5ecd7b06d540a2`
- Integration log SHA-256: `b7a4f6f194e9804665e42d0a2a6b131d440394c37a18fc88f8369d28be063383`
- Evidence manifest SHA-256: `64934d45aafdc1d5ccb027dde4cd9cba2ae10ac5c14b620288a32b81b1a13d4e`

After target-side hash verification, SNE installed under an isolated temporary
home. Pantheon's production Go wrapper invoked the installed package exporter
and verifier and returned a valid ZIP in 2.75 seconds. The package then
uninstalled with zero governed residue.

Result:

`m1_pantheon_sne_support_integration accepted=true exact_package=true exact_pantheon_test=true isolated_install=true production_go_wrapper=true packaged_exporter=true packaged_verifier=true valid_zip=true uninstall=true residue=false model_or_metal_run=false`

Evidence: `artifacts/evidence/sne-package-bound-support-m1-clean-host-20260821`.

## Claim boundary

This proves source-level and focused-test enforcement in Pantheon. It does not
claim that the currently installed signed SNE candidate contains the new
verifier. The next copied SNE package assembled from current source will contain
it; that package must still pass normal signing, notarization, clean-host, and
runtime qualification before GA.
