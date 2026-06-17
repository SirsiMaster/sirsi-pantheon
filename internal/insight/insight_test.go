package insight

import (
	"sort"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
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

func TestIsSpotlightOffender(t *testing.T) {
	yes := []string{"spotlightknowledged ×11 | FPCKService ×1", "mds_stores ×3", "mdworker ×2"}
	no := []string{"Google Chrome ×4 | Slack ×2", ""}
	for _, d := range yes {
		if !isSpotlightOffender(d) {
			t.Errorf("isSpotlightOffender(%q) = false, want true", d)
		}
	}
	for _, d := range no {
		if isSpotlightOffender(d) {
			t.Errorf("isSpotlightOffender(%q) = true, want false", d)
		}
	}
}

func TestHorusActions_AppHangsRelief(t *testing.T) {
	// A Spotlight indexer offender → the Spotlight-exclude relief (renice would
	// fail on a root daemon).
	f := guard.DiagnosticFinding{Check: "App Hangs (7d)", Severity: guard.SeverityCritical,
		Message: "17 hang/CPU-spike events", Detail: "spotlightknowledged ×11"}
	acts := horusActions(f, 3)
	if len(acts) != 1 || acts[0].Command != "sirsi spotlight-exclude ~/Development" {
		t.Fatalf("spotlight hang relief: got %+v, want spotlight-exclude", acts)
	}

	// A normal CPU hog → guard's renice throttler.
	f.Detail = "Google Chrome Helper ×6"
	acts = horusActions(f, 3)
	if len(acts) != 1 || acts[0].Command != "sirsi guard" {
		t.Fatalf("generic hang relief: got %+v, want sirsi guard", acts)
	}

	// Healthy (OK severity) → no action.
	if a := horusActions(guard.DiagnosticFinding{Check: "App Hangs (7d)", Severity: guard.SeverityOK}, 0); len(a) != 0 {
		t.Errorf("healthy hangs should yield no action, got %+v", a)
	}
}
