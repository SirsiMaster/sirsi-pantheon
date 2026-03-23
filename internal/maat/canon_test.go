package maat

import (
	"testing"
)

// --- ParseCommitLog tests ---

func TestParseCommitLogBasic(t *testing.T) {
	input := `abc1234567890
feat(maat): add QA/QC governance agent

Implements Ma'at as the first deity agent.

Refs: ADR-004, ANUBIS_RULES
Changelog: v0.4.0
---END---
def5678901234
fix(ci): golangci-lint errors

Fixed formatting.
---END---`

	commits := ParseCommitLog(input)
	if len(commits) != 2 {
		t.Fatalf("got %d commits, want 2", len(commits))
	}

	// First commit should have refs.
	if commits[0].SHA != "abc1234567890" {
		t.Errorf("SHA = %q, want abc1234567890", commits[0].SHA)
	}
	if commits[0].Subject != "feat(maat): add QA/QC governance agent" {
		t.Errorf("Subject = %q", commits[0].Subject)
	}
	if len(commits[0].Refs) == 0 {
		t.Error("first commit should have canon refs")
	}

	// Second commit has no refs.
	if commits[1].SHA != "def5678901234" {
		t.Errorf("SHA = %q, want def5678901234", commits[1].SHA)
	}
	if len(commits[1].Refs) != 0 {
		t.Errorf("second commit should have no refs, got %v", commits[1].Refs)
	}
}

func TestParseCommitLogEmpty(t *testing.T) {
	commits := ParseCommitLog("")
	if len(commits) != 0 {
		t.Errorf("empty input should produce 0 commits, got %d", len(commits))
	}
}

func TestParseCommitLogNoTrailingEnd(t *testing.T) {
	input := `abc1234
some commit subject
some body`

	commits := ParseCommitLog(input)
	if len(commits) != 1 {
		t.Fatalf("got %d commits, want 1", len(commits))
	}
	if commits[0].SHA != "abc1234" {
		t.Errorf("SHA = %q", commits[0].SHA)
	}
}

// --- findCanonRefs tests ---

func TestFindCanonRefsADR(t *testing.T) {
	refs := findCanonRefs("Implements ADR-001 and ADR-004")
	if len(refs) != 2 {
		t.Fatalf("got %d refs, want 2", len(refs))
	}
}

func TestFindCanonRefsRuleA(t *testing.T) {
	refs := findCanonRefs("Per Rule A14, all numbers must be verifiable")
	if len(refs) == 0 {
		t.Error("should find Rule A14 reference")
	}
}

func TestFindCanonRefsRefsFooter(t *testing.T) {
	refs := findCanonRefs("Refs: ANUBIS_RULES, ADR-003")
	if len(refs) < 2 {
		t.Errorf("got %d refs, want at least 2", len(refs))
	}
}

func TestFindCanonRefsNone(t *testing.T) {
	refs := findCanonRefs("just a regular commit message with no canon")
	if len(refs) != 0 {
		t.Errorf("got %d refs, want 0", len(refs))
	}
}

func TestFindCanonRefsDeduplicate(t *testing.T) {
	refs := findCanonRefs("ADR-001 ADR-001 ADR-001")
	if len(refs) != 1 {
		t.Errorf("duplicates should be removed, got %d refs", len(refs))
	}
}

// --- CanonAssessor tests ---

func TestCanonAssessorAllLinked(t *testing.T) {
	ca := &CanonAssessor{
		CommitCount: 2,
		Runner: func(count int) (string, error) {
			return `abc1234
feat(maat): governance agent

Refs: ADR-004, ANUBIS_RULES
---END---
def5678
docs(adr): add ADR-004

Refs: ADR-004
---END---`, nil
		},
	}

	assessments, err := ca.Assess()
	if err != nil {
		t.Fatalf("Assess() error = %v", err)
	}

	// Should have 2 commit assessments + 1 summary.
	if len(assessments) != 3 {
		t.Fatalf("got %d assessments, want 3", len(assessments))
	}

	// Summary should pass (100% linked).
	summary := assessments[len(assessments)-1]
	if summary.Verdict != VerdictPass {
		t.Errorf("summary verdict = %v, want pass (100%% linked)", summary.Verdict)
	}
}

func TestCanonAssessorNoneLinked(t *testing.T) {
	ca := &CanonAssessor{
		CommitCount: 2,
		Runner: func(count int) (string, error) {
			return `abc1234
fix typo
---END---
def5678
update readme
---END---`, nil
		},
	}

	assessments, err := ca.Assess()
	if err != nil {
		t.Fatalf("Assess() error = %v", err)
	}

	summary := assessments[len(assessments)-1]
	if summary.Verdict != VerdictFail {
		t.Errorf("summary verdict = %v, want fail (0%% linked)", summary.Verdict)
	}
}

func TestCanonAssessorMixed(t *testing.T) {
	ca := &CanonAssessor{
		CommitCount: 4,
		Runner: func(count int) (string, error) {
			return `abc1
feat: with refs

Refs: ADR-001
---END---
abc2
fix: no refs
---END---
abc3
docs: with Rule A14
---END---
abc4
chore: no refs
---END---`, nil
		},
	}

	assessments, err := ca.Assess()
	if err != nil {
		t.Fatalf("Assess() error = %v", err)
	}

	summary := assessments[len(assessments)-1]
	// 2/4 = 50%, which is exactly the warning boundary.
	if summary.Verdict != VerdictWarning {
		t.Errorf("summary verdict = %v, want warning (50%% linked)", summary.Verdict)
	}
}

func TestCanonAssessorNoCommits(t *testing.T) {
	ca := &CanonAssessor{
		CommitCount: 5,
		Runner: func(count int) (string, error) {
			return "", nil
		},
	}

	assessments, err := ca.Assess()
	if err != nil {
		t.Fatalf("Assess() error = %v", err)
	}

	if len(assessments) != 1 {
		t.Fatalf("got %d assessments, want 1", len(assessments))
	}
	if assessments[0].Verdict != VerdictWarning {
		t.Errorf("verdict = %v, want warning", assessments[0].Verdict)
	}
}

func TestCanonAssessorDomain(t *testing.T) {
	ca := &CanonAssessor{}
	if ca.Domain() != DomainCanon {
		t.Errorf("domain = %v, want canon", ca.Domain())
	}
}

// --- truncate tests ---

func TestTruncate(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is longer than ten", 10, "this is..."},
		{"", 5, ""},
	}
	for _, tt := range tests {
		got := truncate(tt.input, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}
