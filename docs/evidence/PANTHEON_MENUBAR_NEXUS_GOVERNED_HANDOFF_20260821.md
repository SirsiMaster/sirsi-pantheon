# Pantheon Menu Bar to Nexus Governed Handoff

**Date:** 2026-08-21  
**Status:** source and focused gate passed; installed live-browser proof remains open

The Pantheon menu bar previously hard-coded a divergent Nexus launch URL at the
site root. The dashboard and CLI already used the governed `/local-ai` route and
central fragment-only capability builder.

The menu bar now calls the same `BuildNexusCapabilityURL` contract. The focused
gate proves:

- HTTPS `sirsi.ai/local-ai` destination;
- no capability in the query string;
- exact capability in the URL fragment;
- rejection when the local capability is absent.

```text
go test ./cmd/sirsi-menubar -run TestMenubarNexusLaunchUsesGovernedLocalAIFragment -count=1
ok github.com/SirsiMaster/sirsi-pantheon/cmd/sirsi-menubar
```

This closes source drift across Pantheon launch surfaces. It does not prove an
installed menu-bar click, browser fragment capture, authenticated live SNE
stream, or release readiness; those remain gated behind an admitted practical-
context tuple.
