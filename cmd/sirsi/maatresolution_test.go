package main

import (
	"path/filepath"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

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
