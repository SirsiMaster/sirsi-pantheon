# ADR-070 — Ma'at structured verdict + screen→escalate flow (jev-style QA gate)

- **Status:** Revised 2026-10-01 for re-review — codex-pantheon CHANGES REQUIRED (item 20260930-225319) amendments 1-5 applied below; owner-directed 2026-09-27 ("Ma'at is being rewritten to operate more like jev"). Drafted by `ra` for the router-gate consumer side; Ma'at itself is cross-Pantheon (ADR-004), owned by claude-home/claude-pantheon.
- **Date:** 2026-09-27
- **Steward:** `ra` (proposer); Ma'at custodian owns acceptance.
- **Refs:** ADR-004 (Ma'at QA/QC governance — the agent this evolves), ADR-034 / PANTHEON_RULES A29–A30 (Orchestration Brain, model tiering), A17 (Feather Weight, QA Sovereign), A28 (armed CI/pre-push gate), A34 (a bind cannot override a live rejection), A35 (scope the check to the claim), A11 (privacy/sovereignty). External: `jev` / System One Models, TypeSafe AI (https://typesafe.ai/blog/introducing-system-one-models-and-jev).

## 1. Context

Ma'at (𓆄, the QA Sovereign, A17) is being rewritten to operate like **jev** — a "System One" model that returns a **type-safe structured value with calibrated confidence** in one parallel query, ~100× faster/cheaper than a chat LLM, for exactly the *classify / route / score / branch* decisions a quality gate makes. Today Ma'at's judgment is LLM-shaped (prose a human reads); a jev-style Ma'at emits a machine-actionable verdict software gates on directly.

The router's pre-push + CI gate is a Ma'at consumer. For the gate to consume Ma'at without weakening it, two things must be nailed down: (1) a **strict verdict schema** (jev requires a predefined output shape, zero type errors), and (2) a **screen→escalate flow** that keeps the fast probabilistic model from *binding* where canon says a binding verdict must be frontier and irreversible-by-bind. This ADR fixes both as a contract the router gate can code against when the new Ma'at lands.

The failure mode to avoid is the A35 class: a check narrower than its claim. A fast model that renders a *binding* pass is a forgery of a verdict when it is only a *screen* — the same shape as A34's PR #416 (a bind that contradicted the reviewer's actual verdict).

## 2. Decision

Ma'at emits one **strict, versioned verdict schema**, produced by a two-stage **screen→escalate** flow: a fast jev-style screen decides the clean/confident cases and **escalates everything else to a frontier (Tier-2) review** for the binding call. A **deterministic floor** (gofmt/vet/lint/`-race`) sits outside the model and can never be overridden by it.

### 2.1 Verdict schema (the type-safe contract)

The gate reads only this. No prose; every field is machine-actionable.

```go
// MaatVerdict is the sole output of a Ma'at screen (jev-style). Strict schema,
// zero type errors — the gate acts on this, nothing else.
type MaatVerdict struct {
    SchemaVersion int          `json:"schema_version"`
    Subject       Subject      `json:"subject"`
    FeatherWeight int          `json:"feather_weight"` // 0..100 (A17)
    Gate          GateDecision `json:"gate"`           // pass|changes|block|escalate (pass = non-binding SCREEN pass, see 2.3 inv. 6)
    Confidence    float64      `json:"confidence"`     // calibrated 0..1 for THIS gate call
    Findings      []Finding    `json:"findings"`       // most-severe first
    Floor         FloorResult  `json:"floor"`          // deterministic checks, OUTSIDE the model
    Escalation    *Escalation  `json:"escalation,omitempty"`
    Model         ModelStamp   `json:"model"`          // provenance (A29 no-black-box)
}

type GateDecision string

const (
    GatePass     GateDecision = "pass"     // clean screen — NON-BINDING; never the final approval
    GateChanges  GateDecision = "changes"  // fixable findings, non-binding advice
    GateBlock    GateDecision = "block"    // floor red OR confirmed blocking finding
    GateEscalate GateDecision = "escalate" // not confident enough to bind → frontier
)

type Subject struct {
    Kind     string `json:"kind"`      // pr|diff|file|commit
    Repo     string `json:"repo"`
    Ref      string `json:"ref"`
    HeadSHA  string `json:"head_sha"`  // verdict is PINNED to this head (A34)
    Boundary string `json:"boundary"`  // ""|security|delivery|identity — raises the bar
}

type Finding struct {
    ID         string  `json:"id"`         // stable per (rule,file,line)
    Severity   string  `json:"severity"`   // block|major|minor|nit
    Category   string  `json:"category"`   // over-claim|scope-gap|correctness|unscoped-check|...
    File       string  `json:"file"`
    Line       int     `json:"line"`
    Claim      string  `json:"claim"`      // one sentence: the defect
    EvidenceID string  `json:"evidence_id"` // independently addressable: digest of this finding's cited proof
    Evidence   string  `json:"evidence"`   // the cited proof (A35: scoped to the claim)
    Confidence float64 `json:"confidence"` // calibrated 0..1 for THIS finding
    FixHint    string  `json:"fix_hint,omitempty"`
}

type FloorResult struct { // deterministic, the model can never override
    Passed bool    `json:"passed"`
    Checks []Check `json:"checks"` // gofmt|vet|golangci-lint|test-race
}

type Check struct {
    Name   string `json:"name"`
    Passed bool   `json:"passed"`
    Detail string `json:"detail,omitempty"` // failing output when !passed
}

type Escalation struct {
    Reason     string  `json:"reason"`      // why the screen couldn't bind
    MissedConf float64 `json:"missed_conf"` // threshold it fell short of
    ReviewTier string  `json:"review_tier"` // "frontier" (Tier-2)
}

type ModelStamp struct {
    Provider  string `json:"provider"`  // jev|local:<id>
    Version   string `json:"version"`
    EvidenceSetDigest string `json:"evidence_set_digest"` // digest of the recipe: diff + context + prompt + every evidence input
    CalibrationID     string `json:"calibration_id"`      // the pinned labelled eval set + threshold table the confidence was calibrated against
    Calibration       string `json:"calibration"`         // qualified|unqualified — unqualified is fail-closed (2.3 inv. 8)
    Local     bool   `json:"local"`     // A11: did source leave the host?
    LatencyMs int    `json:"latency_ms"`
}
```

