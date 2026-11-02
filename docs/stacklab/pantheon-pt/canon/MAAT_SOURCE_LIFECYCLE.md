# Source lifecycle observation and pure replay

Additive continuation of the separately qualified f514 source-import slice.
This document defines the lifecycle source contract and its compatibility
limits; it does not claim full ADR-070 completion, empirical calibration,
producer authentication, installed acceptance, or gate activation. The current
source composition still requires its admitted finite qualification.

## Contract and available workflow

`maat.source-lifecycle-receipt.v1` has closed policy/evaluation/outcome kinds.
All serialized fields in `LifecycleReceipt` are required, with empty arrays and
strings for inapplicable fields. The Go strict decoder enforces exact keys,
duplicates, required zero values, types, depth and 256KiB. Profile identity binds
producer, model artifact/version, prompt, reusable evidence recipe and policy
SHA256. It does not change for each source head; every sample/outcome still binds
its own exact repository/head and source-screen digest.

Policy numbers are explicitly declared operands, not empirically established
thresholds. Positive minima do not select a scientifically sufficient sample
size. Result-set SHA hashes the canonical ordered sample array, including
unqualified v2 source packets and outcome references. Dataset SHA hashes the
ordered `id:repo:head` inventory. Selected IDs preserve the sampling decision and
denominators; absent labels cannot qualify. Shadow PASS/CHANGES classification
never emits a qualified source verdict or changes public v2's R2 escalation.

The existing CLI now offers `maat lifecycle --receipt-id ID --expect-repo REPO
--expect-head FULLSHA --preview --json`. It reads canonical sender-authored
Instructions through `Resolve()->Store.Get` and shows custody, unknown acceptance,
profile/epoch and declared/verified/missing counts. Correct the selected ID/repo/
head if unavailable, nonfinal, legacy prose or malformed. Result text is never
borrowed as the original sender's review. Unknown receipts do not become success.

After review, `--confirm --expect-receipt-sha256 SHA` retains this exact
observation, not a grant. The native System One control uses the same command and
bounded runner: editable scope -> explicit preview -> frozen exact scope/profile/
digest confirmation -> one record -> retained operation receipt. It never runs
automatically on load, refresh or remount. Failed preview/readback retains the
original operation receipt and requires a new explicit preview. Legacy comparison
metrics are named accordingly, not “Calibrated”. The lifecycle control appears
beside source import in both the normal System One view and its history-error
view; the Decisions view's separate control is not a substitute. Casebook's
shared optional event and reason propagate to existing CLI/MCP/TUI/dashboard
projections.

## Atomic history and failure bounds

The displayed epoch is the deterministic unique-event revision for this profile,
not a claim that a qualification epoch was accepted. When provenance is unresolved,
sample coverage is explicitly unknown, not a successful zero-missing-outcome count.

The existing append-only decision journal is reused, without a new DB or daemon.
Confirm holds ONE `withJournalMutationLock` across complete bounded read,
canonical resolution, replay, duplicate/conflict check, normalization and
`appendLocked`; it never calls generic Append recursively under the lock.
The nonblocking existing lock reports contention; clients do not spin/retry.
Replay uses file order, not timestamps or `Recent(50/1000)`. It retains the full
relevant history even beyond a display window. Bounds:64MiB whole regular owned
single-link no-follow/nonblocking input,100000 lines,256KiB/line,512 resolved
receipts per replay. A bound, malformed row, changed file or missing reference
fails closed/unqualified, never truncates into qualification. Windows refuses
this lifecycle reader; existing host workflows are unaffected.

Receipt IDs cannot silently change bytes/head/profile. Duplicate identical events
do not append or inflate samples; conflicts refuse. Pure accepted-fixture replay
models PASS/CHANGES coverage, missing labels, exact policy binding and sticky
auto-pass overturns. Overturn sets requalification-required and records that the
next threshold must exceed the invalidated policy's threshold. It invents no
replacement numerical threshold or newly validated statistical policy.
A changed model/prompt/recipe/policy is a new unqualified profile. A threshold
change alone does not erase retained invalidation.

