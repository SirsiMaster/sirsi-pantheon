package main

import (
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
)

type capturedDecisionJournal struct{ decisions []maat.Decision }

func (j *capturedDecisionJournal) Append(d maat.Decision) error {
	j.decisions = append(j.decisions, d)
	return nil
}

func TestMaatCasebookCommandProjectsJournalWithoutPolicyWrites(t *testing.T) {
	oldFactory := newMaatDecisionJournal
	oldKind, oldStatus, oldLimit, oldJSON := maatCasebookKind, maatCasebookStatus, maatCasebookLimit, maatJSON
	defer func() {
		newMaatDecisionJournal = oldFactory
		maatCasebookKind, maatCasebookStatus, maatCasebookLimit, maatJSON = oldKind, oldStatus, oldLimit, oldJSON
	}()
	journal := &capturedDecisionJournal{decisions: []maat.Decision{{
		Time: "2026-09-27T09:00:00Z", Host: "m5", Kind: "reservation refusal", Requester: "codex-pantheon", Resource: "m5",
		Assessed: "occupied", Determination: "refuse", Why: "live reservation", Evidence: "reservation:m5-1",
	}}}
	newMaatDecisionJournal = func() (maat.DecisionJournal, error) { return journal, nil }
	maatCasebookKind, maatCasebookStatus, maatCasebookLimit, maatJSON = "allocation", "open", 10, false
	if err := maatCasebookCmd.RunE(maatCasebookCmd, []string{"m5"}); err != nil {
		t.Fatalf("casebook command: %v", err)
	}
	if len(journal.decisions) != 1 || journal.decisions[0].Determination != "refuse" {
		t.Fatalf("casebook changed source journal: %+v", journal.decisions)
	}
	maatCasebookStatus = "invented"
	err := maatCasebookCmd.RunE(maatCasebookCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid --status") {
		t.Fatalf("invalid status error = %v", err)
	}
}

func (j *capturedDecisionJournal) Recent(int) ([]maat.Decision, error) { return j.decisions, nil }

func TestRecordReservationDecisionExplainsGrantAndRefusal(t *testing.T) {
	oldFactory := newMaatDecisionJournal
	defer func() { newMaatDecisionJournal = oldFactory }()
	journal := &capturedDecisionJournal{}
	newMaatDecisionJournal = func() (maat.DecisionJournal, error) { return journal, nil }

	req := schedule.Reservation{Holder: "codex-pantheon", Resource: "m5"}
	if err := recordReservationDecision(req, schedule.ReserveResult{Granted: true, Reservation: &schedule.Reservation{ID: "m5-1"}}); err != nil {
		t.Fatal(err)
	}
	if err := recordReservationDecision(req, schedule.ReserveResult{Conflict: &schedule.Reservation{ID: "m5-2", Holder: "claude-io"}}); err != nil {
		t.Fatal(err)
	}
	if len(journal.decisions) != 2 {
		t.Fatalf("recorded %d decisions, want 2", len(journal.decisions))
	}
	if got := journal.decisions[0]; got.Determination != "grant" || got.Evidence != "reservation:m5-1" {
		t.Fatalf("grant record = %+v", got)
	}
	if got := journal.decisions[1]; got.Determination != "refuse" || got.Affected != "claude-io" || got.Evidence != "reservation:m5-2" {
		t.Fatalf("refusal record = %+v", got)
	}
}
