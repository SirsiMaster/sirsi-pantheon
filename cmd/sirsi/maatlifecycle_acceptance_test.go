package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

// This resolver is an inert, exact-byte stand-in for canonical Get. The test
// calls the real Cobra command and private file journal; it never opens Router,
// credentials, a model, or a service.
type maatLifecycleRawFixture map[string]maat.LifecycleRawRecord

func (r maatLifecycleRawFixture) ResolveLifecycleRaw(_ context.Context, id string) (maat.LifecycleRawRecord, error) {
	value, ok := r[id]
	if !ok {
		return maat.LifecycleRawRecord{}, fmt.Errorf("unknown inert receipt %q", id)
	}
	return value, nil
}

func (r maatLifecycleRawFixture) ResolveLifecycle(ctx context.Context, id string) (maat.LifecycleEvidence, error) {
	value, err := r.ResolveLifecycleRaw(ctx, id)
	if err != nil {
		return maat.LifecycleEvidence{}, err
	}
	return maat.DecodeLifecycleCustodyRecord(value)
}

func maatLifecycleTestDigest(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func maatLifecycleTestReceiptBytes(t *testing.T, receipt maat.LifecycleReceipt) []byte {
	t.Helper()
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Kind != "evaluation" {
		return raw
	}
	// The strict source receipt decoder requires explicit empty boundary and
	// host_binding members even though encoding/json omits them due to
	// `omitempty` for commit subjects.
	// A selected sample can legitimately have a different subject head from the
	// evaluation's top-level run head. Add the required wire member to each
	// serialized sample subject, not to a guessed occurrence of the run head.
	subjectHeads := make(map[string]int, len(receipt.Samples))
	for _, sample := range receipt.Samples {
		subjectHeads[sample.Screen.Subject.HeadSHA]++
	}
	for head, expected := range subjectHeads {
		needle := []byte(`"head_sha":"` + head + `"}`)
		replacement := []byte(`"head_sha":"` + head + `","boundary":"","host_binding":""}`)
		if count := bytes.Count(raw, needle); count != expected {
			t.Fatalf("expected %d exact sample subject(s) with head %s, found %d", expected, head, count)
		}
		raw = bytes.ReplaceAll(raw, needle, replacement)
	}
	return raw
}

func maatLifecycleRawFixtures(t *testing.T) (maatLifecycleRawFixture, maat.SourceProfileIdentity, string, string, string) {
	t.Helper()
	policy := maat.QualificationPolicy{ID: "inert-local-policy", Threshold: .90, BoundaryThreshold: .98, MinimumPass: 1, MinimumChanges: 1}
	profile := maat.SourceProfileIdentity{Producer: "inert-fixture-producer", ArtifactSHA: strings.Repeat("a", 64),
		ModelVersion: "inert-fixture-v1", PromptSHA: strings.Repeat("b", 64), RecipeSHA: strings.Repeat("c", 64), PolicySHA: maatLifecycleTestDigest(policy)}
	repo, head := "fixture/local-only", strings.Repeat("a", 40)
	policyReceipt := maat.LifecycleReceipt{Schema: maat.LifecycleSchema, Kind: "policy", Profile: profile, Repo: repo, HeadSHA: head,
		Policy: policy, RunID: "inert-policy-run", Samples: []maat.LifecycleSample{}, SelectedIDs: []string{}}
	policyRaw := maatLifecycleTestReceiptBytes(t, policyReceipt)

	screen := maat.SourceScreen{Contract: maat.SourceScreenContract, SchemaVersion: maat.SourceScreenSchemaVersion,
		Subject: maat.VerdictSubject{Kind: "commit", Repo: repo, Ref: "main", HeadSHA: head}, FeatherWeight: 92, Confidence: .96,
		Findings: []maat.ScreenFinding{}, Floor: maat.FloorResult{Passed: true, Checks: []maat.FloorCheck{
			{Name: "gofmt", Passed: true}, {Name: "vet", Passed: true}, {Name: "golangci-lint", Passed: true}, {Name: "test-race", Passed: true},
		}}, Model: maat.ModelStamp{Provider: "local:deterministic", Version: profile.ModelVersion, Local: true,
			EvidenceSetDigest: strings.Repeat("c", 64), CalibrationID: strings.Repeat("d", 64), Calibration: "unqualified"}}
	sample := maat.LifecycleSample{ID: "sample-pass", Screen: screen, OutcomeRef: "review-pass"}
	outcome := maat.LifecycleReceipt{Schema: maat.LifecycleSchema, Kind: "outcome", Profile: profile, Repo: repo, HeadSHA: head,
		Policy: policy, RunID: "inert-review-run", Samples: []maat.LifecycleSample{}, SelectedIDs: []string{},
		ScreenSHA: maatLifecycleTestDigest(screen), CandidateGate: maat.GatePass, FinalGate: maat.GatePass}
	outcomeRaw := maatLifecycleTestReceiptBytes(t, outcome)
	evaluation := maat.LifecycleReceipt{Schema: maat.LifecycleSchema, Kind: "evaluation", Profile: profile, Repo: repo, HeadSHA: head,
		PolicyRef: "policy", Policy: policy, RunID: "inert-evaluation-run", Samples: []maat.LifecycleSample{sample}, SelectedIDs: []string{"sample-pass"}}
	evaluation.ResultsSHA = maatLifecycleTestDigest(evaluation.Samples)
	evaluation.DatasetSHA = maatLifecycleTestDigest([]string{"sample-pass:" + repo + ":" + head})
	evaluationRaw := maatLifecycleTestReceiptBytes(t, evaluation)

	return maatLifecycleRawFixture{
		"policy":      {ID: "policy", Issuer: profile.Producer, RawInstructions: policyRaw},
		"evaluation":  {ID: "evaluation", Issuer: profile.Producer, RawInstructions: evaluationRaw},
		"review-pass": {ID: "review-pass", Issuer: "claimed-independent-reviewer", RawInstructions: outcomeRaw},
	}, profile, repo, head, "review-pass"
}

func maatLifecycleMultiHeadRawFixtures(t *testing.T) (maatLifecycleRawFixture, string, string, string) {
	t.Helper()
	resolver, _, repo, evaluationHead, reviewID := maatLifecycleRawFixtures(t)
	sampleHead := strings.Repeat("b", 40)
	evaluationRecord := resolver["evaluation"]
	var evaluation maat.LifecycleReceipt
	if err := json.Unmarshal(evaluationRecord.RawInstructions, &evaluation); err != nil {
		t.Fatal(err)
	}
	if evaluation.HeadSHA != evaluationHead || len(evaluation.Samples) != 1 {
		t.Fatalf("multi-head source fixture changed unexpectedly: %+v", evaluation)
	}
	evaluation.Samples[0].Screen.Subject.HeadSHA = sampleHead
	evaluation.Samples[0].Screen.Findings = []maat.ScreenFinding{{ID: "minor", Severity: "minor", Category: "correctness",
		File: "fixture.go", Line: 1, Claim: "inert selected B-head change", Evidence: "fixture-only evidence",
		EvidenceID: strings.Repeat("d", 64), Confidence: 1}}
	evaluation.ResultsSHA = maatLifecycleTestDigest(evaluation.Samples)
	evaluation.DatasetSHA = maatLifecycleTestDigest([]string{evaluation.Samples[0].ID + ":" + repo + ":" + sampleHead})
	evaluationRecord.RawInstructions = maatLifecycleTestReceiptBytes(t, evaluation)
	resolver["evaluation"] = evaluationRecord

	reviewRecord := resolver[reviewID]
	var review maat.LifecycleReceipt
	if err := json.Unmarshal(reviewRecord.RawInstructions, &review); err != nil {
		t.Fatal(err)
	}
	review.HeadSHA = sampleHead
	review.CandidateGate = maat.GateChanges
	review.FinalGate = maat.GateChanges
	review.ScreenSHA = maatLifecycleTestDigest(evaluation.Samples[0].Screen)
	reviewRecord.RawInstructions = maatLifecycleTestReceiptBytes(t, review)
	resolver[reviewID] = reviewRecord
	return resolver, repo, evaluationHead, sampleHead
}

func runMaatLifecycleActualCommand(t *testing.T, journalPath string, resolver maat.LifecycleResolver, args ...string) ([]byte, error) {
	t.Helper()
	command := newMaatLifecycleCmdWith(
		func() (lifecycleJournal, error) { return &maat.FileDecisionJournal{Path: journalPath}, nil },
		func() (maat.LifecycleResolver, func(), error) { return resolver, func() {}, nil },
	)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs(args)
	err := command.Execute()
	return output.Bytes(), err
}

func adoptionArgs(repo, head string, confirm bool, reviewSHA, originSHA, adoptionSHA string) []string {
	args := []string{"--receipt-id", "review-pass", "--origin-receipt-id", "evaluation", "--expect-repo", repo,
		"--expect-head", head, "--owner-adopt-review", "--json"}
	if confirm {
		args = append(args, "--expect-review-sha256", reviewSHA, "--expect-origin-sha256", originSHA,
			"--expect-adoption-sha256", adoptionSHA, "--confirm")
	} else {
		args = append(args, "--preview")
	}
	return args
}

func TestMaatLifecycleActualCLIPrivateJournalOwnerAdoptionAndOfflineReplay(t *testing.T) {
	resolver, _, repo, head, reviewID := maatLifecycleRawFixtures(t)
	journalPath := filepath.Join(t.TempDir(), "private", "maat-decisions.jsonl")

	previewBytes, err := runMaatLifecycleActualCommand(t, journalPath, resolver, adoptionArgs(repo, head, false, "", "", "")...)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(journalPath); !os.IsNotExist(err) {
		t.Fatalf("read-only CLI preview created journal: %v", err)
	}
	var preview maat.LifecycleAdoptionObservation
	if err := json.Unmarshal(previewBytes, &preview); err != nil {
		t.Fatalf("decode actual CLI preview: %v; output=%s", err, previewBytes)
	}
	if preview.Recorded || preview.Duplicate || preview.Adoption.ReviewID != reviewID || preview.Adoption.ReviewerClaim != "claimed-independent-reviewer" ||
		!strings.Contains(preview.Provenance, "not authenticated reviewer identity") {
		t.Fatalf("preview promoted sender claim or wrote: %+v", preview)
	}

	confirmArgs := adoptionArgs(repo, head, true, preview.Adoption.ReviewSHA, preview.Adoption.OriginSHA, preview.AdoptionSHA)
	wrongDigestArgs := append([]string(nil), confirmArgs...)
	for i := range wrongDigestArgs {
		if wrongDigestArgs[i] == preview.AdoptionSHA {
			wrongDigestArgs[i] = strings.Repeat("0", 64)
			break
		}
	}
	if _, err := runMaatLifecycleActualCommand(t, journalPath, resolver, wrongDigestArgs...); err == nil {
		t.Fatal("CLI accepted a changed preview/adoption digest")
	}
	if _, err := os.Stat(journalPath); !os.IsNotExist(err) {
		t.Fatalf("refused confirmation mutated private journal: %v", err)
	}

	confirmedBytes, err := runMaatLifecycleActualCommand(t, journalPath, resolver, confirmArgs...)
	if err != nil {
		t.Fatal(err)
	}
	var confirmed maat.LifecycleAdoptionObservation
	if err := json.Unmarshal(confirmedBytes, &confirmed); err != nil || !confirmed.Recorded || confirmed.Duplicate || confirmed.AdoptionSHA != preview.AdoptionSHA {
		t.Fatalf("actual CLI/private-journal adoption=%+v decodeErr=%v output=%s", confirmed, err, confirmedBytes)
	}
	retainedBeforeDuplicate, err := os.ReadFile(journalPath)
	if err != nil || len(retainedBeforeDuplicate) == 0 || retainedBeforeDuplicate[len(retainedBeforeDuplicate)-1] != '\n' {
		t.Fatalf("journal append was not one complete private record: %v", err)
	}
	duplicateBytes, err := runMaatLifecycleActualCommand(t, journalPath, resolver, confirmArgs...)
	if err != nil {
		t.Fatal(err)
	}
	var duplicate maat.LifecycleAdoptionObservation
	if err := json.Unmarshal(duplicateBytes, &duplicate); err != nil || duplicate.Recorded || !duplicate.Duplicate {
		t.Fatalf("exact repeat was not idempotent: %+v err=%v", duplicate, err)
	}
	retainedAfterDuplicate, err := os.ReadFile(journalPath)
	if err != nil || !bytes.Equal(retainedBeforeDuplicate, retainedAfterDuplicate) {
		t.Fatalf("duplicate confirmation changed journal bytes: %v", err)
	}

	// Re-open through a fresh journal instance via the actual CLI. Exact local
	// adoption is replayable offline but cannot qualify an unpinned profile.
	observation, err := runMaatLifecycleActualCommand(t, journalPath, resolver, "--receipt-id", "evaluation", "--expect-repo", repo,
		"--expect-head", head, "--preview", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var state maat.LifecycleObservation
	if err := json.Unmarshal(observation, &state); err != nil || state.State.Status != "unqualified" || state.State.Verified != 0 || state.Recorded ||
		!strings.Contains(state.Acceptance, "unknown") {
		t.Fatalf("owner adoption or inert receipts qualified profile: %+v err=%v output=%s", state, err, observation)
	}
}

func TestMaatLifecycleActualCLIAdoptsSelectedSampleHeadDifferentFromEvaluationHead(t *testing.T) {
	resolver, repo, evaluationHead, sampleHead := maatLifecycleMultiHeadRawFixtures(t)
	journalPath := filepath.Join(t.TempDir(), "private", "maat-decisions.jsonl")
	previewBytes, err := runMaatLifecycleActualCommand(t, journalPath, resolver,
		adoptionArgs(repo, sampleHead, false, "", "", "")...)
	if err != nil {
		t.Fatalf("valid selected sample B in evaluation A failed preview: %v", err)
	}
	var preview maat.LifecycleAdoptionObservation
	if err := json.Unmarshal(previewBytes, &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Adoption.HeadSHA != sampleHead || preview.Adoption.OriginID != "evaluation" || preview.Adoption.CandidateGate != maat.GateChanges {
		t.Fatalf("preview lost evaluation/sample head distinction: %+v", preview.Adoption)
	}
	args := adoptionArgs(repo, sampleHead, true, preview.Adoption.ReviewSHA, preview.Adoption.OriginSHA, preview.AdoptionSHA)
	confirmed, err := runMaatLifecycleActualCommand(t, journalPath, resolver, args...)
	if err != nil {
		t.Fatalf("valid selected sample B in evaluation A failed confirmation/journal validation: %v", err)
	}
	var adopted maat.LifecycleAdoptionObservation
	if err := json.Unmarshal(confirmed, &adopted); err != nil || !adopted.Recorded {
		t.Fatalf("cross-head adoption was not durably recorded: adoption=%+v err=%v", adopted, err)
	}
	readback, err := runMaatLifecycleActualCommand(t, journalPath, resolver, "--receipt-id", "review-pass", "--expect-repo", repo,
		"--expect-head", sampleHead, "--preview", "--json")
	if err != nil {
		t.Fatalf("offline journal reopen did not resolve exact selected sample B adoption: %v", err)
	}
	var observed maat.LifecycleObservation
	if err := json.Unmarshal(readback, &observed); err != nil || observed.State.Status != "unqualified" ||
		observed.State.Verified != 1 || !strings.Contains(observed.Acceptance, "informational") {
		t.Fatalf("owner adoption improperly qualified a model or failed exact replay: observation=%+v err=%v", observed, err)
	}
	if evaluationHead == sampleHead {
		t.Fatal("cross-head positive did not distinguish evaluation and sample identity")
	}
}

func TestMaatLifecycleActualCLIRejectsChangedCustodyAndSelfReview(t *testing.T) {
	for _, defect := range []string{"changed-review-bytes", "changed-origin-bytes", "producer-as-reviewer", "unreferenced-outcome"} {
		t.Run(defect, func(t *testing.T) {
			resolver, profile, repo, head, _ := maatLifecycleRawFixtures(t)
			journalPath := filepath.Join(t.TempDir(), "private", "maat-decisions.jsonl")
			previewBytes, err := runMaatLifecycleActualCommand(t, journalPath, resolver, adoptionArgs(repo, head, false, "", "", "")...)
			if err != nil {
				t.Fatal(err)
			}
			var preview maat.LifecycleAdoptionObservation
			if err := json.Unmarshal(previewBytes, &preview); err != nil {
				t.Fatal(err)
			}
			switch defect {
			case "changed-review-bytes":
				record := resolver["review-pass"]
				record.RawInstructions = append(record.RawInstructions, ' ')
				resolver["review-pass"] = record
				args := adoptionArgs(repo, head, true, preview.Adoption.ReviewSHA, preview.Adoption.OriginSHA, preview.AdoptionSHA)
				if _, err := runMaatLifecycleActualCommand(t, journalPath, resolver, args...); err == nil || !strings.Contains(err.Error(), "changed") {
					t.Fatalf("CLI did not reject exact-byte drift: %v", err)
				}
			case "changed-origin-bytes":
				record := resolver["evaluation"]
				record.RawInstructions = append(record.RawInstructions, ' ')
				resolver["evaluation"] = record
				args := adoptionArgs(repo, head, true, preview.Adoption.ReviewSHA, preview.Adoption.OriginSHA, preview.AdoptionSHA)
				if _, err := runMaatLifecycleActualCommand(t, journalPath, resolver, args...); err == nil || !strings.Contains(err.Error(), "changed") {
					t.Fatalf("CLI did not reject originating-evaluation byte drift: %v", err)
				}
			case "producer-as-reviewer":
				record := resolver["review-pass"]
				record.Issuer = profile.Producer
				resolver["review-pass"] = record
				if _, err := runMaatLifecycleActualCommand(t, journalPath, resolver, adoptionArgs(repo, head, false, "", "", "")...); err == nil || !strings.Contains(err.Error(), "non-producer") {
					t.Fatalf("CLI accepted producer's own claimed review: %v", err)
				}
			case "unreferenced-outcome":
				record := resolver["evaluation"]
				var evaluation maat.LifecycleReceipt
				if err := json.Unmarshal(record.RawInstructions, &evaluation); err != nil {
					t.Fatal(err)
				}
				evaluation.Samples[0].OutcomeRef = "other-review"
				evaluation.ResultsSHA = maatLifecycleTestDigest(evaluation.Samples)
				record.RawInstructions = maatLifecycleTestReceiptBytes(t, evaluation)
				resolver["evaluation"] = record
				if _, err := runMaatLifecycleActualCommand(t, journalPath, resolver, adoptionArgs(repo, head, false, "", "", "")...); err == nil || !strings.Contains(err.Error(), "exact outcome reference") {
					t.Fatalf("CLI accepted unreferenced review outcome: %v", err)
				}
			}
			if _, err := os.Stat(journalPath); !os.IsNotExist(err) {
				t.Fatalf("refused CLI adoption changed journal: %v", err)
			}
		})
	}
}

func TestMaatLifecycleActualCLIRequiresExactOriginAndRejectsConflictingAdoption(t *testing.T) {
	resolver, _, repo, head, _ := maatLifecycleRawFixtures(t)
	journalPath := filepath.Join(t.TempDir(), "private", "maat-decisions.jsonl")
	missingOrigin := adoptionArgs(repo, head, false, "", "", "")
	for i := 0; i < len(missingOrigin); i++ {
		if missingOrigin[i] == "--origin-receipt-id" {
			missingOrigin = append(missingOrigin[:i], missingOrigin[i+2:]...)
			break
		}
	}
	if _, err := runMaatLifecycleActualCommand(t, journalPath, resolver, missingOrigin...); err == nil || !strings.Contains(err.Error(), "--origin-receipt-id") {
		t.Fatalf("actual CLI accepted missing origin ID: %v", err)
	}
	unknownOrigin := adoptionArgs(repo, head, false, "", "", "")
	for i := 0; i < len(unknownOrigin); i++ {
		if unknownOrigin[i] == "evaluation" {
			unknownOrigin[i] = "not-an-existing-evaluation"
		}
	}
	if _, err := runMaatLifecycleActualCommand(t, journalPath, resolver, unknownOrigin...); err == nil {
		t.Fatal("actual CLI accepted an unknown origin receipt")
	}

	previewBytes, err := runMaatLifecycleActualCommand(t, journalPath, resolver, adoptionArgs(repo, head, false, "", "", "")...)
	if err != nil {
		t.Fatal(err)
	}
	var preview maat.LifecycleAdoptionObservation
	if err := json.Unmarshal(previewBytes, &preview); err != nil {
		t.Fatal(err)
	}
	confirm := adoptionArgs(repo, head, true, preview.Adoption.ReviewSHA, preview.Adoption.OriginSHA, preview.AdoptionSHA)
	if _, err := runMaatLifecycleActualCommand(t, journalPath, resolver, confirm...); err != nil {
		t.Fatal(err)
	}

	// The same exact review may not be rebound to a different originating
	// evaluation after an adoption already exists.
	origin := resolver["evaluation"]
	var second maat.LifecycleReceipt
	if err := json.Unmarshal(origin.RawInstructions, &second); err != nil {
		t.Fatal(err)
	}
	second.RunID = "second-inert-evaluation-run"
	origin.RawInstructions = maatLifecycleTestReceiptBytes(t, second)
	origin.ID = "evaluation-two"
	resolver[origin.ID] = origin
	conflicting := adoptionArgs(repo, head, false, "", "", "")
	for i := 0; i < len(conflicting); i++ {
		if conflicting[i] == "evaluation" {
			conflicting[i] = "evaluation-two"
		}
	}
	if _, err := runMaatLifecycleActualCommand(t, journalPath, resolver, conflicting...); err == nil || !strings.Contains(err.Error(), "conflicting owner adoption") {
		t.Fatalf("actual private journal allowed a conflicting adoption for the same review: %v", err)
	}
}

func TestMaatLifecycleActualCLIAdoptionConfirmPreservesUnterminatedTail(t *testing.T) {
	resolver, _, repo, head, _ := maatLifecycleRawFixtures(t)
	journalPath := filepath.Join(t.TempDir(), "private", "maat-decisions.jsonl")
	previewBytes, err := runMaatLifecycleActualCommand(t, journalPath, resolver, adoptionArgs(repo, head, false, "", "", "")...)
	if err != nil {
		t.Fatal(err)
	}
	var preview maat.LifecycleAdoptionObservation
	if err := json.Unmarshal(previewBytes, &preview); err != nil {
		t.Fatal(err)
	}
	torn := []byte(`{"existing":"unterminated owner adoption`)
	if err := os.MkdirAll(filepath.Dir(journalPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(journalPath, torn, 0o600); err != nil {
		t.Fatal(err)
	}
	args := adoptionArgs(repo, head, true, preview.Adoption.ReviewSHA, preview.Adoption.OriginSHA, preview.AdoptionSHA)
	if _, err := runMaatLifecycleActualCommand(t, journalPath, resolver, args...); err == nil ||
		!strings.Contains(err.Error(), "unterminated final journal record") || !strings.Contains(err.Error(), "Preserve the existing bytes") {
		t.Fatalf("actual CLI/private journal did not report actionable torn-tail refusal: %v", err)
	}
	after, err := os.ReadFile(journalPath)
	if err != nil || !bytes.Equal(after, torn) {
		t.Fatalf("torn-tail refusal changed original bytes: %v", err)
	}
}
