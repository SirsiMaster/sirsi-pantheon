package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

func TestHandleMaatKnowledgeProjectsSameFilteredLocalView(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".config", "seshat", "store", "latest.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	data := []byte("[{\"title\":\"Safe evidence\",\"summary\":\"Ma'at tracks the package receipt.\",\"references\":[{\"type\":\"file\",\"value\":\"docs/evidence/receipt.json\"}]},{\"title\":\"Withheld\",\"summary\":\"token=examplevalue\",\"references\":[]}]")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	oldHome := maatKnowledgeHome
	t.Cleanup(func() { maatKnowledgeHome = oldHome })
	maatKnowledgeHome = func() (string, error) { return home, nil }

	result, err := handleMaatKnowledge(map[string]interface{}{"query": "receipt"})
	if err != nil || result.IsError || len(result.Content) != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	var view struct {
		Items []struct {
			Title string `json:"title"`
		} `json:"items"`
		Total    int `json:"total"`
		Withheld int `json:"withheld"`
	}
	if err := json.Unmarshal([]byte(result.Content[0].Text), &view); err != nil {
		t.Fatal(err)
	}
	if view.Total != 1 || view.Withheld != 1 || len(view.Items) != 1 || view.Items[0].Title != "Safe evidence" {
		t.Fatalf("Ma'at knowledge MCP projection = %+v", view)
	}
}

func TestHandleMaatKnowledgeRejectsMalformedQuery(t *testing.T) {
	result, err := handleMaatKnowledge(map[string]interface{}{"query": float64(1)})
	if err != nil || !result.IsError {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

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

// MCP is an equal Casebook projection, not a summary-only agent endpoint.
// Pin the exact recovery hint so an agent client receives the same evidence
// the CLI, TUI, dashboard, and native app present—without getting execution
// authority over producer-supplied text.
func TestHandleMaatCasebookProjectsSystemOneRecoveryHint(t *testing.T) {
	oldOpen := openMaatCasebookJournal
	t.Cleanup(func() { openMaatCasebookJournal = oldOpen })
	verdict, err := maat.Screen(maat.SystemOneScreen{
		Subject:       maat.VerdictSubject{Kind: "commit", Repo: "SirsiMaster/sirsi-pantheon", Ref: "main", HeadSHA: strings.Repeat("a", 40)},
		FeatherWeight: 91,
		Confidence:    0.97,
		Findings: []maat.ScreenFinding{{
			ID: "receipt-missing", Severity: "block", Category: "provenance", File: "docs/evidence/release.json", Line: 4,
			Claim: "release receipt is missing", Evidence: "receipt:missing", Confidence: 0.98,
			FixHint: "Create and independently review the exact release receipt.",
		}},
		Floor: maat.FloorResult{Passed: true, Checks: []maat.FloorCheck{{Name: "gofmt", Passed: true}}},
		Model: maat.ModelStamp{Provider: "local:deterministic", Version: "v1", Local: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	journal := &maatCasebookJournal{rows: []maat.Decision{{
		Time: "2026-09-28T03:00:00Z", Host: "m5", Kind: "system one screen", Requester: "sirsi maat screen",
		Assessed: "commit main", Determination: string(verdict.Gate), Why: "release receipt missing", Evidence: "maat-system-one:sha256=screen", SystemOne: &verdict,
	}}}
	openMaatCasebookJournal = func() (maat.DecisionJournal, error) { return journal, nil }

	result, err := handleMaatCasebook(map[string]interface{}{"limit": float64(5)})
	if err != nil || result.IsError || len(result.Content) != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	var view struct {
		Cases []struct {
			SystemOne *maat.MaatVerdict `json:"system_one"`
		} `json:"cases"`
	}
	if err := json.Unmarshal([]byte(result.Content[0].Text), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Cases) != 1 || view.Cases[0].SystemOne == nil || len(view.Cases[0].SystemOne.Findings) != 1 || view.Cases[0].SystemOne.Findings[0].FixHint != "Create and independently review the exact release receipt." {
		t.Fatalf("Maat MCP recovery projection = %+v", view)
	}
}

func TestHandleMaatCasebookProjectsFailedFloorRecoverySteps(t *testing.T) {
	oldOpen := openMaatCasebookJournal
	t.Cleanup(func() { openMaatCasebookJournal = oldOpen })
	verdict, err := maat.Screen(maat.SystemOneScreen{
		Subject:       maat.VerdictSubject{Kind: "commit", Repo: "SirsiMaster/sirsi-pantheon", Ref: "main", HeadSHA: strings.Repeat("a", 40)},
		FeatherWeight: 91, Confidence: 0.98,
		Floor:    maat.FloorResult{Passed: false, Checks: []maat.FloorCheck{{Name: "receipt schema", Passed: false, Detail: "the receipt is malformed"}}},
		Findings: []maat.ScreenFinding{{ID: "receipt-schema", Severity: "block", Category: "provenance", Claim: "release receipt is malformed", Evidence: "receipt:invalid", Confidence: 0.99, FixHint: "Regenerate the exact release receipt."}},
		Model:    maat.ModelStamp{Provider: "local:deterministic", Version: "v1", Local: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	journal := &maatCasebookJournal{rows: []maat.Decision{{Time: "2026-09-28T04:00:00Z", Host: "m5", Kind: "system one screen", Requester: "sirsi maat screen", Assessed: "commit main", Determination: string(verdict.Gate), Why: "receipt schema failed", Evidence: "maat-system-one:sha256=failed-floor", SystemOne: &verdict}}}
	openMaatCasebookJournal = func() (maat.DecisionJournal, error) { return journal, nil }

	result, err := handleMaatCasebook(map[string]interface{}{"limit": float64(5)})
	if err != nil || result.IsError || len(result.Content) != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	var view struct {
		Cases []struct {
			NextAction struct {
				Kind  string `json:"kind"`
				Steps []struct {
					Level int    `json:"level"`
					Title string `json:"title"`
				} `json:"steps"`
			} `json:"next_action"`
		} `json:"cases"`
	}
	if err := json.Unmarshal([]byte(result.Content[0].Text), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Cases) != 1 || view.Cases[0].NextAction.Kind != "system_one_floor_recovery" || len(view.Cases[0].NextAction.Steps) != 3 || view.Cases[0].NextAction.Steps[2].Level != 3 || view.Cases[0].NextAction.Steps[2].Title != "Re-screen and record review" {
		t.Fatalf("Maat MCP failed-floor recovery projection = %+v", view)
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
