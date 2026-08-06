package routerstore

import (
	"errors"
	"strings"
	"testing"
)

func fullEvidence() Evidence {
	return Evidence{
		Commit:     "PR #517 @ abc1234",
		Tests:      "CI run 31063428143",
		Security:   "SEC-REVIEW-2026-08-06",
		Design:     "design accepted 2026-08-06",
		Deployment: "v1.4.2",
		Production: "verified on m5-sirsi 2026-08-06",
	}
}

// The completion gate. ADR-057 §6: `done` requires seven evidence references,
// and a green build is only one of them. This test exists because "tests pass,
// therefore done" is the exact claim the gate is built to refuse.
func TestSatisfyRefusesIncompleteEvidence(t *testing.T) {
	s := newTestStore(t)
	req, err := s.AddRequirement("runnable predicate", "ADR-057", "step 3", "claude-home")
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	if satErr := s.Satisfy(req.ID); !errors.Is(satErr, ErrIncompleteEvidence) {
		t.Fatalf("bare requirement must not satisfy, got %v", satErr)
	}

	// A merged PR and passing tests — the two things agents most often treat as
	// completion — must still be refused.
	if recErr := s.RecordEvidence(req.ID, Evidence{Commit: "PR #517", Tests: "CI 123"}); recErr != nil {
		t.Fatalf("record: %v", recErr)
	}
	err = s.Satisfy(req.ID)
	if !errors.Is(err, ErrIncompleteEvidence) {
		t.Fatalf("commit+tests alone must not satisfy, got %v", err)
	}
	for _, want := range []string{"security control", "design acceptance", "deployment version", "production verification"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error must name what is missing (%q), got: %v", want, err)
		}
	}

	if err := s.RecordEvidence(req.ID, fullEvidence()); err != nil {
		t.Fatalf("record full: %v", err)
	}
	if err := s.Satisfy(req.ID); err != nil {
		t.Fatalf("full evidence must satisfy, got %v", err)
	}
}

func TestRecordEvidenceIsAdditiveAndMovesStatus(t *testing.T) {
	s := newTestStore(t)
	req, _ := s.AddRequirement("leases", "ADR-057", "step 2", "claude-home")

	if err := s.RecordEvidence(req.ID, Evidence{Commit: "PR #1"}); err != nil {
		t.Fatalf("record: %v", err)
	}
	// A later pass supplying a different field must not erase the first.
	if err := s.RecordEvidence(req.ID, Evidence{Tests: "CI 9"}); err != nil {
		t.Fatalf("record 2: %v", err)
	}
	all, err := s.ListRequirements("claude-home")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("want 1 requirement, got %d", len(all))
	}
	if all[0].Evidence.Commit != "PR #1" {
		t.Fatalf("evidence must accumulate, commit was clobbered: %+v", all[0].Evidence)
	}
	if all[0].Evidence.Tests != "CI 9" {
		t.Fatalf("second field not recorded: %+v", all[0].Evidence)
	}
	if all[0].Status != ReqInProgress {
		t.Fatalf("recording evidence should move open -> in_progress, got %q", all[0].Status)
	}
}

// The third term of the runnable predicate. A lane may park only when this is
// empty; if it never empties correctly, "never stop" is unenforceable.
func TestUnmetRequirementsDrivesRunnablePredicate(t *testing.T) {
	s := newTestStore(t)

	a, _ := s.AddRequirement("registry", "ADR-057", "step 1", "claude-home")
	b, _ := s.AddRequirement("horus surface", "ADR-057", "step 7", "claude-home")
	if _, err := s.AddRequirement("someone elses", "ADR-057", "x", "claude-pantheon"); err != nil {
		t.Fatalf("add other: %v", err)
	}

	unmet, err := s.UnmetRequirements("claude-home")
	if err != nil {
		t.Fatalf("unmet: %v", err)
	}
	if len(unmet) != 2 {
		t.Fatalf("want 2 unmet for claude-home (owner-scoped), got %d", len(unmet))
	}

	if recErr := s.RecordEvidence(a.ID, fullEvidence()); recErr != nil {
		t.Fatalf("evidence: %v", recErr)
	}
	if satErr := s.Satisfy(a.ID); satErr != nil {
		t.Fatalf("satisfy: %v", satErr)
	}
	// Waived counts as met — an explicit decision, not outstanding work.
	if waiveErr := s.Waive(b.ID, "superseded by ADR-058"); waiveErr != nil {
		t.Fatalf("waive: %v", waiveErr)
	}

	unmet, err = s.UnmetRequirements("claude-home")
	if err != nil {
		t.Fatalf("unmet 2: %v", err)
	}
	if len(unmet) != 0 {
		t.Fatalf("satisfied + waived must both count as met, got %d unmet: %+v", len(unmet), unmet)
	}
}

func TestWaiverRequiresReason(t *testing.T) {
	s := newTestStore(t)
	req, _ := s.AddRequirement("thing", "ADR-057", "", "claude-home")
	if err := s.Waive(req.ID, "   "); err == nil {
		t.Fatal("an unexplained waiver is a dropped requirement wearing a terminal status — must be refused")
	}
}

func TestRequirementRequiresSourceForTraceability(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.AddRequirement("untraced", "", "", "claude-home"); err == nil {
		t.Fatal("a requirement with no canon source cannot be audited — must be refused")
	}
}

func TestRequirementIDsAreAllocatedNotGuessed(t *testing.T) {
	s := newTestStore(t)
	a, err := s.AddRequirement("first", "ADR-057", "", "claude-home")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	b, err := s.AddRequirement("second", "ADR-057", "", "claude-pantheon")
	if err != nil {
		t.Fatalf("add 2: %v", err)
	}
	if a.ID == b.ID {
		t.Fatalf("requirement ids collided: %s", a.ID)
	}
	if a.ID != "REQ-001" || b.ID != "REQ-002" {
		t.Fatalf("ids must come from the allocator in order, got %s %s", a.ID, b.ID)
	}
}

func TestSatisfyUnknownRequirementFails(t *testing.T) {
	s := newTestStore(t)
	if err := s.Satisfy("REQ-999"); err == nil {
		t.Fatal("want error for unregistered requirement")
	}
	if err := s.Satisfy("nonsense"); err == nil {
		t.Fatal("want error for malformed id")
	}
}
