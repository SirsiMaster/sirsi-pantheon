// Package verify is the VERIFICATION HALF of ADR-078 (Ownerless Recovery —
// Signed LAN Anchor): a pure Go library that decides whether a signed
// recovery request would be admitted, and why not when it would not.
//
// # What this package is
//
//   - A strict parser for the ADR's eight-field canonical envelope.
//   - An OpenSSH SSHSIG verifier (namespace "sirsi-recovery-anchor";
//     ssh-ed25519 and ecdsa-sha2-nistp256 only; sha256/sha512 digests),
//     implemented in Go on golang.org/x/crypto/ssh. It never shells out to
//     ssh-keygen.
//   - A parser for the grant source ("anchor-grants.allow" semantics) and for
//     authorized_keys narrowing, both FROM BYTES supplied by the caller.
//   - The epoch model, the wall-clock ratchet, the admission-order high-water
//     mark and the atomic replay admission, all against an injected Clock and
//     an injected Store interface (a test-only in-memory Store is included).
//   - A pure pre-dispatch revalidation function (generation fence, dirty
//     policy, epoch, ratchet, expiry, grant revocation).
//
// Every decision is a typed Decision carrying a stable Reason code. Every
// error path fails closed (deny).
//
// # What this package is not
//
// It is not a daemon, runs nothing as root, opens no socket or listener,
// reads no file from any fixed path (callers pass bytes), generates or
// enrolls no key (it parses PUBLIC keys only), writes no anchor-grants.allow
// or authorized_keys, and NEVER executes a recovery verb. The four verbs
// (restart-broker, remount-volume, re-arm-supervisor,
// fetch-known-good-config) exist here only as constants that a grant may name
// and an envelope may request; admission is a decision, not an action. There
// is no durable store here: the Store interface documents the atomicity a
// real one must provide.
//
// # Authority
//
// ADR-078 (docs/ADR-078-OWNERLESS-RECOVERY-SIGNED-LAN-ANCHOR.md) is the
// contract; its 2026-11-11 status entry records the owner decision that
// authorizes this library with fixtures and nothing else. The daemon, verb
// execution, key enrollment, anchor-grants.allow and authorized_keys changes
// each need a separate owner decision with working code in hand. Ledger:
// rs-42-recovery-anchor-implementation.
package verify