The load-bearing fields are the **calibrated `Confidence`** (per-verdict and per-finding — this is what jev buys, and it is A35 "scope the check to the claim" made native), **`Subject.HeadSHA`** (the verdict is about one head), **`Subject.Boundary`** (raises the bar), and **`Floor` kept separate from the model** (A28/A35), and the **provenance digests** (evidence set, calibration identity) that make a verdict reproducible and auditable.

### 2.2 Screen→escalate flow

```
INPUT: subject{diff, head_sha, boundary}

STEP 0 — DETERMINISTIC FLOOR (no model, A28)
    run gofmt · vet · golangci-lint · go test -race
    if any red:  Gate=block, Floor.passed=false, FeatherWeight capped → RETURN
    // A35: a probabilistic "pass" never overrides a deterministic "red".

STEP 1 — SCREEN (jev-Ma'at, fast, type-safe)
    diff+context → draft MaatVerdict: feather_weight, findings[]+conf, gate-conf

STEP 2 — DECIDE: one deterministic, exhaustive precedence. The FIRST matching rule wins;
         rules are evaluated in this order and nothing is evaluated out of it.
    R0  schema invalid ∨ unknown enum/severity/gate ∨ score outside 0..100 ∨ confidence
        NaN/Inf/outside 0..1 ∨ missing required field                  → REJECT the verdict
        (treated as a failed screen: ESCALATE; never coerced, never defaulted)
    R1  floor failed or missing                                        → BLOCK
    R2  Subject.Boundary ≠ "" (tested BEFORE and independent of findings)
        ∨ Calibration = unqualified                                    → ESCALATE
    R3  gate-conf < τ (provisional, see 2.4)                           → ESCALATE
    R4  a finding severity=block with conf ≥ τ                         → BLOCK
    R5  any finding severity=major (any confidence)                    → ESCALATE (frontier decides)
    R6  only minor/nit findings                                        → CHANGES (advisory)
    R7  no findings ∧ feather_weight ≥ 85                              → PASS (non-binding screen pass)
    R8  none of the above (e.g. feather_weight < 85, no findings)      → ESCALATE
    Every input maps to exactly one outcome; R8 makes the table exhaustive.

STEP 3 — ESCALATE → FRONTIER (A30/A34)
    a Tier-2 review runs on the SAME head_sha, takes the screen's findings as leads,
    and returns its OWN binding MaatVerdict. THAT verdict merges/blocks.
    A34: a BLOCK/CHANGES here cannot be cleared by a later bind without a new head
         + new review, or an explicit owner override naming the finding.
```

### 2.3 Invariants (violating any makes the verdict a forgery)

