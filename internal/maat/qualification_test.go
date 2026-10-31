package maat

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type lifecycleFixtureResolver map[string]LifecycleEvidence

func (r lifecycleFixtureResolver) ResolveLifecycle(_ context.Context, id string) (LifecycleEvidence, error) {
	v, ok := r[id]
	if !ok {
		return v, fmt.Errorf("unknown canonical receipt")
	}
	return v, nil
}

// Private accepted operands exercise the pure state machine; they do not
// establish real producer/reviewer provenance or populate the production adapter.
func lifecycleFixtures() (SourceProfileIdentity, lifecycleFixtureResolver) {
	policy := QualificationPolicy{ID: "inert-policy", Threshold: .90, BoundaryThreshold: .98, MinimumPass: 1, MinimumChanges: 1}
	p := SourceProfileIdentity{Producer: "fixture-producer", ArtifactSHA: strings.Repeat("a", 64), ModelVersion: "fixture-v1", PromptSHA: strings.Repeat("b", 64), RecipeSHA: strings.Repeat("c", 64), PolicySHA: lifecycleDigest(policy)}
	base := LifecycleReceipt{Schema: LifecycleSchema, Kind: "policy", Profile: p, Repo: "fixture/repo", HeadSHA: strings.Repeat("a", 40), Policy: policy, RunID: "owned-inert-evaluation", Samples: []LifecycleSample{}, SelectedIDs: []string{}}
	r := lifecycleFixtureResolver{}
	add := func(id, issuer string, receipt LifecycleReceipt) {
		raw, _ := json.Marshal(receipt)
		r[id] = LifecycleEvidence{ID: id, SHA: lifecycleRawFixtureSHA(raw), Issuer: issuer, Receipt: receipt, accepted: true}
	}
	add("policy", "independent-policy-review", base)
	s1 := sourceFixture()
	s2 := sourceFixture()
	s2.Subject.HeadSHA = strings.Repeat("b", 40)
	s2.Findings = []ScreenFinding{sourceFinding("minor", "minor", 1)}
	o := base
	o.Kind = "outcome"
	o.CandidateGate = GatePass
	o.FinalGate = GatePass
	o.ScreenSHA = lifecycleDigest(s1)
	add("pass-review", "independent-reviewer", o)
	o.HeadSHA = s2.Subject.HeadSHA
	o.CandidateGate = GateChanges
	o.FinalGate = GateChanges
	o.ScreenSHA = lifecycleDigest(s2)
	add("changes-review", "independent-reviewer", o)
	e := base
	e.Kind = "evaluation"
	e.PolicyRef = "policy"
	e.Samples = []LifecycleSample{{ID: "pass", Screen: s1, OutcomeRef: "pass-review"}, {ID: "changes", Screen: s2, OutcomeRef: "changes-review"}}
	e.SelectedIDs = []string{"pass", "changes"}
	e.ResultsSHA = lifecycleDigest(e.Samples)
	e.DatasetSHA = lifecycleDigest([]string{"pass:fixture/repo:" + s1.Subject.HeadSHA, "changes:fixture/repo:" + s2.Subject.HeadSHA})
	add("evaluation", p.Producer, e)
	o = r["pass-review"].Receipt
	o.FinalGate = GateBlock
	add("overturn", "independent-reviewer", o)
	return p, r
}

