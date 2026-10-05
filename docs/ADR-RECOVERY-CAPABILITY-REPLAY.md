# Recovery capability replay identity

Status: proposed security repair; independent bind and release required.
Date: 2026-10-05

Candidate a991c42d hashes raw Authorization text after accepting non-strict
base64url. Alternate unused bits can represent the same Ed25519 signature and
bypass consumption. Require strict decoding and exact canonical re-encoding.
Consume the verified key-id/nonce pair atomically until its signed expiry,
not until cookie expiry. Cap cookie lifetime at capability expiry. A signer
must issue a unique nonce for every admission; differently serialized claims
with the same signer and nonce remain the same consumed authority.

This preserves the existing public-key-only gateway and closed destinations.
It introduces no host controls, credential persistence, network publication,
or vendor. Replay consumption remains local to one process; restart and
multi-process one-time admission require a separately reviewed durable
contract and remain release blockers. Regression controls cover alternate
signature bits, newline spelling, re-signed JSON, disconnect/session expiry,
and exact capability expiry. Actual HTTPS/WSS and device login stay separate.
