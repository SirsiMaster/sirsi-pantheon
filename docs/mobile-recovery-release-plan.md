# Mobile recovery release integration

/plan Pin release68b1ee17 confirmed through GitHub connector; transplant only desktoprecovery, serve command, tests, client assets and user documentation from565b43cf; retain current dependencies; independently review exact candidate.
/goal Tested minimal release-source candidate. Product goal remains open until approved issuer/private ingress, M1 phone Apple login/view-only/control/disconnect and independently enrolled M5 anchor proof.
Estimated duration: 45 minutes. Next check: 2026-10-05T12:30:00Z.

Classification: platform-foundation (development candidate).
Buyer/user: authorized Mac operator recovering active desktop from iPhone.
Pain: loss of local desktop access while keeping Mac protections intact.
Workflow: approved private origin, signed one-use admission, Apple authentication, view-only, explicit control, disconnect.
Value: operational continuity; willingness to pay remains unmeasured.
Trust boundary: private desktop pixels and input; no credentials persisted; operator security decision required before issuer/enrollment/ingress.
Owner: Pantheon implementation; SHA host qualification; Ra issuer authority.
Done evidence: exact source independent review, race/vet/build, private-authority decision, phone and alternate-anchor receipts. No release claim from tests alone.

The candidate adds only desktop recovery prerequisites. Old apprecovery registration/remediation commands and SNE compute supervisor history are unrelated and excluded. ADR064 already belongs to Isis on release main: proposed authority renumbered075, historical064 references retained in the handoff. No production authority created.
