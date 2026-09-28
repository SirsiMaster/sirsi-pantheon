package maat

import (
	"strings"
	"testing"
)

func validSystemOneInput() SystemOneScreen {
	return SystemOneScreen{
		Subject:       VerdictSubject{Kind: "commit", Repo: "SirsiMaster/sirsi-pantheon", Ref: "main", HeadSHA: strings.Repeat("a", 40)},
		FeatherWeight: 92,
		Confidence:    0.96,
		Floor: FloorResult{Passed: true, Checks: []FloorCheck{
			{Name: "gofmt", Passed: true}, {Name: "go vet", Passed: true},
		}},
		Model: ModelStamp{Provider: "local:deterministic", Version: "v1", Local: true, LatencyMS: 2},
	}
}

func TestSystemOneScreenPassesOnlyACompleteHighConfidenceFloor(t *testing.T) {
	verdict, err := Screen(validSystemOneInput())
	if err != nil {
		t.Fatal(err)
	}
	if verdict.Gate != GatePass || verdict.Escalation != nil {
		t.Fatalf("verdict = %+v, want local pass without escalation", verdict)
	}
}

func TestSystemOneFloorFailureCannotBeOverridden(t *testing.T) {
	input := validSystemOneInput()
	input.Floor = FloorResult{Passed: false, Checks: []FloorCheck{{Name: "go test -race", Passed: false, Detail: "race detected"}}}
	verdict, err := Screen(input)
	if err != nil {
		t.Fatal(err)
	}
	if verdict.Gate != GateBlock || verdict.FeatherWeight > 49 || verdict.Escalation != nil {
		t.Fatalf("floor-red verdict = %+v", verdict)
	}
	verdict.Gate = GatePass
	if err := ValidateMaatVerdict(verdict); err == nil {
		t.Fatal("accepted forged pass over deterministic floor failure")
	}
}

func TestSystemOneSensitiveBoundaryAlwaysEscalates(t *testing.T) {
	input := validSystemOneInput()
	input.Subject.Boundary = "delivery"
	verdict, err := Screen(input)
	if err != nil {
		t.Fatal(err)
	}
	if verdict.Gate != GateEscalate || verdict.Escalation == nil || verdict.Escalation.ReviewTier != "frontier" {
		t.Fatalf("delivery verdict = %+v", verdict)
	}
}

func TestSystemOneLowConfidenceAndBlockingFindingsRouteCorrectly(t *testing.T) {
	input := validSystemOneInput()
	input.Confidence = 0.89
	verdict, err := Screen(input)
	if err != nil {
		t.Fatal(err)
	}
	if verdict.Gate != GateEscalate || verdict.Escalation == nil {
		t.Fatalf("low-confidence verdict = %+v", verdict)
	}

	input = validSystemOneInput()
	input.Findings = []ScreenFinding{{
		ID: "identity-mismatch", Severity: "block", Category: "identity", File: "Info.plist", Line: 1,
		Claim: "bundle identifier does not match release contract", Evidence: "receipt:identity", Confidence: 0.97,
	}}
	verdict, err = Screen(input)
	if err != nil {
		t.Fatal(err)
	}
	if verdict.Gate != GateBlock {
		t.Fatalf("high-confidence blocking finding = %+v", verdict)
	}
}

func TestSystemOneRejectsMalformedInputAndForgedEscalation(t *testing.T) {
	input := validSystemOneInput()
	input.Subject.Boundary = "unknown"
	if _, err := Screen(input); err == nil {
		t.Fatal("accepted unknown boundary")
	}
	input = validSystemOneInput()
	input.Floor = FloorResult{Passed: true, Checks: []FloorCheck{{Name: "gofmt", Passed: false}}}
	if _, err := Screen(input); err == nil {
		t.Fatal("accepted failed floor check with no diagnostic detail")
	}
	verdict, err := Screen(validSystemOneInput())
	if err != nil {
		t.Fatal(err)
	}
	verdict.Escalation = &Escalation{Reason: "forged", ReviewTier: "frontier"}
	if err := ValidateMaatVerdict(verdict); err == nil {
		t.Fatal("accepted escalation not derived from the screen")
	}
}

func TestRecordSystemOneProjectsAnEvidenceBoundDecision(t *testing.T) {
	verdict, err := Screen(validSystemOneInput())
	if err != nil {
		t.Fatal(err)
	}
	journal := &captureJournal{}
	decision, err := RecordSystemOne(journal, "sirsi maat screen", verdict)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Kind != "system one screen" || decision.SystemOne == nil || !strings.HasPrefix(decision.Evidence, "maat-system-one:sha256=") {
		t.Fatalf("decision = %+v", decision)
	}
	if len(journal.decisions) != 1 || journal.decisions[0].SystemOne == nil {
		t.Fatalf("journal = %+v", journal.decisions)
	}
}

func TestSystemOneCalibrationMeasuresAutoPassOverturns(t *testing.T) {
	pass, err := Screen(validSystemOneInput())
	if err != nil {
		t.Fatal(err)
	}
	calibration, err := Calibrate([]CalibrationSample{
		{Screen: pass, Frontier: GatePass},
		{Screen: pass, Frontier: GateBlock},
	})
	if err != nil {
		t.Fatal(err)
	}
	if calibration.Samples != 2 || calibration.AutoPasses != 2 || calibration.AutoPassOverturn != 1 || calibration.OverturnRate != 0.5 {
		t.Fatalf("calibration = %+v", calibration)
	}
}