func lifecycleRawFixtureSHA(raw []byte) string { // exact-byte helper, not a receipt verifier
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

func lifecycleEvent(v LifecycleEvidence) QualificationEvent {
	return QualificationEvent{Schema: LifecycleSchema, ReceiptID: v.ID, ReceiptSHA: v.SHA, Profile: v.Receipt.Profile, HeadSHA: v.Receipt.HeadSHA}
}

func TestQualificationReplayVerifiedFixturesCoverageDriftAndPublicImport(t *testing.T) {
	p, r := lifecycleFixtures()
	ctx := context.Background()
	e := lifecycleEvent(r["evaluation"])
	state, err := ReplayQualification(ctx, p, []QualificationEvent{e, e}, r)
	if err != nil || state.Status != "qualified" || state.PassSamples != 1 || state.ChangesSamples != 1 || state.Epoch != 1 {
		t.Fatalf("pure fixture replay=%+v err=%v", state, err)
	}
	state, err = ReplayQualification(ctx, p, []QualificationEvent{e, lifecycleEvent(r["overturn"]), e}, r)
	if err != nil || state.Status != "requalification_required" || state.Overturns == 0 || state.Conflicts == 0 || state.ThresholdMustExceed < .90 {
		t.Fatalf("overturn lost: %+v %v", state, err)
	}
	for _, field := range []string{"prompt", "recipe", "version", "artifact"} {
		changed := p
		switch field {
		case "prompt":
			changed.PromptSHA = strings.Repeat("d", 64)
		case "recipe":
			changed.RecipeSHA = strings.Repeat("d", 64)
		case "version":
			changed.ModelVersion = "new"
		default:
			changed.ArtifactSHA = strings.Repeat("d", 64)
		}
		state, err = ReplayQualification(ctx, changed, []QualificationEvent{e}, r)
		if err != nil || state.Status != "unqualified" {
			t.Fatalf("profile inherited state: %+v %v", state, err)
		}
	}
	s := sourceFixture()
	s.Model.Calibration = "qualified"
	if _, err := ScreenSource(s, s.Subject.HeadSHA); err == nil {
		t.Fatal("private lifecycle fixture enabled public qualified import")
	}
}

func TestQualificationUnknownMissingSelfReviewAndIncompletePopulation(t *testing.T) {
	for _, defect := range []string{"unestablished", "missing-policy", "missing-label", "self-review", "wrong-head", "wrong-profile", "zero-minimum", "only-pass-selection", "selected-escalations"} {
		t.Run(defect, func(t *testing.T) {
			p, r := lifecycleFixtures()
			v := r["evaluation"]
			switch defect {
			case "unestablished":
				v.accepted = false
				v.Reason = "origin unknown"
				r[v.ID] = v
			case "missing-policy":
				delete(r, "policy")
			case "missing-label":
				delete(r, "changes-review")
			case "self-review":
				o := r["pass-review"]
				o.Issuer = p.Producer
				r[o.ID] = o
			case "wrong-head":
				o := r["pass-review"]
				o.Receipt.HeadSHA = strings.Repeat("f", 40)
				r[o.ID] = o
			case "wrong-profile":
				o := r["pass-review"]
				o.Receipt.Profile.PromptSHA = strings.Repeat("f", 64)
				r[o.ID] = o
			case "zero-minimum":
				v.Receipt.Policy.MinimumPass = 0
				r[v.ID] = v
			case "only-pass-selection":
				v.Receipt.SelectedIDs = []string{"pass"}
				r[v.ID] = v
			case "selected-escalations":
				v.Receipt.Samples[0].Screen.Confidence = .1
				v.Receipt.ResultsSHA = lifecycleDigest(v.Receipt.Samples)
				r[v.ID] = v
			}
			state, err := ReplayQualification(context.Background(), p, []QualificationEvent{lifecycleEvent(v)}, r)
			if err == nil && state.Status == "qualified" {
				t.Fatalf("%s falsely qualified: %+v", defect, state)
			}
		})
	}
}

func TestQualificationStrictReceiptKeysRequiredAndDigests(t *testing.T) {
	_, r := lifecycleFixtures()
	receipt := r["evaluation"].Receipt
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	// The strict source-v2 wire contract requires explicit empty boundary and
	// host_binding members for every actual commit sample subject.
	for _, sample := range receipt.Samples {
		needle := []byte(`"head_sha":"` + sample.Screen.Subject.HeadSHA + `"}`)
		if sample.Screen.Subject.Boundary != "" || bytes.Count(raw, needle) != 1 {
			t.Fatal("expected one exact commit fixture subject with empty boundary and host binding per sample")
		}
		raw = bytes.Replace(raw, needle, []byte(`"head_sha":"`+sample.Screen.Subject.HeadSHA+`","boundary":"","host_binding":""}`), 1)
	}
	if _, err := DecodeLifecycleReceipt(raw); err != nil {
		t.Fatal(err)
	}
	missingBoundary := bytes.Replace(raw, []byte(`,"boundary":""`), nil, 1)
	if _, err := DecodeLifecycleReceipt(missingBoundary); err == nil || !strings.Contains(err.Error(), `required field "boundary" missing`) {
		t.Fatalf("missing sample boundary was not refused: %v", err)
	}
	missingHostBinding := bytes.Replace(raw, []byte(`,"host_binding":""`), nil, 1)
	if _, err := DecodeLifecycleReceipt(missingHostBinding); err == nil || !strings.Contains(err.Error(), `required field "host_binding" missing`) {
		t.Fatalf("missing sample host binding was not refused: %v", err)
	}
	for _, bad := range [][]byte{append(append([]byte{}, raw...), []byte(" {}")...), bytes.Replace(raw, []byte(`"schema":`), []byte(`"Schema":`), 1), bytes.Replace(raw, []byte(`"kind":"evaluation"`), []byte(`"kind":"evaluation","kind":"policy"`), 1), bytes.Replace(raw, []byte(`"kind":"evaluation",`), nil, 1), bytes.Replace(raw, []byte(`"minimum_pass_samples":1`), []byte(`"minimum_pass_samples":0`), 1)} {
		if _, err := DecodeLifecycleReceipt(bad); err == nil {
			t.Fatalf("invalid receipt admitted: %s", bad)
		}
	}
	v := r["evaluation"].Receipt
	v.ResultsSHA = strings.Repeat("0", 64)
	if err := v.validate(); err == nil {
		t.Fatal("changed result set admitted")
	}
}

func TestQualificationJournalPreviewConfirmDuplicateConflictAndRestartBeyond1000(t *testing.T) {
	_, r := lifecycleFixtures()
	j := &FileDecisionJournal{Path: filepath.Join(t.TempDir(), "decisions.jsonl")}
	ctx := context.Background()
	v := r["evaluation"]
	preview, err := j.ObserveLifecycle(ctx, v.ID, v.Receipt.Repo, v.Receipt.HeadSHA, "", false, r)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(j.Path); !os.IsNotExist(err) {
		t.Fatal("preview created journal")
	}
	first, err := j.ObserveLifecycle(ctx, v.ID, v.Receipt.Repo, v.Receipt.HeadSHA, preview.ReceiptSHA, true, r)
	if err != nil || !first.Recorded {
		t.Fatalf("record=%+v %v", first, err)
	}
	before, _ := os.ReadFile(j.Path)
	second, err := j.ObserveLifecycle(ctx, v.ID, v.Receipt.Repo, v.Receipt.HeadSHA, preview.ReceiptSHA, true, r)
	if err != nil || !second.Duplicate || second.Recorded {
		t.Fatalf("duplicate=%+v %v", second, err)
	}
	after, _ := os.ReadFile(j.Path)
	if !bytes.Equal(before, after) {
		t.Fatal("duplicate appended")
	}
	o := r["overturn"]
	if _, err := j.ObserveLifecycle(ctx, o.ID, o.Receipt.Repo, o.Receipt.HeadSHA, o.SHA, true, r); err != nil {
		t.Fatal(err)
	}
	var historical bytes.Buffer
	for i := 0; i < 1005; i++ {
		d := Decision{Time: "2020-01-01T00:00:00Z", Host: "fixture", Kind: "assessment", Requester: "fixture", Assessed: "legacy", Determination: "pass", Why: "owned inert history"}
		_ = json.NewEncoder(&historical).Encode(d)
	}
	f, err := os.OpenFile(j.Path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Write(historical.Bytes())
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		t.Fatal(err, closeErr)
	}
	restarted := &FileDecisionJournal{Path: j.Path}
	view, err := restarted.ObserveLifecycle(ctx, v.ID, v.Receipt.Repo, v.Receipt.HeadSHA, "", false, r)
	if err != nil || view.State.Status != "requalification_required" {
		t.Fatalf("restart forgot oldest invalidation: %+v %v", view, err)
	}
	changed := v
	changed.SHA = strings.Repeat("f", 64)
	r[v.ID] = changed
	if _, err := restarted.ObserveLifecycle(ctx, v.ID, v.Receipt.Repo, v.Receipt.HeadSHA, changed.SHA, true, r); err == nil {
		t.Fatal("same receipt ID with changed bytes admitted")
	}
}

func TestQualificationJournalConcurrentEvaluationCannotLoseOverturn(t *testing.T) {
	_, r := lifecycleFixtures()
	j := &FileDecisionJournal{Path: filepath.Join(t.TempDir(), "decisions.jsonl")}
	var wg sync.WaitGroup
	type failure struct {
		id  string
		err error
	}
	errs := make(chan failure, 2)
	for _, id := range []string{"evaluation", "overturn"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			v := r[id]
			_, err := j.ObserveLifecycle(context.Background(), id, v.Receipt.Repo, v.Receipt.HeadSHA, v.SHA, true, r)
			if err != nil {
				errs <- failure{id, err}
			}
		}(id)
	}
	wg.Wait()
	close(errs)
	// Existing journal lock is intentionally nonblocking. Explicitly retry only
	// these owned test submissions after both concurrent attempts are terminal.
	for failed := range errs {
		if !strings.Contains(failed.err.Error(), "another local write or repair") {
			t.Fatal(failed.err)
		}
		v := r[failed.id]
		if _, err := j.ObserveLifecycle(context.Background(), failed.id, v.Receipt.Repo, v.Receipt.HeadSHA, v.SHA, true, r); err != nil {
			t.Fatal(err)
		}
	}
	v := r["evaluation"]
	view, err := j.ObserveLifecycle(context.Background(), v.ID, v.Receipt.Repo, v.Receipt.HeadSHA, "", false, r)
	if err != nil || view.State.Status != "requalification_required" {
		t.Fatalf("lost concurrent invalidation: %+v %v", view, err)
	}
}

