package agentguard

import (
	"context"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

// healthyDoctor is the injected system-health report for command-policy tests:
// a clean machine with zero findings. Injecting it (Rule A16) makes the preflight
// deterministic — guard.DoctorWith reads some state directly (not via Platform),
// so on a loaded CI box a live Critical would BLOCK an otherwise-safe command and
// flake TestPreflightAllows*/TestSafeRun*. These tests exercise COMMAND policy;
// live-health gating is covered separately against the real doctor.
func healthyDoctor(platform.Platform) (*guard.DoctorReport, error) {
	return &guard.DoctorReport{}, nil
}

func healthyPlatform() *platform.Mock {
	return &platform.Mock{
		NameStr: "mock",
		CommandResults: map[string]string{
			"sysctl -n hw.memsize":               "17179869184",
			"sysctl -n vm.swapusage":             "total = 0.00M  used = 0.00M  free = 0.00M  (encrypted)",
			"df -h /":                            "Filesystem Size Used Avail Capacity Mounted on\n/dev/disk 460Gi 100Gi 360Gi 22% /\n",
			"ps -axo pid,rss,vsz,%cpu,user,comm": "  PID RSS VSZ %CPU USER COMM\n  100 1024 2048 0.1 user /bin/zsh\n",
			"ps -axo pid,rss,comm":               "  PID RSS COMM\n  100 1024 /bin/zsh\n",
			"vm_stat": `Mach Virtual Memory Statistics: (page size of 16384 bytes)
Pages free: 400000.
Pages active: 200000.
Pages wired down: 100000.
Pages occupied by compressor: 25000.`,
		},
	}
}

func TestPreflightAllowsHealthyNarrowCommand(t *testing.T) {
	report := Preflight(PreflightOptions{
		Command:        []string{"rg", "--files", "internal/agentguard"},
		Platform:       healthyPlatform(),
		LoadProvider:   func() (float64, float64, error) { return 1, 1, nil },
		HealthProvider: healthyDoctor,
	})
	if report.Verdict != VerdictAllow {
		t.Fatalf("verdict = %s, want allow; findings=%v", report.Verdict, report.Findings)
	}
}

func TestPreflightBlocksHomeScan(t *testing.T) {
	report := Preflight(PreflightOptions{
		Command:      []string{"find", "~"},
		Platform:     healthyPlatform(),
		LoadProvider: func() (float64, float64, error) { return 1, 1, nil },
		IgnoreChecks: []string{"Kernel Panics (7d)", "Jetsam Events (7d)", "App Crashes (7d)"},
	})
	if report.Verdict != VerdictBlock {
		t.Fatalf("verdict = %s, want block; findings=%v", report.Verdict, report.Findings)
	}
}

func TestPreflightBlocksCodexSessionCat(t *testing.T) {
	report := Preflight(PreflightOptions{
		Command:      []string{"cat", "/Users/me/.codex/sessions/2026/05/example.jsonl"},
		Platform:     healthyPlatform(),
		LoadProvider: func() (float64, float64, error) { return 1, 1, nil },
		IgnoreChecks: []string{"Kernel Panics (7d)", "Jetsam Events (7d)", "App Crashes (7d)"},
	})
	if report.Verdict != VerdictBlock {
		t.Fatalf("verdict = %s, want block; findings=%v", report.Verdict, report.Findings)
	}
}

func TestPreflightWarnsOnHighLoad(t *testing.T) {
	report := Preflight(PreflightOptions{
		Command:        []string{"rg", "--files", "."},
		Platform:       healthyPlatform(),
		LoadProvider:   func() (float64, float64, error) { return 100, 100, nil },
		HealthProvider: healthyDoctor,
	})
	if report.Verdict != VerdictWarn {
		t.Fatalf("verdict = %s, want warn; findings=%v", report.Verdict, report.Findings)
	}
}

func TestSafeRunBlocksHazardousCommand(t *testing.T) {
	result, err := SafeRun(context.Background(), RunOptions{
		Command:      []string{"python3", "scan.py", "/Users/me/Development"},
		Platform:     healthyPlatform(),
		LoadProvider: func() (float64, float64, error) { return 1, 1, nil },
		// Host crash logs are intentionally ignored here; this test covers command policy.
		// Real CLI preflight still reports recent Jetsam and panic findings.
		IgnoreChecks: []string{"Kernel Panics (7d)", "Jetsam Events (7d)", "App Crashes (7d)"},
	})
	if err == nil {
		t.Fatal("expected block error")
	}
	if result == nil || result.ExitCode != 126 || result.Report.Verdict != VerdictBlock {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestSafeRunTruncatesOutput(t *testing.T) {
	result, err := SafeRun(context.Background(), RunOptions{
		Command:        []string{"printf", strings.Repeat("x", 128)},
		Platform:       healthyPlatform(),
		LoadProvider:   func() (float64, float64, error) { return 1, 1, nil },
		HealthProvider: healthyDoctor,
		MaxOutputBytes: 32,
		MaxOutputLines: 10,
	})
	if err != nil {
		t.Fatalf("safe run failed: %v", err)
	}
	if !result.Truncated {
		t.Fatalf("expected truncated result: %#v", result)
	}
	if result.FilteredBytes > 128 {
		t.Fatalf("filtered output grew unexpectedly: %#v", result)
	}
}
