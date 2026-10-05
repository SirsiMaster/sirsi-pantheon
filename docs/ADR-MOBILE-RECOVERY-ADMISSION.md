# ADR: Independent mobile recovery admission

Status: Proposed; requires independent security bind before release.
Date: 2026-10-05
Governing canon: SIRSI_MANIFESTO.md, SAFETY_DESIGN.md.

SHA independently demonstrated a loopback caller could forge a tailnet identity header at 58c6dfc3. Loopback address proves locality, not Serve provenance. Admission now requires a separate operator password checked against a configured bcrypt hash (cost 10–14) in addition to the allowlisted identity. This uses the existing Go crypto dependency; no external vendor or credential broker is introduced. Operator provisioning is explicit; no default password or plaintext config secret exists.

HTTPS/WSS and private Serve are mandatory deployment boundaries. POST exact-origin checks and single-use expiring cookies remain. The entry page handles fresh/expired admission. Mac RFB credentials remain transient in noVNC memory; documentation must not claim they never reach the browser. No permissions, services or network exposure are changed by this source repair.

Verification: forged local header and wrong password reject; valid operator credential admits; fresh and expired browser entry renders. Live Mac auth, Safari, credential trust and ingress need independent qualification.
