package insight

import (
	"sort"
	"testing"
)

func TestMapDiagSeverity(t *testing.T) {
	cases := map[int]int{0: 0, 1: 0, 2: 1, 3: 2, 9: 2}
	for in, want := range cases {
		if got := mapDiagSeverity(in); got != want {
			t.Errorf("mapDiagSeverity(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestPluralIssues(t *testing.T) {
	if pluralIssues(1) != "1 issue" {
		t.Errorf("pluralIssues(1) = %q", pluralIssues(1))
	}
	if pluralIssues(3) != "3 issues" {
		t.Errorf("pluralIssues(3) = %q", pluralIssues(3))
	}
}

// TestBuildIsDeterministicAndAIFree is the core guarantee: Build() produces a
// complete platform view with ZERO AI involvement — Source is always "rules",
// it never blocks, and actions come back priority-sorted.
func TestBuildIsDeterministicAndAIFree(t *testing.T) {
	p := Build("") // empty root → host signals only, no panic

	if p.Source != "rules" {
		t.Errorf("Build().Source = %q, want \"rules\" (Build must never invoke AI)", p.Source)
	}
	if len(p.Signals) == 0 {
		t.Error("Build() returned no deity signals")
	}
	// Actions must be priority-sorted (deterministic ordering).
	if !sort.SliceIsSorted(p.Actions, func(i, j int) bool { return p.Actions[i].Priority < p.Actions[j].Priority }) {
		t.Error("Build() actions are not priority-sorted")
	}
	// Worst severity is the max across signals.
	maxSev := 0
	for _, s := range p.Signals {
		if s.Severity > maxSev {
			maxSev = s.Severity
		}
	}
	if p.Worst != maxSev {
		t.Errorf("Build().Worst = %d, want %d", p.Worst, maxSev)
	}
}
