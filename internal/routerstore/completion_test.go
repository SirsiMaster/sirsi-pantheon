package routerstore

import (
	"errors"
	"testing"
	"time"
)

func TestCompletionRequiresAuditedEmptyThreeSources(t *testing.T) {
	s := newTestStore(t)
	if _, ifErr1 := s.VerifyCompletion("codex-home"); !errors.Is(ifErr1, ErrNotComplete) {
		t.Fatalf("unaudited empty store claimed complete: %v", ifErr1)
	}
	if ifErr2 := s.MarkRequirementAudit("codex-home", "audit://all-canon"); ifErr2 != nil {
		t.Fatal(ifErr2)
	}
	report, err := s.VerifyCompletion("codex-home")
	if err != nil || !report.Complete {
		t.Fatalf("audited empty lane not complete: %+v err=%v", report, err)
	}
}

func TestCompletionRejectsSatisfiedRequirementWithMissingEvidence(t *testing.T) {
	s := newTestStore(t)
	if ifErr3 := s.MarkRequirementAudit("codex-home", "audit://all-canon"); ifErr3 != nil {
		t.Fatal(ifErr3)
	}
	req, err := s.AddRequirement("gap", "ADR-057", "R6", "codex-home")
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a buggy/bypassing writer; the final gate must independently
	// revalidate evidence rather than trusting the status label.
	if _, ifErr4 := s.db.Exec(`UPDATE requirements SET status='satisfied' WHERE req_id=?;`, req.ID); ifErr4 != nil {
		t.Fatal(ifErr4)
	}
	report, err := s.VerifyCompletion("codex-home")
	if !errors.Is(err, ErrNotComplete) || report.InvalidSatisfiedRequirements != 1 {
		t.Fatalf("invalid satisfied requirement passed gate: %+v err=%v", report, err)
	}
}

func TestCompletionRejectsTerminalWakeFailure(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	s.now = func() time.Time { return now }
	if ifErr5 := s.MarkRequirementAudit("codex-home", "audit://all-canon"); ifErr5 != nil {
		t.Fatal(ifErr5)
	}
	if _, ifErr6 := s.Send("owner", "codex-home", "work", "review", "x"); ifErr6 != nil {
		t.Fatal(ifErr6)
	}
	for i := 0; i < MaxWakeAttempts; i++ {
		e, err := s.ClaimWakeEvent(time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if ifErr7 := s.FailWakeEvent(e.EventID, e.LeaseToken, "no route"); ifErr7 != nil {
			t.Fatal(ifErr7)
		}
		now = now.Add(time.Duration(1<<uint(i))*time.Minute + time.Second)
	}
	items, _ := s.Inbox("codex-home")
	if ifErr8 := s.CloseItem(items[0].ID, "removed only to isolate wake failure"); ifErr8 != nil {
		t.Fatal(ifErr8)
	}
	report, err := s.VerifyCompletion("codex-home")
	if !errors.Is(err, ErrNotComplete) || report.TerminalWakeFailures != 1 {
		t.Fatalf("terminal wake failure passed completion: %+v err=%v", report, err)
	}
}
