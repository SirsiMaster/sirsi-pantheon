# Cloud Worker Placement and Funding Law

> Repository mirror. Canonical root law: `~/Development/AGENTS.md` §
> "Cloud Worker Placement and Funding Law". Owner Reading Room: *Cloud Worker
> Placement and Funding Law.md*. Rule SHA-256: `97327e9d4de9588de93cd540c0e230363eec4686bcc69c11935356ca9cd89f49`.
> Decreed 2026-10-11. Binding on every present and future thread, runner,
> worker, lane, and process in sirsi-pantheon.

## The law

Prefer Codex Cloud and Claude Cloud for suitable code, review, and
documentation work. Hardware measurements and native GPU qualification stay
on the Macs — cloud workers cannot substitute for measurements on the target
Macs. Identify cloud budgets and present them to Ma'at before working so
billing never exceeds available cloud funding.

- All lanes are released from the blanket cloud-work hold; provider budget
  admission still applies. Do not substitute paid infrastructure deployment
  for this instruction.
- Before cloud-worker work: present Ma'at the provider/account identity,
  verified included allowance and/or available funded credits and expiry,
  current spend and outstanding commitments, worst-case job cost, and
  enforceable resource/concurrency/runtime limits.
- Start only after Ma'at admits the job within the remaining funded
  envelope. Reserve its worst-case cost; release/reconcile after completion.
  Concurrent jobs share that envelope.
- Unknown funding/allowance, expired admission, or unavailable budget
  enforcement means no new metered cloud jobs. Continue suitable unbilled
  work or native work within existing admissions.
- Budget alerts alone are not spending caps. Enforce the admitted ceiling
  before dispatch and stop work before it can exhaust funding; include
  compute, model/API, storage, egress, and teardown costs.
- Ra and Pantheon own propagation into worker startup and dispatch; Ma'at
  owns budget admission. SHA supplies native host facts.

## Adoption in sirsi-pantheon (this repo)

- **No new dispatch framework.** The existing dispatcher — `internal/dispatch`
  (agent registry facade) + `internal/router/dispatchgate.go` (progress gate +
  spawn-rate ceiling, ADR per #636/#639) — is the integration point. Placement
  preference (cloud-suitable vs. native-only work) is a dispatch-time policy
  check layered on this existing path, not a parallel system.
- **No standalone Ma'at router identity exists in the verified registry**
  (confirmed against `.agents/idea-router/agents.json`). Budget admission is
  carried by a separate restricted message per the owner decree; this repo
  does not fabricate a Ma'at agent to route around that.
- **Budget/enforcement changes are blocked on measured provider access**
  (included allowance, funded credits, spend-to-date) that this worker does
  not hold. Policy adoption (this document + the dispatch-point note above)
  is in effect now; metered cloud-job admission remains gated until that
  provider-specific evidence is presented to Ma'at's governance path.
- **Mercury-runner** retains native hardware execution, unaffected by this
  law.
