package main

import (
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
)

type capturedDecisionJournal struct{ decisions []maat.Decision }

func (j *capturedDecisionJournal) Append(d maat.Decision) error {
	j.decisions = append(j.decisions, d)
	return nil
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
