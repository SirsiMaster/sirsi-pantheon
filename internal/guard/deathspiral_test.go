package guard

import (
	"fmt"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

// spiralMock builds a mock platform with the given memory-death signals.
// fanChildren children are parented under one Claude.app pid.
func spiralMock(load1 string, swapUsed, swapTotal float64, freePages int, fanChildren int) *platform.Mock {
	ps := "  PID  PPID COMM\n    1     0 /sbin/launchd\n  500     1 /Applications/Claude.app/Contents/MacOS/Claude\n"
	for i := 0; i < fanChildren; i++ {
		ps += fmt.Sprintf("  %d   500 claude-code\n", 1000+i)
	}
	return &platform.Mock{
		NameStr: "mock",
		CommandResults: map[string]string{
			"sysctl -n vm.loadavg":   "{ " + load1 + " 10.0 8.0 }",
			"sysctl -n hw.ncpu":      "16",
			"sysctl -n vm.swapusage": fmt.Sprintf("total = %.2fM  used = %.2fM  free = %.2fM  (encrypted)", swapTotal, swapUsed, swapTotal-swapUsed),
			"vm_stat": fmt.Sprintf(`Mach Virtual Memory Statistics: (page size of 16384 bytes)
Pages free:                    %d.
Pages occupied by compressor:  1620000.
`, freePages),
			"ps -axo pid=,ppid=,comm=": ps,
		},
	}
}

func spiralFinding(t *testing.T, m *platform.Mock) DiagnosticFinding {
	t.Helper()
	var r DoctorReport
	checkMemoryDeathSpiral(m, &r)
	if len(r.Findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(r.Findings))
	}
	return r.Findings[0]
}

// TestMemoryDeathSpiral pins the 2026-07-16 incident contract: load 31.8 /
// swap 95% / 244 leaked sessions must be Critical (live → forces RED), never
// a 6-point deduction.
func TestMemoryDeathSpiral(t *testing.T) {
	cases := []struct {
		name string
		m    *platform.Mock
		want DiagnosticSeverity
		msg  string
	}{
		{"the incident: swap 95% + load 2x cores + leak → Critical",
			spiralMock("31.80", 25231, 26624, 11000, 244), SeverityCritical, "death spiral"},
		{"swap 95% + free < 512MB, load modest → Critical",
			spiralMock("10.0", 25231, 26624, 20000, 5), SeverityCritical, "death spiral"},
		{"swap 85%, load modest, no leak → Warn",
			spiralMock("10.0", 22630, 26624, 500000, 5), SeverityWarn, "Swap 85%"},
		{"session leak alone (244 children) → Warn naming the parent",
			spiralMock("5.0", 2000, 26624, 500000, 244), SeverityWarn, "Claude has 244 child processes"},
		{"healthy machine → OK",
			spiralMock("5.0", 2000, 26624, 500000, 5), SeverityOK, "headroom normal"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := spiralFinding(t, c.m)
			if f.Severity != c.want {
				t.Fatalf("severity = %d (%s), want %d", f.Severity, f.Message, c.want)
			}
			if !strings.Contains(f.Message, c.msg) {
				t.Errorf("message %q does not contain %q", f.Message, c.msg)
			}
		})
	}
}

// TestMemoryDeathSpiralForcesRed: the spiral is a LIVE critical — the one-light
// rollup must go RED, not amber (the 94/100-during-spiral bug).
func TestMemoryDeathSpiralForcesRed(t *testing.T) {
	f := spiralFinding(t, spiralMock("31.80", 25231, 26624, 11000, 244))
	if got := classifyHealth([]DiagnosticFinding{f}); got != HealthRed {
		t.Fatalf("classifyHealth = %v, want HealthRed", got)
	}
	if remediationCommand(f) != "sirsi relieve --memory" {
		t.Fatalf("no lever: remediationCommand = %q", remediationCommand(f))
	}
	if remediationKind(f) != FixRelief {
		t.Fatalf("remediationKind = %q, want relief", remediationKind(f))
	}
}

// TestMemoryDeathSpiralUnreadableEmitsNothing: no signals → no finding, never
// a guessed severity.
func TestMemoryDeathSpiralUnreadableEmitsNothing(t *testing.T) {
	var r DoctorReport
	checkMemoryDeathSpiral(&platform.Mock{NameStr: "mock", CommandResults: map[string]string{}}, &r)
	if len(r.Findings) != 0 {
		t.Fatalf("findings = %v, want none when nothing is readable", r.Findings)
	}
}