## Installed acceptance verifier and owner-adoption seam

`lifecycleevidence.Router` is a real canonical Get reader of exact Instructions
bytes. On macOS, the production CLI uses the installed acceptance verifier only
after the current executable resolves to the canonical packaged engine and
passes the existing Developer-ID signature requirement. Other environments
retain custody-only resolution. The embedded, closed inventory is
`internal/maat/qualificationacceptance-v1.json`; it currently contains no
accepted policy, actual evaluation, or independent-outcome pins. Thus no model
profile is empirically qualified. Inert Go/Swift fixtures remain software tests
and are never inserted or counted as real calibration evidence.

The installed-engine identity and no-follow validation live in the leaf
`internal/packagedengine` package shared by setup and Ma'at. It retains the two
exact shipped paths and the existing Team-ID/identifier signature check;
Ma'at alone mints the opaque lifecycle authority after both checks succeed.

The CLI additionally supports an explicit owner-adoption preview/confirm for an
exact canonical independent outcome receipt and its originating evaluation
receipt. It binds both raw Instructions SHA256 values, repo/head/profile,
screen, candidate/final outcome, producer and the canonical reviewer
attribution. The preview reports that `From` is not authenticated identity.
Confirm re-resolves both receipts and appends the bounded exact raw receipt
bytes plus the immutable envelope using the existing journal lock. The combined
raw receipt bound is 96KiB; larger operands refuse before append. Replay can use
the retained adoption offline for that outcome/origin, but
the absent installed evaluation/policy pins still leave every model profile
unqualified. No caller boolean or sender string creates acceptance.

The native observation control now uses the canonical static runner and binds
its frozen generation and exact receipt/repo/head approval at the same
MainActor launch turn as `Process.run`; confirmation is consumed synchronously
before starting the operation task. The lifecycle append path also refuses an
unterminated nonempty journal tail under the existing lock, preserving bytes and
returning an actionable error rather than concatenating JSON records.

The router reader remains **custody only** in the absence of the installed
signature boundary.
`serve.go` authenticates sessions, but unscoped sends do not universally bind
From to the creating session. Scoped lane admission tests From against token
scope, not exact creating identity. Item exposes no immutable creating-session
proof; ItemSession/GetSession are server-only current-lease machinery, not a
historical reviewer API. No new role/keyring/auth service or router mutation is
introduced to obscure this gap.

No public JSON boolean, qualified stamp, expected issuer, provider/tier string
or legacy frontier-gate flag can set the private accepted context. The verifier
checks only exact manifest pins; no accepted empirical evidence is present.
Owner adoption records the operator's explicit acceptance of the exact retained
review/origin pair, not proof of the reviewer's identity. Private inert accepted
fixtures test the pure machine only; they do not satisfy real operands. Default
host.v1, old journal reads, repair and preflight remain model-free. No
pre-push/CI probabilistic consumer is activated.

## Qualification proposal (not executed)

- Go normal/race: `./internal/maat -run '^TestQualification'`;
  `./internal/maat/lifecycleevidence -run '^TestCanonicalLifecycle'`;
  actual handler `./cmd/sirsi -run '^TestMaatLifecycleActualCommand'`.
- Retain original v1/v2 strict-reader/host/preflight/source-import compatibility
  selectors from f514; no inherited result replaces changed-source qualification.
- Swift strict release and `MaatLifecycleTests` plus source import/operation tests.
- Negative overlays: omit atomic lock/dedup/history-completeness/digest/head fence,
  label canonical custody accepted, drop independent outcome/profile binding,
  forget earliest invalidation, or omit native pre-await approval consumption.
  Require compiling semantic fixture failures, not build failures or invented runs.

Installed UI/workflow qualification and any full calibrated/gate-ready claim are
separate from this source checkpoint. Main owns normal admitted qualification.
