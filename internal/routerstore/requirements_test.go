package routerstore

import (
	"errors"
	"strings"
	"testing"
	"time"
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

func TestRequirementMutationsUseInjectedClock(t *testing.T) {
	s := newTestStore(t)
	now := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	s.now = func() time.Time { return now }
	req, err := s.AddRequirement("clock", "ADR-061", "R1", "codex-home")
	if err != nil {
		t.Fatal(err)
	}
	if req.Created != now.Format(time.RFC3339) || req.Updated != now.Format(time.RFC3339) {
		t.Fatalf("creation ignored injected clock: %+v", req)
	}
	now = now.Add(time.Hour)
	if ifErr1 := s.RecordEvidence(req.ID, Evidence{Commit: "PR"}); ifErr1 != nil {
		t.Fatal(ifErr1)
	}
	all, _ := s.ListRequirements("codex-home")
	if all[0].Updated != now.Format(time.RFC3339) {
		t.Fatalf("mutation ignored injected clock: %+v", all[0])
	}
}

// The completion gate. ADR-061 §6: `done` requires seven evidence references,
// and a green build is only one of them. This test exists because "tests pass,
// therefore done" is the exact claim the gate is built to refuse.
func TestSatisfyRefusesIncompleteEvidence(t *testing.T) {
	s := newTestStore(t)
	req, err := s.AddRequirement("runnable predicate", "ADR-061", "step 3", "claude-home")
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	if ifErr2 := s.Satisfy(req.ID); !errors.Is(ifErr2, ErrIncompleteEvidence) {
		t.Fatalf("bare requirement must not satisfy, got %v", ifErr2)
	}

	// A merged PR and passing tests — the two things agents most often treat as
	// completion — must still be refused.
	if ifErr3 := s.RecordEvidence(req.ID, Evidence{Commit: "PR #517", Tests: "CI 123"}); ifErr3 != nil {
		t.Fatalf("record: %v", ifErr3)
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

	if ifErr4 := s.RecordEvidence(req.ID, fullEvidence()); ifErr4 != nil {
		t.Fatalf("record full: %v", ifErr4)
	}
	if ifErr5 := s.Satisfy(req.ID); ifErr5 != nil {
		t.Fatalf("full evidence must satisfy, got %v", ifErr5)
	}
}

func TestRecordEvidenceIsAdditiveAndMovesStatus(t *testing.T) {
	s := newTestStore(t)
	req, _ := s.AddRequirement("leases", "ADR-061", "step 2", "claude-home")

	if ifErr6 := s.RecordEvidence(req.ID, Evidence{Commit: "PR #1"}); ifErr6 != nil {
		t.Fatalf("record: %v", ifErr6)
	}
	// A later pass supplying a different field must not erase the first.
	if ifErr7 := s.RecordEvidence(req.ID, Evidence{Tests: "CI 9"}); ifErr7 != nil {
		t.Fatalf("record 2: %v", ifErr7)
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

	a, _ := s.AddRequirement("registry", "ADR-061", "step 1", "claude-home")
	b, _ := s.AddRequirement("horus surface", "ADR-061", "step 7", "claude-home")
	if _, ifErr8 := s.AddRequirement("someone elses", "ADR-061", "x", "claude-pantheon"); ifErr8 != nil {
		t.Fatalf("add other: %v", ifErr8)
	}

	unmet, err := s.UnmetRequirements("claude-home")
	if err != nil {
		t.Fatalf("unmet: %v", err)
	}
	if len(unmet) != 2 {
		t.Fatalf("want 2 unmet for claude-home (owner-scoped), got %d", len(unmet))
	}

	if ifErr9 := s.RecordEvidence(a.ID, fullEvidence()); ifErr9 != nil {
		t.Fatalf("evidence: %v", ifErr9)
	}
	if ifErr10 := s.Satisfy(a.ID); ifErr10 != nil {
		t.Fatalf("satisfy: %v", ifErr10)
	}
	// Waived counts as met — an explicit decision, not outstanding work.
	decision, _ := s.Send("owner", "claude-home", "waive", "decision", "supersede")
	if ifErr11 := s.CloseItem(decision, "owner approved supersession"); ifErr11 != nil {
		t.Fatal(ifErr11)
	}
	if ifErr12 := s.Waive(b.ID, "superseded by ADR-058", decision); ifErr12 != nil {
		t.Fatalf("waive: %v", ifErr12)
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
	req, _ := s.AddRequirement("thing", "ADR-061", "", "claude-home")
	if ifErr13 := s.Waive(req.ID, "   ", "missing"); ifErr13 == nil {
		t.Fatal("an unexplained waiver is a dropped requirement wearing a terminal status — must be refused")
	}
}

func TestRequirementRequiresSourceForTraceability(t *testing.T) {
	s := newTestStore(t)
	if _, ifErr14 := s.AddRequirement("untraced", "", "", "claude-home"); ifErr14 == nil {
		t.Fatal("a requirement with no canon source cannot be audited — must be refused")
	}
}

func TestRequirementIDsAreAllocatedNotGuessed(t *testing.T) {
	s := newTestStore(t)
	a, err := s.AddRequirement("first", "ADR-061", "", "claude-home")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	b, err := s.AddRequirement("second", "ADR-061", "", "claude-pantheon")
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
	if ifErr15 := s.Satisfy("REQ-999"); ifErr15 == nil {
		t.Fatal("want error for unregistered requirement")
	}
	if ifErr16 := s.Satisfy("nonsense"); ifErr16 == nil {
		t.Fatal("want error for malformed id")
	}
}
