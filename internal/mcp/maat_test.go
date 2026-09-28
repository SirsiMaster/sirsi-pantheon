package mcp

import (
	"encoding/json"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

type maatCasebookJournal struct{ rows []maat.Decision }

func (j *maatCasebookJournal) Append(decision maat.Decision) error {
	j.rows = append(j.rows, decision)
	return nil
}

func (j *maatCasebookJournal) Recent(limit int) ([]maat.Decision, error) {
	if limit <= 0 || limit > len(j.rows) {
		limit = len(j.rows)
	}
	return append([]maat.Decision(nil), j.rows[:limit]...), nil
}

func TestHandleMaatCasebookProjectsSharedCalibrationEvidence(t *testing.T) {
	oldOpen := openMaatCasebookJournal
	t.Cleanup(func() { openMaatCasebookJournal = oldOpen })
	journal := &maatCasebookJournal{rows: []maat.Decision{{
		Time: "2026-09-28T02:00:00Z", Host: "m5", Kind: "system one calibration", Requester: "sirsi maat calibrate",
		Assessed: "System One auto-pass calibration", Determination: "block", Why: "independent review overturned a local System One auto-pass", Evidence: "maat-system-one-calibration:sha256=record",
		SystemOneCalibration: &maat.CalibrationRecord{SchemaVersion: maat.SystemOneSchemaVersion, ScreenEvidence: "maat-system-one:sha256=screen", FrontierEvidence: "review:sha256=independent", ScreenGate: maat.GatePass, FrontierGate: maat.GateBlock},
	}}}
	openMaatCasebookJournal = func() (maat.DecisionJournal, error) { return journal, nil }

	result, err := handleMaatCasebook(map[string]interface{}{"query": "independent", "status": "resolved", "limit": float64(5)})
	if err != nil || result.IsError || len(result.Content) != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	var view struct {
		Cases []struct {
			Status      string                  `json:"status"`
			Calibration *maat.CalibrationRecord `json:"system_one_calibration"`
		} `json:"cases"`
	}
	if err := json.Unmarshal([]byte(result.Content[0].Text), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Cases) != 1 || view.Cases[0].Status != "resolved" || view.Cases[0].Calibration == nil || view.Cases[0].Calibration.FrontierEvidence != "review:sha256=independent" {
		t.Fatalf("Maat MCP projection = %+v", view)
	}
}

func TestHandleMaatCasebookRejectsMalformedFilters(t *testing.T) {
	for _, args := range []map[string]interface{}{
		{"status": "blocked"},
		{"limit": float64(1.5)},
		{"query": float64(1)},
	} {
		result, err := handleMaatCasebook(args)
		if err != nil || !result.IsError {
			t.Fatalf("args=%+v result=%+v err=%v", args, result, err)
		}
	}
}
