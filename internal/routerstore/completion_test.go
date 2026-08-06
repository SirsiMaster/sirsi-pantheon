package routerstore

import (
	"errors"
	"testing"
	"time"
)

func TestCompletionRequiresAuditedEmptyThreeSources(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.VerifyCompletion("codex-home"); !errors.Is(err, ErrNotComplete) {
		t.Fatalf("unaudited empty store claimed complete: %v", err)
	}
	if err := s.MarkRequirementAudit("codex-home", "audit://all-canon"); err != nil {
		t.Fatal(err)
	}
	report, err := s.VerifyCompletion("codex-home")
	if err != nil || !report.Complete {
		t.Fatalf("audited empty lane not complete: %+v err=%v", report, err)
	}
}

func TestCompletionRejectsSatisfiedRequirementWithMissingEvidence(t *testing.T) {
	s := newTestStore(t)
	if err := s.MarkRequirementAudit("codex-home", "audit://all-canon"); err != nil {
		t.Fatal(err)
	}
	req, err := s.AddRequirement("gap", "ADR-057", "R6", "codex-home")
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a buggy/bypassing writer; the final gate must independently
	// revalidate evidence rather than trusting the status label.
	if _, err := s.db.Exec(`UPDATE requirements SET status='satisfied' WHERE req_id=?;`, req.ID); err != nil {
		t.Fatal(err)
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
	if err := s.MarkRequirementAudit("codex-home", "audit://all-canon"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Send("owner", "codex-home", "work", "review", "x"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < MaxWakeAttempts; i++ {
		e, err := s.ClaimWakeEvent(time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.FailWakeEvent(e.EventID, e.LeaseToken, "no route"); err != nil {
			t.Fatal(err)
		}
		now = now.Add(time.Duration(1<<uint(i))*time.Minute + time.Second)
	}
	items, _ := s.Inbox("codex-home")
	if err := s.CloseItem(items[0].ID, "removed only to isolate wake failure"); err != nil {
		t.Fatal(err)
	}
	report, err := s.VerifyCompletion("codex-home")
	if !errors.Is(err, ErrNotComplete) || report.TerminalWakeFailures != 1 {
		t.Fatalf("terminal wake failure passed completion: %+v err=%v", report, err)
	}
}