1. **Floor red ⇒ block, always.** The model cannot un-red a deterministic failure.
2. **Boundary can't auto-pass.** security/delivery/identity changes always escalate to frontier for the binding call (A30: binding verdicts stay Tier-2). The boundary test is R2, ahead of every findings-based rule: zero findings at 0.99 confidence on a boundary subject still escalates. The confidence threshold never substitutes for frontier review.
3. **Low confidence ⇒ escalate, never guess.** Below τ, a frontier makes the call (A35: a screen is not a verdict).
4. **Verdict pinned to `head_sha`.** A pass/block is about that head; new commits ⇒ new screen (A34).
5. **Provenance stamped.** Provider, version, evidence-set digest, calibration identity and local/hosted recorded; every finding's evidence individually addressable; no black-box gate (A29). `Local=false` is a visible A11 sovereignty cost.
6. **A screen pass never supplies the binding approval.** A30: EVERY binding verdict stays frontier, not only boundary changes. `GatePass` means "the screen found nothing"; a confidence number cannot convert a screen into a verdict, and no consumer may treat `GatePass` as the independent approval A34 requires. Merge approval comes from a frontier/independent review of the same head.
7. **Invalid input never passes.** Schema violations, unknown enums, out-of-range or non-finite numbers are rejected (R0), never coerced.
8. **Unqualified calibration fails closed.** Until a pinned labelled eval set supports the thresholds (2.4), `Calibration` is `unqualified` and every call escalates (R2): the screen is advisory only.

### 2.4 Calibration harness (the operational must)

jev's value is that "0.90 confidence is right ~90% of the time" — true only if verified in **both directions** (A35: a guard never shown red is untested).

- **Thresholds are provisional.** τ = 0.90 (0.98 on a boundary) and the feather_weight floor are placeholders until a **pinned, labelled eval set** (`CalibrationID`) supports them. Until then `Calibration = unqualified` and the flow escalates everything (inv. 8). The earlier "fast 90% path" was a design hope, not a measured acceptance rate; this ADR makes no such claim.
- **Sample the auto-passes, not only the escalations.** Every PASS (R7) and CHANGES (R6) is eligible for an independent frontier re-review at a sampled rate; the **auto-PASS overturn rate** is measured against those independent outcomes. Logging only the escalations measures the screen where it already doubted itself and cannot detect false assurance.
- **Raise τ on drift.** If the screen ever auto-passes something an independent frontier outcome would have blocked, calibration is broken: set `Calibration = unqualified` and raise τ before any further auto-pass is trusted.
- **Re-qualify on change.** A new model version, prompt, or evidence recipe changes `EvidenceSetDigest`/`CalibrationID` and starts unqualified.

## 3. Alternatives Considered

1. **Keep Ma'at as an LLM that renders prose verdicts** — Rejected: not machine-gateable, slow, and uncalibrated; the gate can't act on prose and a human must re-read every run. This is the state the rewrite replaces.
2. **Let the fast model render the *binding* merge verdict directly** — Rejected: violates A30 (binding verdicts on security/delivery boundaries stay frontier) and A34 (a probabilistic pass would be a bind that can contradict the actual verdict). The screen→escalate split exists precisely to keep speed without a forged bind.
3. **Replace the deterministic checks with the model** — Rejected: A35/A28 — `gofmt`/`vet`/`lint`/`-race` are deterministic ground truth; a model "pass" must never override a real red. The floor stays outside the model.
4. **Adopt hosted jev as the default provider** — Rejected as a default (kept as an option): jev is hosted, so our source would leave the host (A11 sovereignty cost). `ModelStamp.Local` records it; a local System-One equivalent is preferred for the default screen, with hosted jev an opt-in per-role provider (A29 pluggable brain).

## 4. Consequences

**Positive:** the gate consumes a strict, versioned, auditable verdict; the clean case clears the screen fast and cheap (as a non-binding pass; the rate is to be measured, 2.4); every finding carries scoped evidence + calibrated confidence (A35 native); binding verdicts stay frontier and irreversible-by-bind (A30/A34); the deterministic floor is untouched (A28); provenance is explicit (A29).

**Costs / obligations:** requires a calibration harness (§2.4) or the fast gate drifts into false assurance; hosted jev carries an A11 sovereignty cost that must be a conscious per-role choice; the router gate must be updated to read `MaatVerdict` (a small consumer change, `SchemaVersion`-guarded) when the new Ma'at ships.

**Router-side follow-up (ra):** keep the current deterministic gate until the corrected schema, consumer and negative-control tests are independently accepted (this revision supersedes no live rejection or safety gate). Then, when Ma'at emits `MaatVerdict`, wire the router's pre-push + CI gate to consume it, keeping the deterministic floor beneath. Until then the router keeps its current gate; no code changes land from this ADR alone.

## 5. References

- ADR-004 (Ma'at QA/QC Governance — the agent this evolves), ADR-034 (Orchestration Brain), ADR-062 (router service).
- PANTHEON_RULES A17 (Feather Weight / QA Sovereign), A28 (armed gate), A29–A30 (Orchestration Brain / model tiering), A34 (bind cannot override a rejection), A35 (scope the check), A11 (privacy/sovereignty).
- `jev` / System One Models, TypeSafe AI — https://typesafe.ai/blog/introducing-system-one-models-and-jev
- Memory: `project_maat_rewritten_jev_style_structured_decisions`.
