package main

import (
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

func TestMaatCalibrateRecordsIndependentOutcomeForRecordedAutoPass(t *testing.T) {
	verdict, err := maat.Screen(maat.SystemOneScreen{
		Subject:       maat.VerdictSubject{Kind: "commit", Repo: "SirsiMaster/sirsi-pantheon", Ref: "main", HeadSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		FeatherWeight: 92, Confidence: 0.96,
		Floor: maat.FloorResult{Passed: true, Checks: []maat.FloorCheck{{Name: "gofmt", Passed: true}}},
		Model: maat.ModelStamp{Provider: "local:deterministic", Version: "v1", Local: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	journal := &maatTestJournal{}
	screen, err := maat.RecordSystemOne(journal, "sirsi maat screen", verdict)
	if err != nil {
		t.Fatal(err)
	}
	oldFactory, oldScreen, oldFrontier, oldGate, oldJSON, oldMaatJSON := newMaatDecisionJournal, maatCalibrationScreenEvidence, maatCalibrationFrontierEvidence, maatCalibrationFrontierGate, JsonOutput, maatJSON
	t.Cleanup(func() {
		newMaatDecisionJournal, maatCalibrationScreenEvidence, maatCalibrationFrontierEvidence, maatCalibrationFrontierGate, JsonOutput, maatJSON = oldFactory, oldScreen, oldFrontier, oldGate, oldJSON, oldMaatJSON
	})
	newMaatDecisionJournal = func() (maat.DecisionJournal, error) { return journal, nil }
	maatCalibrationScreenEvidence = screen.Evidence
	maatCalibrationFrontierEvidence = "review:sha256=frontier"
	maatCalibrationFrontierGate = string(maat.GateBlock)
	JsonOutput, maatJSON = true, false
	if err := maatCalibrateCmd.RunE(maatCalibrateCmd, nil); err != nil {
		t.Fatal(err)
	}
	if len(journal.decisions) != 2 || journal.decisions[1].SystemOneCalibration == nil {
		t.Fatalf("calibration journal = %+v", journal.decisions)
	}
}

func TestMaatCalibrateRejectsNonFinalIndependentGate(t *testing.T) {
	oldGate := maatCalibrationFrontierGate
	t.Cleanup(func() { maatCalibrationFrontierGate = oldGate })
	maatCalibrationFrontierGate = "escalate"
	if err := maatCalibrateCmd.RunE(maatCalibrateCmd, nil); err == nil {
		t.Fatal("accepted non-final independent gate")
	}
}
