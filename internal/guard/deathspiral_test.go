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
	return spiralMockWithPS(load1, swapUsed, swapTotal, freePages, ps)
}

func spiralMockWithPS(load1 string, swapUsed, swapTotal float64, freePages int, ps string) *platform.Mock {
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

func simFanoutPS(simChildren, claudeChildren int) string {
	ps := "  PID  PPID COMM\n    1     0 /sbin/launchd\n  500     1 /Applications/Claude.app/Contents/MacOS/Claude\n  700     1 /Library/Developer/CoreSimulator/Devices/A507/data/var/run/launchd_sim\n"
	for i := 0; i < simChildren; i++ {
		ps += fmt.Sprintf("  %d   700 sim-daemon\n", 2000+i)
	}
	for i := 0; i < claudeChildren; i++ {
		ps += fmt.Sprintf("  %d   500 claude-code\n", 4000+i)
	}
	return ps
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

func TestReadSwapPctTelemetryValidity(t *testing.T) {
	tests := []struct {
		name       string
		output     string
		wantPct    float64
		wantUsedGB float64
		wantOK     bool
	}{
		{
			name:   "fresh boot allocated-zero swap is readable",
			output: "total = 0.00M  used = 0.00M  free = 0.00M  (encrypted)",
			wantOK: true,
		},
		{
			name:       "allocated swap is parsed",
			output:     "total = 4096.00M  used = 1024.00M  free = 3072.00M  (encrypted)",
			wantPct:    25,
			wantUsedGB: 1,
			wantOK:     true,
		},
		{
			name:   "missing total remains unreadable",
			output: "used = 0.00M  free = 0.00M  (encrypted)",
		},
		{
			name:   "malformed value remains unreadable",
			output: "total = unknown  used = 0.00M  free = unknown",
		},
		{
			name:   "used greater than total remains unreadable",
			output: "total = 512.00M  used = 1024.00M  free = 0.00M  (encrypted)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &platform.Mock{NameStr: "mock", CommandResults: map[string]string{
				"sysctl -n vm.swapusage": tt.output,
			}}
			pct, usedGB, ok := readSwapPct(m)
			if pct != tt.wantPct || usedGB != tt.wantUsedGB || ok != tt.wantOK {
				t.Fatalf("readSwapPct() = (%v, %v, %v), want (%v, %v, %v)",
					pct, usedGB, ok, tt.wantPct, tt.wantUsedGB, tt.wantOK)
			}
		})
	}
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

func TestMemoryDeathSpiralIgnoresBootedSimulatorFanOut(t *testing.T) {
	m := spiralMockWithPS("5.0", 2000, 26624, 500000, simFanoutPS(136, 5))
	f := spiralFinding(t, m)
	if f.Severity != SeverityOK {
		t.Fatalf("severity = %d (%s), want OK for normal launchd_sim fan-out", f.Severity, f.Message)
	}
	if strings.Contains(f.Message, "restart") || strings.Contains(f.Message, "launchd_sim") {
		t.Fatalf("message %q should not recommend restarting a booted simulator", f.Message)
	}
}

func TestMemoryDeathSpiralStillFlagsRealFanOutBesideSimulator(t *testing.T) {
	m := spiralMockWithPS("5.0", 2000, 26624, 500000, simFanoutPS(136, 244))
	f := spiralFinding(t, m)
	if f.Severity != SeverityWarn {
		t.Fatalf("severity = %d (%s), want Warn for Claude fan-out", f.Severity, f.Message)
	}
	if !strings.Contains(f.Message, "Claude has 244 child processes") {
		t.Fatalf("message %q should name the real fan-out parent", f.Message)
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

// TestIsDeathSpiral_InactiveIsAvailable pins the false-positive that paged an
// agent every 15 minutes on 2026-07-24. The host: 48 GB, swap 92% used,
// load 8.6 on 14 cores, "Pages free" 0.5 GB — but 20.2 GB sat in
// "Pages inactive", which macOS reclaims on demand, and macOS itself reported
// 85% memory free. Reading FREE alone called that a death spiral. Reading
// AVAILABLE (free + inactive) — what a process can actually get — does not.
func TestIsDeathSpiral_InactiveIsAvailable(t *testing.T) {
	const cores = 14
	tests := []struct {
		name                    string
		swapPct, load1, availGB float64
		want                    bool
	}{
		{"live host: swap full but 20.7 GB reclaimable", 92, 8.6, 20.7, false},
		{"swap full and genuinely no memory left", 92, 8.6, 0.2, true},
		{"swap full and a paging-driven load storm", 92, 30, 20.7, true},
		{"no memory but swap has room — not yet a spiral", 40, 8.6, 0.2, false},
		{"idle and empty", 10, 1, 40, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isDeathSpiral(tc.swapPct, tc.load1, tc.availGB, cores); got != tc.want {
				t.Errorf("isDeathSpiral(swap %.0f%%, load %.1f, avail %.1f GB, %d cores) = %v, want %v",
					tc.swapPct, tc.load1, tc.availGB, cores, got, tc.want)
			}
		})
	}
}
