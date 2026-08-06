# SNE-31 Independent Review — Multi-Mac Sharding Design

**Reviewer:** claude-pantheon  
**Review commit:** `2ab2a845692edf850f3de59d49542e7f2ed8abff`  
**File reviewed:** `docs/design/MULTI-MAC-SHARDING-DESIGN.md`  
**Review date:** 2026-08-05  
**Verdict:** **CHANGES REQUIRED**

---

## Summary

The design is structurally sound: boundaries are drawn cleanly, the
pipeline-first JACCL decision is well-justified, the epoch model is tight, KV
and COW semantics extend SNE-41 correctly without weakening it, the failure
table is fail-closed, and the Exo comparison is honest and disciplined.

Two open questions in the document (Q1 and Q4) prevent the design from being
"precisely enough to implement and test" per the `/goal` acceptance criterion.
Both must be resolved before the design can be accepted and implementation
slices 3 and 4 can proceed. All other findings are informational.

---

## Dimension-by-dimension findings

### 1. Pantheon/SNE ownership boundaries — PASS

The boundary table assigns every responsibility exactly once. The flow is
clean: Ra proposes a `ClusterPlan`; every rank validates it locally; SNE owns
compute and KV; Pantheon owns topology, consent, process launch, and the model
router. The statement "Anubis never negotiates process flags or rank layout
over the public chat contract" is an explicit seam, not implied.

The `model and tokenizer identity | SNE, verified by Pantheon` row could be
read two ways: who initiates the identity check? The ClusterPlan fields
(`model_manifest_sha256`, `tokenizer_sha256`, `chat_template_sha256`) clarify
this: Ra supplies the expected checksums; SNE verifies its local files match.
That is sound. Calling it out because a careless implementor could read the
table alone and put the verification logic in the wrong binary.

No changes required for this dimension.

### 2. Pipeline-first JACCL decision — PASS

The substrate is verified present: `third_party/mlx-c/mlx/c/distributed_group.h`,
`distributed.h`, `libjaccl.a`. The statement "the open engineering question is
model partitioning and lifecycle, not whether SNE should invent transport
primitives" is directly supported by the substrate inventory.

The pipeline-first ordering over tensor parallelism is correct: pipeline adds
exactly one boundary activation and one token/control message per decode step,
avoids per-layer all-reduces, and yields a correctness oracle against
single-node SNE before collective latency needs to be measured.

The TCP/RING canary before JACCL qualification is a good CI fallback:
transport-independent correctness can be proven without physical Thunderbolt
topology.

Open question Q3 (retain RING canary as supported fallback or only as test
fixture) is appropriately deferred and does not block slice 2 or 3. It must be
answered before slice 6 (JACCL qualification) to avoid parallel-path confusion.

No changes required for this dimension.

### 3. Fixed topology epochs — PASS with note

The epoch fields in `ClusterPlan` are complete, mismatch is a startup error
rather than a warning, and the failure table correctly states "never hot-swap a
rank mid-token." The epoch is immutable while requests are active. This is
tight.

**Note (maps to Q4, flagged as CHANGES REQUIRED in §4 below):** The document
specifies "Ra proposes a `ClusterPlan`" but does not specify what Pantheon
identity signs the plan or what the lease duration is when the control plane is
temporarily unavailable. Without a signing identity, a rank cannot authenticate
the ClusterPlan it receives at startup. Open question Q4 names this gap
correctly. It must be resolved before the epoch model can be called
implementation-ready. See §Changes Required below.

### 4. Rank-local transactional KV/COW semantics — PASS

The design extends the SNE-41 transaction rule correctly:

- Each rank owns KV only for its assigned layers; active KV is never
  centralized.
- COW and cold tiers are rank-local.
- A distributed prefix transaction commits only after every rank reports
  success; otherwise all ranks discard.
- A prefix hit is valid only when every rank proves the same prefix domain,
  token identity, model manifest, layer range, and local block identity — a
  strict condition that prevents partial cache hits from causing inconsistency.

