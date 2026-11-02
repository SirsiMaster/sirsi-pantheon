package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/casebook"
)

func TestMaatRecordResolutionEvidenceCarriesCommittedCasebookIdentity(t *testing.T) {
	journal := &maat.FileDecisionJournal{Path: filepath.Join(t.TempDir(), "decisions.jsonl")}
	decision, err := maat.RecordDiagnosticOwnerReview(journal, "fixture", maat.DiagnosticOwnerReview{
		Check: "host health", Message: "review finding", Detail: "retained detail",
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := journal.Recent(1)
	if err != nil || len(rows) != 1 {
		t.Fatalf("persisted owner review = %+v, err=%v", rows, err)
	}
	returnedCases := casebook.Build([]maat.Decision{decision}).Cases
	persistedCases := casebook.Build(rows).Cases
	if len(returnedCases) != 1 || len(persistedCases) != 1 || returnedCases[0].ID != persistedCases[0].ID ||
		rows[0].Time != decision.Time || rows[0].Host != decision.Host {
		t.Fatalf("returned owner identity differs from committed row: returned=%+v persisted=%+v", decision, rows)
	}
	var ids, evidences []string
	for _, fact := range maatRecordResolutionEvidence(decision) {
		switch fact.Label {
		case "Case ID":
			ids = append(ids, fact.Value)
		case "Evidence":
			evidences = append(evidences, fact.Value)
		}
	}
	if len(ids) != 1 || ids[0] != persistedCases[0].ID || !strings.HasPrefix(ids[0], "maat-case-") {
		t.Fatalf("result case ID facts = %v, persisted projection = %q", ids, persistedCases[0].ID)
	}
	if len(evidences) != 1 || evidences[0] != rows[0].Evidence {
		t.Fatalf("result evidence facts = %v, persisted evidence = %q", evidences, rows[0].Evidence)
	}
}

func TestMaatOwnerResolutionCommandsRecordThenAcceptExactlyOneCase(t *testing.T) {
	oldFactory := newMaatDecisionJournal
	oldCheck, oldMessage, oldDetail, oldOrigin, oldRecordConfirm := maatResolutionCheck, maatResolutionMessage, maatResolutionDetail, maatResolutionOriginEvidence, maatResolutionConfirm
	oldEvidence, oldNote, oldAcceptConfirm := maatAcceptanceEvidence, maatAcceptanceNote, maatAcceptanceConfirm
	t.Cleanup(func() {
		newMaatDecisionJournal = oldFactory
		maatResolutionCheck, maatResolutionMessage, maatResolutionDetail, maatResolutionOriginEvidence, maatResolutionConfirm = oldCheck, oldMessage, oldDetail, oldOrigin, oldRecordConfirm
		maatAcceptanceEvidence, maatAcceptanceNote, maatAcceptanceConfirm = oldEvidence, oldNote, oldAcceptConfirm
	})

	journal := &maat.FileDecisionJournal{Path: filepath.Join(t.TempDir(), "maat", "decisions.jsonl")}
	newMaatDecisionJournal = func() (maat.DecisionJournal, error) { return journal, nil }
	maatResolutionCheck, maatResolutionMessage, maatResolutionDetail, maatResolutionOriginEvidence, maatResolutionConfirm = "Kernel Panics (7d)", "Two kernel panics need an owner decision.", "retained observation", "maat-report:fixture", true
	if err := maatRecordResolutionCmd.RunE(maatRecordResolutionCmd, nil); err != nil {
		t.Fatal(err)
	}
	rows, err := journal.Recent(10)
	if err != nil || len(rows) != 1 || rows[0].OriginEvidence != "maat-report:fixture" {
		t.Fatalf("owner review rows = %+v, err = %v", rows, err)
	}
	maatAcceptanceEvidence, maatAcceptanceNote, maatAcceptanceConfirm = rows[0].Evidence, "Owner accepted the documented next step.", true
	if acceptErr := maatAcceptResolutionCmd.RunE(maatAcceptResolutionCmd, nil); acceptErr != nil {
		t.Fatal(acceptErr)
	}
	rows, err = journal.Recent(10)
	if err != nil || len(rows) != 2 || rows[0].ResolutionFor != maatAcceptanceEvidence {
		t.Fatalf("owner acceptance rows = %+v, err = %v", rows, err)
	}
}

func TestMaatOwnerResolutionCommandsRequireExplicitConfirmation(t *testing.T) {
	oldConfirm := maatResolutionConfirm
	t.Cleanup(func() { maatResolutionConfirm = oldConfirm })
	maatResolutionConfirm = false
	if err := maatRecordResolutionCmd.RunE(maatRecordResolutionCmd, nil); err == nil {
		t.Fatal("owner-resolution record command did not require --confirm")
	}
}
