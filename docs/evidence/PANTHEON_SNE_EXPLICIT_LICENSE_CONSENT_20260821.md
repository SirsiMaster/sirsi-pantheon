# Pantheon SNE Explicit License Consent

**Date:** 2026-08-21  
**Classification:** platform-foundation product UX  
**Status:** source and focused contract gates passed; clean-host assistive-technology proof remains open

## User outcome

Pantheon no longer represents model-license acceptance with a generic browser
confirmation prompt. Selecting an installable signed model opens a dedicated
terms dialog that:

- identifies the exact model;
- explains that Pantheon downloads and verifies the signed tuple;
- links to the governed license URL in a separate window;
- starts with consent unchecked and installation disabled;
- requires explicit acceptance before enabling `Accept and install`;
- offers a separate `Cancel` action;
- records acceptance through the existing transactional backend contract.

Unknown or absent license identity/URL still fails closed. Research tuples are
not enabled by this change. The API request remains
`accept_license:true, allow_research:false`.

## Verification

```text
go test ./internal/dashboard -run 'TestSNEDirectRouteLoadsSNEView|TestSNELicenseTermsRegistryFailsClosed|TestSNEInstallAPI' -count=1
ok github.com/SirsiMaster/sirsi-pantheon/internal/dashboard
```

## Claim boundary

This proves the source-level interaction and backend contract. It does not yet
prove VoiceOver behavior, visual quality on a clean installed build, a real
network model acquisition initiated through the dialog, notarization, or
complete Pantheon-to-Nexus use. Those remain launch gates.

Google Workspace synchronization is not available in this agent session; the
canonical repository and Owner Reading Room copies are present.