func TestQualificationJournalTruncationMalformedAndScopeRefuseWithoutAppend(t *testing.T) {
	_, r := lifecycleFixtures()
	v := r["evaluation"]
	j := &FileDecisionJournal{Path: filepath.Join(t.TempDir(), "decisions.jsonl")}
	if err := os.WriteFile(j.Path, []byte("not JSON\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := j.ObserveLifecycle(context.Background(), v.ID, v.Receipt.Repo, v.Receipt.HeadSHA, v.SHA, true, r); err == nil {
		t.Fatal("malformed history admitted")
	}
	if raw, _ := os.ReadFile(j.Path); string(raw) != "not JSON\n" {
		t.Fatal("failed replay changed journal")
	}
	f, err := os.Create(j.Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(lifecycleHistoryBytes + 1); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	if _, err := j.ObserveLifecycle(context.Background(), v.ID, v.Receipt.Repo, v.Receipt.HeadSHA, "", false, r); err == nil {
		t.Fatal("oversized history treated as complete")
	}
	if err := os.Remove(j.Path); err != nil {
		t.Fatal(err)
	}
	for _, scope := range [][2]string{{"other/repo", v.Receipt.HeadSHA}, {v.Receipt.Repo, strings.Repeat("f", 40)}} {
		if _, err := j.ObserveLifecycle(context.Background(), v.ID, scope[0], scope[1], v.SHA, true, r); err == nil {
			t.Fatal("wrong subject admitted")
		}
	}
}

func TestInstalledLifecycleEmptyInventoryAndOpaqueAuthorityKeepFixtureUnqualified(t *testing.T) {
	inventory, err := readLifecycleAcceptanceInventory()
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Policy) != 0 || len(inventory.Evaluation) != 0 || len(inventory.Outcome) != 0 {
		t.Fatalf("fixture data unexpectedly populated installed acceptance inventory: policy=%d evaluation=%d outcome=%d",
			len(inventory.Policy), len(inventory.Evaluation), len(inventory.Outcome))
	}
	profile, fixtures := lifecycleFixtures()
	evaluation := fixtures["evaluation"]
	raw := lifecycleReceiptRawFixture(t, evaluation.Receipt)
	record := LifecycleRawRecord{ID: evaluation.ID, Issuer: evaluation.Issuer, RawInstructions: raw}
	if _, err := VerifyInstalledLifecycleRecord(context.Background(), InstalledEngineAuthority{}, record, nil); err == nil {
		t.Fatal("zero-value caller authority admitted an inert fixture")
	}
	custody, err := DecodeLifecycleCustodyRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	if custody.accepted {
		t.Fatal("strict raw decoding became installed acceptance")
	}
	if _, ok := InstalledLifecycleRawRecord("evaluation"); ok {
		t.Fatal("uninstalled fixture ID was returned from the embedded production inventory")
	}
	// Exercise the closed pin decoder with a synthetic parser-only policy pin.
	// This value is never written into the embedded release inventory and is not
	// empirical evidence or a qualification claim.
	policyFixture := fixtures["policy"]
	policyRaw, err := json.Marshal(policyFixture.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	pin := lifecycleAcceptancePin{ReceiptID: policyFixture.ID, ReceiptSHA: lifecycleRawSHA(policyRaw),
		Kind: "policy", Repo: policyFixture.Receipt.Repo, HeadSHA: policyFixture.Receipt.HeadSHA,
		ProfileID: policyFixture.Receipt.Profile.ID(), PolicySHA: policyFixture.Receipt.Profile.PolicySHA,
		IssuerClaim: policyFixture.Issuer, ReceiptInstructions: base64.StdEncoding.EncodeToString(policyRaw)}
	synthetic := lifecycleAcceptanceInventory{Schema: "maat.lifecycle-installed-acceptance.v1", Policy: []lifecycleAcceptancePin{pin},
		Evaluation: []lifecycleAcceptancePin{}, Outcome: []lifecycleAcceptancePin{}}
	if err := synthetic.validate(); err != nil {
		t.Fatalf("valid synthetic parser operand rejected: %v", err)
	}
	tampered := synthetic
	tampered.Policy = append([]lifecycleAcceptancePin(nil), synthetic.Policy...)
	tampered.Policy[0].ReceiptInstructions = base64.StdEncoding.EncodeToString(append(append([]byte(nil), policyRaw...), ' '))
	if err := tampered.validate(); err == nil {
		t.Fatal("changed raw bytes remained bound by an unchanged installed pin digest")
	}
	state, err := ReplayQualification(context.Background(), profile, []QualificationEvent{lifecycleEvent(custody)}, lifecycleFixtureResolver{"evaluation": custody})
	if err != nil || state.Status != "unqualified" || state.Verified != 0 {
		t.Fatalf("fixture profile became qualified: state=%+v err=%v", state, err)
	}
}

func TestEmptyLifecycleInventoryDoesNotGateDeterministicHostScreen(t *testing.T) {
	if _, err := readLifecycleAcceptanceInventory(); err != nil {
		t.Fatal(err)
	}
	input := validSystemOneInput()
	verdict, err := Screen(input)
	if err != nil {
		t.Fatalf("deterministic local host screen depended on lifecycle calibration: %v", err)
	}
	if verdict.Gate != GatePass || verdict.Model.Provider != "local:deterministic" || !verdict.Model.Local {
		t.Fatalf("deterministic host baseline changed: %+v", verdict)
	}
}

type installedLifecycleFixtureResolver struct {
	authority InstalledEngineAuthority
	inventory lifecycleAcceptanceInventory
	records   map[string]LifecycleRawRecord
}

func (r installedLifecycleFixtureResolver) ResolveLifecycleRaw(_ context.Context, id string) (LifecycleRawRecord, error) {
	record, ok := r.records[id]
	if !ok {
		return LifecycleRawRecord{}, fmt.Errorf("unknown inert installed raw receipt %q", id)
	}
	return record, nil
}

func (r installedLifecycleFixtureResolver) ResolveLifecycle(ctx context.Context, id string) (LifecycleEvidence, error) {
	record, ok := r.records[id]
	if !ok {
		return LifecycleEvidence{}, fmt.Errorf("unknown inert installed receipt %q", id)
	}
	return verifyInstalledLifecycleRecordWithInventory(ctx, r.authority, record, r, r.inventory)
}

func lifecycleReceiptRawFixture(t *testing.T, receipt LifecycleReceipt) []byte {
	t.Helper()
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range receipt.Samples {
		needle := []byte(`"head_sha":"` + sample.Screen.Subject.HeadSHA + `"}`)
		replacement := []byte(`"head_sha":"` + sample.Screen.Subject.HeadSHA + `","boundary":"","host_binding":""}`)
		raw = bytes.ReplaceAll(raw, needle, replacement)
	}
	return raw
}

func lifecyclePinFixture(t *testing.T, evidence LifecycleEvidence, originID, originSHA string) lifecycleAcceptancePin {
	t.Helper()
	raw := lifecycleReceiptRawFixture(t, evidence.Receipt)
	return lifecycleAcceptancePin{ReceiptID: evidence.ID, ReceiptSHA: lifecycleRawSHA(raw), Kind: evidence.Receipt.Kind,
		Repo: evidence.Receipt.Repo, HeadSHA: evidence.Receipt.HeadSHA, ProfileID: evidence.Receipt.Profile.ID(),
		PolicySHA: evidence.Receipt.Profile.PolicySHA, DatasetSHA: evidence.Receipt.DatasetSHA, ResultsSHA: evidence.Receipt.ResultsSHA,
		ScreenSHA: evidence.Receipt.ScreenSHA, OriginReceiptID: originID, OriginReceiptSHA: originSHA,
		IssuerClaim: evidence.Issuer, ReceiptInstructions: base64.StdEncoding.EncodeToString(raw)}
}

func TestInstalledVerifierAcceptsSelectedSampleHeadAndRejectsOriginDrift(t *testing.T) {
	profile, fixtures := lifecycleFixtures()
	policy := fixtures["policy"]
	evaluation := fixtures["evaluation"]  // top-level run head is a*40
	outcome := fixtures["changes-review"] // selected sample/review head is b*40
	if evaluation.Receipt.HeadSHA == outcome.Receipt.HeadSHA || outcome.Receipt.HeadSHA != strings.Repeat("b", 40) {
		t.Fatalf("fixture no longer models an evaluation spanning heads A and B: evaluation=%s outcome=%s", evaluation.Receipt.HeadSHA, outcome.Receipt.HeadSHA)
	}
	policyRaw := lifecycleReceiptRawFixture(t, policy.Receipt)
	evaluationRaw := lifecycleReceiptRawFixture(t, evaluation.Receipt)
	outcomeRaw := lifecycleReceiptRawFixture(t, outcome.Receipt)
	passOutcome := fixtures["pass-review"]
	passOutcomeRaw := lifecycleReceiptRawFixture(t, passOutcome.Receipt)
	evaluationSHA := lifecycleRawSHA(evaluationRaw)
	inventory := lifecycleAcceptanceInventory{Schema: "maat.lifecycle-installed-acceptance.v1",
		Policy:     []lifecycleAcceptancePin{lifecyclePinFixture(t, policy, "", "")},
		Evaluation: []lifecycleAcceptancePin{lifecyclePinFixture(t, evaluation, "", "")},
		Outcome: []lifecycleAcceptancePin{lifecyclePinFixture(t, passOutcome, evaluation.ID, evaluationSHA),
			lifecyclePinFixture(t, outcome, evaluation.ID, evaluationSHA)}}
	if err := inventory.validate(); err != nil {
		t.Fatalf("valid selected B sample in evaluation A was rejected by installed pin validation: %v", err)
	}
	authority := InstalledEngineAuthority{enginePath: "inert-internal-verifier-test-seam"}
	resolver := installedLifecycleFixtureResolver{authority: authority, inventory: inventory, records: map[string]LifecycleRawRecord{
		policy.ID:      {ID: policy.ID, Issuer: policy.Issuer, RawInstructions: policyRaw},
		evaluation.ID:  {ID: evaluation.ID, Issuer: evaluation.Issuer, RawInstructions: evaluationRaw},
		passOutcome.ID: {ID: passOutcome.ID, Issuer: passOutcome.Issuer, RawInstructions: passOutcomeRaw},
		outcome.ID:     {ID: outcome.ID, Issuer: outcome.Issuer, RawInstructions: outcomeRaw},
	}}
	verified, err := verifyInstalledLifecycleRecordWithInventory(context.Background(), authority,
		LifecycleRawRecord{ID: outcome.ID, Issuer: outcome.Issuer, RawInstructions: outcomeRaw}, resolver, inventory)
	if err != nil || !verified.accepted || verified.Receipt.HeadSHA != strings.Repeat("b", 40) {
		t.Fatalf("installed verifier rejected exact B outcome selected by A evaluation: accepted=%v evidence=%+v err=%v", verified.accepted, verified, err)
	}
	evaluationEvidence, err := verifyInstalledLifecycleRecordWithInventory(context.Background(), authority,
		LifecycleRawRecord{ID: evaluation.ID, Issuer: evaluation.Issuer, RawInstructions: evaluationRaw}, resolver, inventory)
	if err != nil || !evaluationEvidence.accepted {
		t.Fatalf("installed verifier rejected exact evaluation A: accepted=%v err=%v", evaluationEvidence.accepted, err)
	}
	state, err := ReplayQualification(context.Background(), profile, []QualificationEvent{lifecycleEvent(evaluationEvidence)}, resolver)
	if err != nil || state.Status != "qualified" || state.PassSamples != 1 || state.ChangesSamples != 1 {
		t.Fatalf("full installed verifier/replay did not resolve selected sample B in evaluation A: state=%+v err=%v", state, err)
	}

	wrongHead := outcome.Receipt
	wrongHead.HeadSHA = strings.Repeat("c", 40)
	wrongHeadRaw := lifecycleReceiptRawFixture(t, wrongHead)
	wrongHeadEvidence, err := lifecycleEvidenceFromRaw(LifecycleRawRecord{ID: outcome.ID, Issuer: outcome.Issuer, RawInstructions: wrongHeadRaw})
	if err != nil {
		t.Fatal(err)
	}
	wrongHeadInventory := inventory
	wrongHeadInventory.Outcome = []lifecycleAcceptancePin{lifecyclePinFixture(t, wrongHeadEvidence, evaluation.ID, evaluationSHA)}
	if err := wrongHeadInventory.validate(); err == nil {
		t.Fatal("installed verifier inventory accepted an outcome head different from its selected sample")
	}

	wrongScreen := outcome.Receipt
	wrongScreen.ScreenSHA = strings.Repeat("d", 64)
	wrongScreenRaw := lifecycleReceiptRawFixture(t, wrongScreen)
	wrongScreenEvidence, err := lifecycleEvidenceFromRaw(LifecycleRawRecord{ID: outcome.ID, Issuer: outcome.Issuer, RawInstructions: wrongScreenRaw})
	if err != nil {
		t.Fatal(err)
	}
	wrongScreenInventory := inventory
	wrongScreenInventory.Outcome = []lifecycleAcceptancePin{lifecyclePinFixture(t, wrongScreenEvidence, evaluation.ID, evaluationSHA)}
	if err := wrongScreenInventory.validate(); err == nil {
		t.Fatal("installed verifier inventory accepted a screen not present in the selected sample")
	}

	missingSelection := evaluation.Receipt
	missingSelection.SelectedIDs = []string{"pass"}
	missingSelectionRaw := lifecycleReceiptRawFixture(t, missingSelection)
	missingSelectionEvidence, err := lifecycleEvidenceFromRaw(LifecycleRawRecord{ID: evaluation.ID, Issuer: evaluation.Issuer, RawInstructions: missingSelectionRaw})
	if err != nil {
		t.Fatal(err)
	}
	missingSelectionSHA := lifecycleRawSHA(missingSelectionRaw)
	missingSelectionInventory := inventory
	missingSelectionInventory.Evaluation = []lifecycleAcceptancePin{lifecyclePinFixture(t, missingSelectionEvidence, "", "")}
	missingSelectionInventory.Outcome = []lifecycleAcceptancePin{lifecyclePinFixture(t, outcome, evaluation.ID, missingSelectionSHA)}
	if err := missingSelectionInventory.validate(); err == nil {
		t.Fatal("installed verifier inventory accepted an outcome absent from selected sample IDs")
	}

	changedOrigin := append(append([]byte(nil), evaluationRaw...), ' ')
	resolver.records[evaluation.ID] = LifecycleRawRecord{ID: evaluation.ID, Issuer: evaluation.Issuer, RawInstructions: changedOrigin}
	refused, err := verifyInstalledLifecycleRecordWithInventory(context.Background(), authority,
		LifecycleRawRecord{ID: outcome.ID, Issuer: outcome.Issuer, RawInstructions: outcomeRaw}, resolver, inventory)
	if err != nil || refused.accepted || !strings.Contains(refused.Reason, "not in the installed accepted-evidence inventory") {
		t.Fatalf("changed exact origin bytes retained installed acceptance: accepted=%v reason=%q err=%v", refused.accepted, refused.Reason, err)
	}
	if outcome.Receipt.Profile != profile {
		t.Fatal("multi-head fixture profile unexpectedly drifted")
	}
}
