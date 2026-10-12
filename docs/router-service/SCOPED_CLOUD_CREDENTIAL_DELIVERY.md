# Scoped cloud credential delivery

Cloud mint jobs must not print bearer tokens into Cloud Logging. Use `router token mint <dedicated-cloud-host> --agents <one-canonical-agent> --secret-resource projects/<project-number>/secrets/<dedicated-secret>`.

The native publisher uses the existing Cloud Run service identity through the Google metadata server and posts directly to the fixed HTTPS Secret Manager API. Redirects are refused. Only a nonempty lane scope can use this export path. Output contains token ID, logical host, scope and immutable secret version, never the token or metadata credential. Failed publication or receipt writing revokes the freshly minted token; failed revocation reports explicit cleanup debt with the token ID.

Grant the existing mint job identity `roles/secretmanager.secretVersionAdder` on each exact dedicated destination secret, not project-wide. Keep its database credential server-side. Cloud workers receive only their own lane credential through a supported secret proxy for the exact canonical router domain; never an unscoped Mac token, bootstrap token or database credential. Separate SSA/Ma’at and Ra credentials retain the canonical agents `sirsi-software-admin` and `ra`.

The legacy plaintext output remains for explicit existing local operator flows; do not use it in cloud jobs. This correction does not retroactively erase historical log exposure, claim leaked credentials revoked, grant owner-account scope, or complete cloud runtime qualification.

Focused tests exercise success, metadata failure, publication failure, wrong returned version identity, redirect refusal, receipt writer failure and revocation debt. Tokens and provider identity sentinels must not appear in output/errors.