"Prefix metadata may be content-addressed across ranks" implies some metadata
coordination but the commit protocol (coordinator, timeout, failure path) is
not fully specified. However, the high-level two-phase rule ("all succeed or
all discard") is stated explicitly, and the detail is appropriate to defer to
implementation slice 3. Flagged informational, not blocking.

No changes required for this dimension.

### 5. Failure and recovery behavior — PASS

The failure table is comprehensive and consistently fail-closed:

| Failure | Verdict |
|---|---|
| rank missing before admission | correct — reject, model router may select alternate lane |
| rank loss during prefill/decode | correct — cancel whole request, terminate epoch, no fabricated completion |
| checksum/config mismatch | correct — refuse group formation |
| collective timeout | correct — cancel, mark group unhealthy |
| client disconnect | correct — bounded cancellation + KV release verification |
| Ra/Pantheon loss | conditionally correct — see note below |
| topology change | correct — drain/cancel, increment epoch, fresh group |
| restart | correct — rank-local cold cache only, no active request resurrection in v1 |

**Note on Ra/Pantheon loss:** "existing admitted request may finish only within
its fixed epoch and deadline." This requires SNE to hold a locally cached
ClusterPlan with a lease. The lease duration is open question Q4. Without it,
an implementor cannot determine when in-flight requests must be cancelled after
Ra disappears. This is the same gap as in §3 and is covered in the CHANGES
REQUIRED section below.

"There is no transparent single-node continuation of a partially generated
distributed request in v1." — Correct. Partial output would be unsound; the
design correctly rejects it.

No additional changes required beyond the Q4 resolution already flagged.

### 6. Exo comparison and claim discipline — PASS

The comparison is disciplined. Specific findings:

- The authority source is the current Exo repository and signed releases, not
  the archived pre-1.0 project — correct.
- Exo's scaling claim ("up to 1.8x on 2 devices, 3.2x on 4") is cited without
  adoption; SNE makes no equivalent claim. Correct.
- "Exo is presently broader and more mature in multi-node product behavior" —
  honest acknowledgment.
- SNE's stated design advantages ("sealed-engine seam, one-owner boundaries,
  deterministic proof, fail-closed epochs, integrated Pantheon/Hypergraph
  evidence") are correctly qualified: "Those remain design claims until
  implemented."
- The WWDC 2026 session reference (session 233) cannot be independently
  verified in a static review; the reviewer notes it as an unverified
  informational reference.

No changes required for this dimension.

### 7. Implementation slices — PASS with note

The seven slices are correctly ordered: contract → substrate → compute →
admission → faults → qualification → tensor. No slice depends on a later one.
Slice 4 correctly places "Pantheon launch/admission integration in its own
repo," respecting A26 repo segmentation.

**Note:** Slice 3 ("exact prompt/token oracle against single-node SNE") implies
the oracle prompt and token-count set is pinned before implementation begins.
Completion gate 1 references "declared prompts and token counts" — so the gate
exists, but the oracle set is not listed in the design document. It should be
pinned in the implementation plan for slice 3 before that slice begins, to
prevent correctness-oracle drift.

**Note:** Open question Q1 (embedding and LM head placement on rank 1 vs
replicated) directly affects the partition specification in slice 3. An
implementor cannot write slice 3 without resolving Q1. This is flagged as
CHANGES REQUIRED below.

### 8. Completion and claim gates — PASS with note

The ten gates are thorough. Gate 9 (independent review per slice) and gate 10
(three-home synchronization) are particularly strong controls.

Gate 7 ("one cold discard plus at least three measured trials") is consistent
with the existing SNE benchmark policy — correct.

**Note on gate 5:** "stale-epoch and partial-group admission rejection" should
explicitly include testing the Pantheon model-router fallback lane (the
"model router may select a separately qualified lane" row in the failure
table). As written, gate 5 tests rejection; it should also test that the
fallback lane is correctly selected when one exists. Informational; does not
block acceptance.

---

## CHANGES REQUIRED

### CR-1: Resolve Q1 (embedding/LM head placement) before closing design

**Evidence:** The partition definition in slice 3 reads "Rank 1 owns
transformer layers `[split, L)`, final norm, LM head, sampling, and response
projection." This statement implies embedding and LM head are on rank 1, but
the open question asks whether they should remain on rank 1 or be replicated to
rank 0 to reduce the final token/control path.

An implementor writing slice 3 must choose one layout and implement it. Two
unresolved layouts cannot coexist in the oracle test. The design document leaves
this ambiguous: the main text assigns LM head to rank 1, but Q1 introduces
doubt. Resolving Q1 either removes Q1 or amends the partition table. Either is
acceptable; ambiguity is not.

**Required action:** Either (a) close Q1 by confirming LM head on rank 1 with
a brief rationale and removing Q1 from the open questions, or (b) amend the
partition table to show the replicated-LM-head layout and close Q1 accordingly.
The chosen layout must appear unambiguously in the design before slice 3 can
begin.

### CR-2: Resolve Q4 (ClusterPlan signing identity and lease duration) before closing design

**Evidence:** The failure table states "existing admitted request may finish
only within its fixed epoch and deadline" after Ra/Pantheon loss. The epoch
model states the epoch is immutable while requests are active. Both require:

1. A Pantheon identity that signs the ClusterPlan so ranks can authenticate it
   at startup (without this, a rank cannot detect a spoofed or replayed plan).
2. A lease duration that bounds how long a locally cached ClusterPlan remains
   valid when Ra is unreachable (without this, the "finish within its fixed
   epoch and deadline" rule has no concrete timeout for implementors).

Open question Q4 correctly identifies both. Because slice 4 (fleet admission)
and slice 6 (JACCL qualification) both depend on authenticated plan delivery
and Ra-loss behavior, Q4 must be resolved before the design can be accepted.

**Required action:** Either (a) specify the Pantheon identity (e.g., a
Ra-issued HMAC key or signed JWT with a named expiry field) and a representative
lease duration, or (b) explicitly defer the signing identity to a named ADR
(e.g., ADR-055 or an existing equivalent) with a binding reference, closing Q4
with a pointer. In either case, the lease duration must appear in the design or
in the referenced ADR before slice 4 begins.

---

## Informational findings (no changes required to accept)

- **I-1:** The `model and tokenizer identity` boundary-table row could be
  clarified: Ra supplies expected checksums; SNE validates local files. A
  one-line implementation note here would prevent the identity check from
  landing in the wrong binary.

- **I-2:** Cross-rank prefix metadata commit protocol (coordinator, timeout,
  failure path) is not specified beyond "all succeed or all discard." Acceptable
  at design level; should be specified in the implementation plan for slice 3.

- **I-3:** Oracle prompt and token-count set for the slice-3 correctness
  oracle is not named in the design. Should be pinned before slice 3 begins.

- **I-4:** Gate 5 should be extended to also test the Pantheon model-router
  fallback lane, not only the rejection case.

- **I-5:** Q3 (RING canary disposition) should be answered before slice 6
  (JACCL qualification) to prevent parallel-path confusion. Acceptable to
  defer it past slice-2 testing.

---

## Open questions from the document — disposition

| Q# | Status | Notes |
|---|---|---|
| Q1 | **CHANGES REQUIRED — resolve before close** | Ambiguous partition; slice 3 cannot proceed |
| Q2 | Deferred — acceptable | Exact split ratio is a measured implementation detail |
| Q3 | Deferred — acceptable before slice 6 | RING canary as fallback vs. test-only; answer before JACCL qualification |
| Q4 | **CHANGES REQUIRED — resolve before close** | ClusterPlan signing + lease duration; security and recovery gap |
| Q5 | Deferred — acceptable | Binary identity before distribution is a release policy question |

---

## Verdict

**CHANGES REQUIRED**

Resolve CR-1 and CR-2, then resubmit for final accept. The design is otherwise
sound and ready to drive implementation once those two questions are closed.
Informational findings I-1 through I-5 do not block acceptance and may be
addressed in implementation plans for the relevant slices.
