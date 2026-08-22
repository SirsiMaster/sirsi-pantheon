# Pantheon SNE Model Acquisition Provider

Date: 2026-08-17

Pantheon now has a separate, authenticated acquisition boundary for SNE model checkpoints. A strict source catalog binds a Pantheon/SNE catalog entry to one Hugging Face repository, revision, license identity, and exact file list. The provider downloads or resumes those exact files into a prepared-source directory. It never installs or launches a model directly.

The SNE `sne-model-checkout` transaction remains the promotion authority: it independently resolves readiness, enforces license acceptance, verifies the model manifest, and atomically installs the prepared checkpoint. This separation keeps network credentials out of SNE and prevents acquisition success from being mistaken for model admission.

## Proven provider controls

- HTTPS is required outside loopback tests.
- Redirects are bounded and restricted to the initial or recognized model-content hosts.
- Bearer credentials come from an environment variable rather than a process-list command argument.
- Downloads use identity encoding and HTTP byte ranges to resume partial files.
- Servers that ignore a range request trigger a safe restart from byte zero.
- Every completed source file must match its source-catalog size and SHA-256.
- Unsafe paths, duplicate files, malformed repositories, unknown providers, and trailing JSON fail closed.

## Deliberately incomplete

The provider implementation is ready, but the production source catalog is not. Several durable MXFP8/NVFP4 checkpoints do not retain authoritative upstream repository identity in their local configuration. Pantheon must not invent those records. Each source entry requires confirmation from the upstream model page or API before it is signed and distributed.

The next product steps are source-catalog population, source/admission cross-verification, governed receipt persistence, chaining acquisition to SNE checkout, cancellation and disk-full evidence, and native Swift progress/recovery UI.
