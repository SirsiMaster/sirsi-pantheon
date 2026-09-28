package maat

import (
	"strings"
	"testing"
)

type systemOneJournal struct{ decisions []Decision }

func (j *systemOneJournal) Append(decision Decision) error {
	j.decisions = append(j.decisions, decision)
	return nil
}

func (j *systemOneJournal) Recent(limit int) ([]Decision, error) {
	if limit <= 0 || limit > len(j.decisions) {
		limit = len(j.decisions)
	}
	return append([]Decision(nil), j.decisions[len(j.decisions)-limit:]...), nil
}

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

func TestSystemOneCanonicalizesInputOrderAndRejectsDuplicateFindingIDs(t *testing.T) {
	input := validSystemOneInput()
	input.Floor.Checks = []FloorCheck{{Name: "go vet", Passed: true}, {Name: "gofmt", Passed: true}}
	input.Findings = []ScreenFinding{
		{ID: "zeta", Severity: "minor", Category: "quality", Claim: "zeta", Evidence: "evidence:zeta", Confidence: 0.92},
		{ID: "alpha", Severity: "minor", Category: "quality", Claim: "alpha", Evidence: "evidence:alpha", Confidence: 0.92},
	}
	verdict, err := Screen(input)
	if err != nil {
		t.Fatal(err)
	}
	if verdict.Floor.Checks[0].Name != "go vet" || verdict.Floor.Checks[1].Name != "gofmt" || verdict.Findings[0].ID != "alpha" || verdict.Findings[1].ID != "zeta" {
		t.Fatalf("canonical verdict = %+v", verdict)
	}
	journal := &systemOneJournal{}
	first, err := RecordSystemOne(journal, "sirsi maat screen", verdict)
	if err != nil {
		t.Fatal(err)
	}
	reversed := validSystemOneInput()
	reversed.Floor.Checks = []FloorCheck{{Name: "gofmt", Passed: true}, {Name: "go vet", Passed: true}}
	reversed.Findings = []ScreenFinding{input.Findings[1], input.Findings[0]}
	canonical, err := Screen(reversed)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RecordSystemOne(&systemOneJournal{}, "sirsi maat screen", canonical)
	if err != nil {
		t.Fatal(err)
	}
	if first.Evidence != second.Evidence {
		t.Fatalf("equivalent input evidence differs: %q != %q", first.Evidence, second.Evidence)
	}

	duplicate := validSystemOneInput()
	duplicate.Findings = []ScreenFinding{
		{ID: "same", Severity: "minor", Category: "quality", Claim: "first", Evidence: "evidence:first", Confidence: 0.92},
		{ID: "same", Severity: "minor", Category: "quality", Claim: "second", Evidence: "evidence:second", Confidence: 0.92},
	}
	if _, err := Screen(duplicate); err == nil {
		t.Fatal("accepted duplicate finding identity")
	}
	verdict.Floor.Checks[0], verdict.Floor.Checks[1] = verdict.Floor.Checks[1], verdict.Floor.Checks[0]
	if err := ValidateMaatVerdict(verdict); err == nil {
		t.Fatal("accepted a noncanonical verdict ordering")
	}
}

func TestRecordSystemOneProjectsAnEvidenceBoundDecision(t *testing.T) {
	verdict, err := Screen(validSystemOneInput())
	if err != nil {
		t.Fatal(err)
	}
	journal := &systemOneJournal{}
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

func TestRecordSystemOneCalibrationBindsARecordedAutoPassOnce(t *testing.T) {
	verdict, err := Screen(validSystemOneInput())
	if err != nil {
		t.Fatal(err)
	}
	journal := &systemOneJournal{}
	screen, err := RecordSystemOne(journal, "sirsi maat screen", verdict)
	if err != nil {
		t.Fatal(err)
	}
	decision, calibration, err := RecordSystemOneCalibration(journal, "independent reviewer", screen.Evidence, "review:sha256=frontier", GateBlock)
	if err != nil {
		t.Fatal(err)
	}
	if decision.SystemOneCalibration == nil || calibration.AutoPasses != 1 || calibration.AutoPassOverturn != 1 || calibration.OverturnRate != 1 {
		t.Fatalf("calibration decision=%+v calibration=%+v", decision, calibration)
	}
	if _, _, err := RecordSystemOneCalibration(journal, "independent reviewer", screen.Evidence, "review:sha256=frontier", GateBlock); err == nil {
		t.Fatal("accepted replayed calibration pair")
	}
}

func TestRecordSystemOneCalibrationRejectsMissingAndNonPassScreens(t *testing.T) {
	journal := &systemOneJournal{}
	if _, _, err := RecordSystemOneCalibration(journal, "independent reviewer", "maat-system-one:sha256=missing", "review:sha256=frontier", GatePass); err == nil {
		t.Fatal("accepted calibration without a recorded screen")
	}
	input := validSystemOneInput()
	input.Subject.Boundary = "delivery"
	verdict, err := Screen(input)
	if err != nil {
		t.Fatal(err)
	}
	screen, err := RecordSystemOne(journal, "sirsi maat screen", verdict)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := RecordSystemOneCalibration(journal, "independent reviewer", screen.Evidence, "review:sha256=frontier", GatePass); err == nil {
		t.Fatal("accepted calibration for a non-auto-pass screen")
	}
}

func TestCalibrationFromDecisionsRejectsUnboundAndDuplicatePairs(t *testing.T) {
	verdict, err := Screen(validSystemOneInput())
	if err != nil {
		t.Fatal(err)
	}
	record := CalibrationRecord{SchemaVersion: SystemOneSchemaVersion, ScreenEvidence: "maat-system-one:sha256=screen", FrontierEvidence: "review:sha256=frontier", ScreenGate: GatePass, FrontierGate: GatePass}
	if _, err := CalibrationFromDecisions([]Decision{{SystemOneCalibration: &record}}); err == nil {
		t.Fatal("accepted calibration without the recorded screen")
	}
	screen := Decision{Kind: "system one screen", Evidence: record.ScreenEvidence, SystemOne: &verdict}
	if _, err := CalibrationFromDecisions([]Decision{screen, Decision{SystemOneCalibration: &record}, Decision{SystemOneCalibration: &record}}); err == nil {
		t.Fatal("accepted duplicate durable calibration pair")
	}
	if _, err := CalibrationFromDecisions([]Decision{screen, Decision{SystemOneCalibration: &record}}); err != nil {
		t.Fatalf("rejected calibration bound to recorded local auto-pass: %v", err)
	}
}
