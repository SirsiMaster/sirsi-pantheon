package autoheal

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

func finding(check, fix string, sev guard.DiagnosticSeverity) guard.DiagnosticFinding {
	return guard.DiagnosticFinding{Check: check, Message: check + " degraded", Fix: fix, Severity: sev}
}

// harness wires every seam; returns the recorded exec calls.
func harness(t *testing.T, auto bool, findings []guard.DiagnosticFinding, gated map[string]bool) *[][]string {
	t.Helper()
	var calls [][]string
	statePath := filepath.Join(t.TempDir(), "state.json")
	setSeams(
		func() bool { return auto },
		func() ([]guard.DiagnosticFinding, error) { return findings, nil },
		func(action string) router.GateDecision {
			for k := range gated {
				if strings.Contains(action, k) {
					return router.GateDecision{Gated: true, Reason: "test rule"}
				}
			}
			return router.GateDecision{}
		},
		func(argv []string) error {
			calls = append(calls, argv)
			if argv[len(argv)-1] == "--boom" {
				return errors.New("boom")
			}
			return nil
		},
		func() string { return statePath },
	)
	return &calls
}

func TestAutoHeal_OffIsInert(t *testing.T) {
	calls := harness(t, false, []guard.DiagnosticFinding{
		finding("Swap Usage", "sirsi relieve --memory", guard.SeverityCritical),
	}, nil)
	if err := Run("", ""); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 0 {
		t.Fatalf("autonomous OFF executed %v — must observe/propose only", *calls)
	}
}

