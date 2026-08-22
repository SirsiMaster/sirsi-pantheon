# SNE Privacy-Safe Support Bundle Integration

**Date:** 2026-08-20  
**Status:** source and focused Go contract passed; clean-host package evidence pending

## Responsibility boundary

SNE owns support-bundle composition and privacy policy. Pantheon does not recreate or enrich that archive. It invokes only the exporter installed inside the selected SNE package, applies a 15-second deadline and 4 MiB archive ceiling, streams the resulting ZIP to the browser, and removes the private temporary workspace after reading it.

## User experience

Pantheon's SNE view now retains the lightweight JSON diagnostics export and adds a separate keyboard-accessible **Export complete SNE support bundle** action. Before execution, the user sees the exact included and excluded data classes and must confirm. The browser receives a no-store, `nosniff`, `application/zip` download. The interface tells the user to review it before sharing.

## Security and failure behavior

- POST only.
- Same-origin only; hostile origins fail before helper execution.
- Installed package helper must be a regular executable.
- Helper failures return generic actionable errors without command output or paths.
- Archive must be a non-empty regular file no larger than 4 MiB.
- Temporary output is removed after the response bytes are read.
- Pantheon does not collect logs, prompts, model content, or additional machine state.

## Evidence

`go test ./internal/dashboard` passed, including exact helper identity, hostile-origin rejection, ZIP content type and disposition, and byte-preserving download behavior.

No SNE service, model, inference, or GPU workload ran. Clean-host UI execution against the signed/notarized package remains required.