func TestAutoHeal_ExplicitApprovedRunBypassesPassiveSwitch(t *testing.T) {
	calls := harness(t, false, []guard.DiagnosticFinding{
		finding("Swap Usage", "sirsi relieve --memory", guard.SeverityCritical),
	}, nil)
	outcomes, err := RunApprovedReport("", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 {
		t.Fatalf("explicit approved run executed %v, want one fix", *calls)
	}
	if len(outcomes) != 1 || !outcomes[0].Applied {
		t.Fatalf("outcomes = %+v, want applied fix", outcomes)
	}
}

func TestAutoHeal_AppliesWarnPlusLevers(t *testing.T) {
	calls := harness(t, true, []guard.DiagnosticFinding{
		finding("Swap Usage", "sirsi relieve --memory", guard.SeverityCritical),
		finding("Disk Space", "sirsi clean --include-caution", guard.SeverityWarn),
		finding("Healthy Thing", "sirsi relieve", guard.SeverityOK), // below Warn — never touched
		finding("No Lever", "", guard.SeverityCritical),             // no fix — never touched
	}, nil)
	if err := Run("", ""); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 2 {
		t.Fatalf("exec calls = %v, want the two Warn+ levers", *calls)
	}
	if got := strings.Join((*calls)[0], " "); got != "sirsi relieve --memory --confirm --quiet" {
		t.Errorf("call[0] = %q", got)
	}
	if got := strings.Join((*calls)[1], " "); got != "sirsi clean --include-caution --confirm --yes --quiet" {
		t.Errorf("call[1] = %q", got)
	}
}

// TestAutoHeal_GateActionSecondLine: the previously-unwired ADR-039 P3 floor —
// a Gated action text downgrades to a proposal, never executes.
func TestAutoHeal_GateActionSecondLine(t *testing.T) {
	calls := harness(t, true, []guard.DiagnosticFinding{
		finding("Scary Thing", "sirsi relieve", guard.SeverityCritical),
	}, map[string]bool{"Scary Thing": true})
	if err := Run("", ""); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 0 {
		t.Fatalf("gated action executed %v — GateAction floor breached", *calls)
	}
}

func TestAutoHeal_WhitelistNonSirsiVerb(t *testing.T) {
	calls := harness(t, true, []guard.DiagnosticFinding{
		finding("Weird", "rm -rf /tmp/x", guard.SeverityCritical),
	}, nil)
	if err := Run("", ""); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 0 {
		t.Fatalf("non-sirsi verb executed %v — whitelist breached", *calls)
	}
}

func TestAutoHeal_CooldownAndBudget(t *testing.T) {
	f := []guard.DiagnosticFinding{
		finding("A", "sirsi relieve --memory", guard.SeverityCritical),
		finding("B", "sirsi clean --include-caution", guard.SeverityCritical),
		finding("C", "sirsi reclaim-snapshots", guard.SeverityWarn),
		finding("D", "sirsi self-update", guard.SeverityWarn), // 4th — over the per-pass budget
	}
	calls := harness(t, true, f, nil)
	if err := Run("", ""); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != maxFixesPerPass {
		t.Fatalf("first pass = %d execs, want budget %d", len(*calls), maxFixesPerPass)
	}
	// Second pass immediately after: everything applied is inside its cooldown.
	before := len(*calls)
	if err := Run("", ""); err != nil {
		t.Fatal(err)
	}
	// Only D (never run, budget freed) may run now.
	ran := (*calls)[before:]
	if len(ran) != 0 {
		t.Fatalf("second pass = %v, want no unsupported preview-only auto-apply", ran)
	}
	outcomes, err := RunReport("", "")
	if err != nil {
		t.Fatal(err)
	}
	var unsupported bool
	for _, o := range outcomes {
		if o.Check == "D" && strings.Contains(o.Reason, "no-approved-apply-form") {
			unsupported = true
		}
	}
	if !unsupported {
		t.Fatalf("second pass = %v, want only the budget-deferred sirsi self-update (cooldown holds the rest)", ran)
	}
}

func TestAutoHeal_ExplicitApprovedRunBypassesLoopCooldown(t *testing.T) {
	calls := harness(t, true, []guard.DiagnosticFinding{
		finding("Swap Usage", "sirsi relieve --memory", guard.SeverityCritical),
	}, nil)
	if err := Run("", ""); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 {
		t.Fatalf("first loop pass calls = %v, want one fix", *calls)
	}
	outcomes, err := RunApprovedReport("", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 2 {
		t.Fatalf("explicit run should bypass loop cooldown and fix now; calls = %v", *calls)
	}
	if len(outcomes) != 1 || !outcomes[0].Applied || outcomes[0].Reason != "applied" {
		t.Fatalf("outcomes = %+v, want applied despite prior loop cooldown", outcomes)
	}
}

func TestAutoHeal_ApprovedApplyPlanUsesApplyFlagsAndQuiet(t *testing.T) {
	tests := []struct {
		fix  string
		want string
	}{
		{"sirsi relieve --memory", "sirsi relieve --memory --confirm --quiet"},
		{"sirsi relieve", "sirsi relieve --confirm --quiet"},
		{"sirsi clean --include-caution", "sirsi clean --include-caution --confirm --yes --quiet"},
		{"sirsi reclaim-snapshots", "sirsi reclaim-snapshots --confirm --quiet"},
		{"sirsi reap-sessions", "sirsi reap-sessions --apply --quiet"},
		{"sirsi liveness-watch install", "sirsi liveness-watch install --quiet"},
	}
	for _, tt := range tests {
		t.Run(tt.fix, func(t *testing.T) {
			plan, err := approvedApplyPlan(tt.fix)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(plan.argv, " "); got != tt.want {
				t.Fatalf("argv = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAutoHeal_UnsupportedPreviewOnlyFixIsNotMarkedApplied(t *testing.T) {
	calls := harness(t, true, []guard.DiagnosticFinding{
		finding("Binary Drift", "sirsi self-update", guard.SeverityCritical),
	}, nil)
	outcomes, err := RunReport("", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 0 {
		t.Fatalf("unsupported preview-only fix executed %v", *calls)
	}
	if len(outcomes) != 1 || outcomes[0].Applied || !strings.Contains(outcomes[0].Reason, "no-approved-apply-form") {
		t.Fatalf("outcomes = %+v, want held unsupported auto-apply", outcomes)
	}
}

func TestAutoHeal_FixFailureIsolatedAndReported(t *testing.T) {
	calls := harness(t, true, []guard.DiagnosticFinding{
		finding("Bad", "sirsi relieve --boom", guard.SeverityCritical),
		finding("Good", "sirsi relieve --memory", guard.SeverityCritical),
	}, nil)
	if err := Run("", ""); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 2 {
		t.Fatalf("calls = %v — a failing fix must not stop the pass", *calls)
	}
}
