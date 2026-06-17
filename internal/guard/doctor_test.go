package guard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

// ── Helpers ──────────────────────────────────────────────────────────────

// healthyMock returns a Mock platform simulating a healthy macOS system:
// 16 GB RAM, low usage, no swap, 50% disk, small processes, no sirsi procs.
func healthyMock() *platform.Mock {
	return &platform.Mock{
		NameStr: "mock",
		CommandResults: map[string]string{
			// 16 GB total RAM
			"sysctl -n hw.memsize": "17179869184",

			// vm_stat: ~30% used (active + wired)
			// page size 16384
			// active:  200000 pages  = 3.2 GB
			// wired:   100000 pages  = 1.6 GB  => total used ~4.8 GB / 16 GB = 30%
			// free:    400000 pages
			// compressed: 50000 pages
			"vm_stat": `Mach Virtual Memory Statistics: (page size of 16384 bytes)
Pages free:                              400000.
Pages active:                            200000.
Pages inactive:                          100000.
Pages speculative:                        50000.
Pages throttled:                              0.
Pages wired down:                        100000.
Pages purgeable:                          20000.
"Translation faults":                  12345678.
Pages copy-on-write:                    1234567.
Pages zero filled:                      9876543.
Pages reactivated:                        12345.
Pages purged:                              6789.
File-backed pages:                       150000.
Anonymous pages:                         200000.
Pages stored in compressor:               50000.
Pages occupied by compressor:             25000.`,

			// No swap
			"sysctl -n vm.swapusage": "total = 0.00M  used = 0.00M  free = 0.00M  (encrypted)",

			// Disk at 50%
			"df -h /": `Filesystem     Size   Used  Avail Capacity  iused ifree %iused  Mounted on
/dev/disk3s1  460Gi  230Gi  230Gi    50%  1234567 9876543    11%   /`,

			// ps for getProcessListWith (used by checkTopMemoryProcesses)
			"ps -axo pid,rss,vsz,%cpu,user,comm": `  PID   RSS    VSZ  %CPU USER     COMM
  100  51200  102400  1.0 user     /usr/bin/node
  200  30720   61440  0.5 user     /usr/local/bin/gopls
  300  20480   40960  0.2 user     /Applications/Safari.app/Contents/MacOS/Safari
  400  10240   20480  0.1 user     /usr/bin/vim
  500   5120   10240  0.0 user     /bin/zsh`,

			// ps for checkSirsiProcesses
			"ps -axo pid,rss,comm": `  PID   RSS COMM
  100  51200 /usr/bin/node
  200  30720 /usr/local/bin/gopls`,
		},
	}
}

// ── TestDoctorWith_HealthySystem ─────────────────────────────────────────

func TestDoctorWith_HealthySystem(t *testing.T) {
	m := healthyMock()
	report, err := DoctorWith(m)
	if err != nil {
		t.Fatalf("DoctorWith() error = %v", err)
	}

	// Note: checkRecentCrashLogs reads the real filesystem (not mocked),
	// so the score and severity of crash/jetsam findings depend on the host.
	// We only assert on the mock-controlled checks.

	// RAM should be OK
	ramFinding := findByCheck(report.Findings, "RAM Pressure")
	if ramFinding == nil {
		t.Fatal("missing RAM Pressure finding")
	}
	if ramFinding.Severity != SeverityOK {
		t.Errorf("RAM Pressure severity = %v, want OK", ramFinding.Severity)
	}

	// Swap should be OK
	swapFinding := findByCheck(report.Findings, "Swap Usage")
	if swapFinding == nil {
		t.Fatal("missing Swap Usage finding")
	}
	if swapFinding.Severity != SeverityOK {
		t.Errorf("Swap Usage severity = %v, want OK", swapFinding.Severity)
	}
	if !strings.Contains(swapFinding.Message, "no memory pressure") {
		t.Errorf("Swap message = %q, want 'no memory pressure' substring", swapFinding.Message)
	}

	// Disk should be OK
	diskFinding := findByCheck(report.Findings, "Disk Space")
	if diskFinding == nil {
		t.Fatal("missing Disk Space finding")
	}
	if diskFinding.Severity != SeverityOK {
		t.Errorf("Disk Space severity = %v, want OK", diskFinding.Severity)
	}

	// Sirsi processes should be Info (none running)
	sirsiFinding := findByCheck(report.Findings, "Sirsi Processes")
	if sirsiFinding == nil {
		t.Fatal("missing Sirsi Processes finding")
	}
	if sirsiFinding.Severity != SeverityInfo {
		t.Errorf("Sirsi Processes severity = %v, want Info", sirsiFinding.Severity)
	}

	// Duration should be populated
	if report.Duration == "" {
		t.Error("Duration is empty")
	}
}

// ── TestDoctorWith_HighRAMPressure ───────────────────────────────────────

func TestDoctorWith_HighRAMPressure(t *testing.T) {
	m := healthyMock()

	// Override vm_stat: active + wired > 90% of 16 GB
	// 16 GB = 17179869184 bytes / 16384 page_size = 1048576 total pages
	// active 600000 pages = 9.83 GB, wired 400000 pages = 6.55 GB => 16.38 GB / 16 GB = ~96%
	m.CommandResults["vm_stat"] = `Mach Virtual Memory Statistics: (page size of 16384 bytes)
Pages free:                               10000.
Pages active:                            600000.
Pages inactive:                           20000.
Pages speculative:                         5000.
Pages throttled:                              0.
Pages wired down:                        400000.
Pages purgeable:                           1000.
"Translation faults":                  12345678.
Pages copy-on-write:                    1234567.
Pages zero filled:                      9876543.
Pages reactivated:                        12345.
Pages purged:                              6789.
File-backed pages:                       150000.
Anonymous pages:                         200000.
Pages stored in compressor:               80000.
Pages occupied by compressor:             40000.`

	report, err := DoctorWith(m)
	if err != nil {
		t.Fatalf("DoctorWith() error = %v", err)
	}

	ramFinding := findByCheck(report.Findings, "RAM Pressure")
	if ramFinding == nil {
		t.Fatal("missing RAM Pressure finding")
	}
	if ramFinding.Severity != SeverityCritical {
		t.Errorf("RAM Pressure severity = %v, want CRITICAL", ramFinding.Severity)
	}
	if !strings.Contains(ramFinding.Message, "critically high") {
		t.Errorf("RAM message = %q, want 'critically high' substring", ramFinding.Message)
	}

	// Score should be penalized
	if report.Score > 80 {
		t.Errorf("high RAM pressure score = %d, want <= 80", report.Score)
	}
}

// ── TestDoctorWith_SwapActive ────────────────────────────────────────────

func TestDoctorWith_SwapActive(t *testing.T) {
	tests := []struct {
		name          string
		swapOutput    string
		wantSeverity  DiagnosticSeverity
		wantSubstring string
	}{
		{
			// A few hundred MB of swap with healthy RAM is routine, NOT pressure.
			name:          "minimal swap (256 MB) is healthy",
			swapOutput:    "total = 512.00M  used = 256.00M  free = 256.00M  (encrypted)",
			wantSeverity:  SeverityOK,
			wantSubstring: "no memory pressure",
		},
		{
			name:          "moderate swap (2 GB) warns",
			swapOutput:    "total = 4096.00M  used = 2048.00M  free = 2048.00M  (encrypted)",
			wantSeverity:  SeverityWarn,
			wantSubstring: "under memory pressure",
		},
		{
			name:          "heavy swap (6 GB) is critical thrashing",
			swapOutput:    "total = 12288.00M  used = 6144.00M  free = 6144.00M  (encrypted)",
			wantSeverity:  SeverityCritical,
			wantSubstring: "thrashing",
		},
		{
			name:          "no swap",
			swapOutput:    "total = 0.00M  used = 0.00M  free = 0.00M  (encrypted)",
			wantSeverity:  SeverityOK,
			wantSubstring: "no memory pressure",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := healthyMock()
			m.CommandResults["sysctl -n vm.swapusage"] = tt.swapOutput

			report, err := DoctorWith(m)
			if err != nil {
				t.Fatalf("DoctorWith() error = %v", err)
			}

			swapFinding := findByCheck(report.Findings, "Swap Usage")
			if swapFinding == nil {
				t.Fatal("missing Swap Usage finding")
			}
			if swapFinding.Severity != tt.wantSeverity {
				t.Errorf("Swap severity = %v, want %v", swapFinding.Severity, tt.wantSeverity)
			}
			if !strings.Contains(swapFinding.Message, tt.wantSubstring) {
				t.Errorf("Swap message = %q, want %q substring", swapFinding.Message, tt.wantSubstring)
			}
		})
	}
}

// ── TestDoctorWith_DiskFull ──────────────────────────────────────────────

func TestDoctorWith_DiskFull(t *testing.T) {
	tests := []struct {
		name         string
		dfOutput     string
		wantSeverity DiagnosticSeverity
		wantSubstr   string
	}{
		{
			name: "critically full 97%",
			dfOutput: `Filesystem     Size   Used  Avail Capacity  iused ifree %iused  Mounted on
/dev/disk3s1  460Gi  447Gi   13Gi    97%  1234567 9876543    11%   /`,
			wantSeverity: SeverityCritical,
			wantSubstr:   "critically full",
		},
		{
			name: "high usage 90%",
			dfOutput: `Filesystem     Size   Used  Avail Capacity  iused ifree %iused  Mounted on
/dev/disk3s1  460Gi  414Gi   46Gi    90%  1234567 9876543    11%   /`,
			wantSeverity: SeverityWarn,
			wantSubstr:   "high",
		},
		{
			name: "healthy 50%",
			dfOutput: `Filesystem     Size   Used  Avail Capacity  iused ifree %iused  Mounted on
/dev/disk3s1  460Gi  230Gi  230Gi    50%  1234567 9876543    11%   /`,
			wantSeverity: SeverityOK,
			wantSubstr:   "healthy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := healthyMock()
			m.CommandResults["df -h /"] = tt.dfOutput

			report, err := DoctorWith(m)
			if err != nil {
				t.Fatalf("DoctorWith() error = %v", err)
			}

			diskFinding := findByCheck(report.Findings, "Disk Space")
			if diskFinding == nil {
				t.Fatal("missing Disk Space finding")
			}
			if diskFinding.Severity != tt.wantSeverity {
				t.Errorf("Disk severity = %v, want %v", diskFinding.Severity, tt.wantSeverity)
			}
			if !strings.Contains(diskFinding.Message, tt.wantSubstr) {
				t.Errorf("Disk message = %q, want %q substring", diskFinding.Message, tt.wantSubstr)
			}
		})
	}
}

// ── TestCalculateScore ───────────────────────────────────────────────────

func TestParseCrashReportName(t *testing.T) {
	tests := []struct {
		name     string
		wantProc string
		wantOK   bool
	}{
		{"Google Chrome-2026-06-03-163345.ips", "Google Chrome", true},
		{"chrome-headless-shell-2026-06-02-210739.ips", "chrome-headless-shell", true},
		{"Code Helper (Renderer)-2026-06-01-090046.ips", "Code Helper (Renderer)", true},
		{"chrome_crashpad_handler-2026-06-03-164451.0004.ips", "chrome_crashpad_handler", true},
		{"garbage.ips", "", false},
		{"NoDate-file.ips", "", false},
	}
	for _, tt := range tests {
		proc, _, ok := parseCrashReportName(tt.name)
		if ok != tt.wantOK || (ok && proc != tt.wantProc) {
			t.Errorf("parseCrashReportName(%q) = (%q, %v), want (%q, %v)", tt.name, proc, ok, tt.wantProc, tt.wantOK)
		}
	}
}

func TestDetectCrashloop(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name        string
		crashes     []appCrash
		wantLooping bool
		wantProc    string
	}{
		{
			name: "three within 5min, recent, is a loop",
			crashes: []appCrash{
				{"Chrome", now.Add(-4 * time.Minute)},
				{"Chrome", now.Add(-3 * time.Minute)},
				{"Chrome", now.Add(-2 * time.Minute)},
			},
			wantLooping: true,
			wantProc:    "Chrome",
		},
		{
			name: "three recent but spread >5min apart is not a loop",
			crashes: []appCrash{
				{"Chrome", now.Add(-25 * time.Minute)},
				{"Chrome", now.Add(-15 * time.Minute)},
				{"Chrome", now.Add(-5 * time.Minute)},
			},
			wantLooping: false,
		},
		{
			name: "three within 5min but days old is not an ACTIVE loop",
			crashes: []appCrash{
				{"Chrome", now.AddDate(0, 0, -2)},
				{"Chrome", now.AddDate(0, 0, -2).Add(time.Minute)},
				{"Chrome", now.AddDate(0, 0, -2).Add(2 * time.Minute)},
			},
			wantLooping: false,
		},
		{
			name:        "two is below threshold",
			crashes:     []appCrash{{"Chrome", now.Add(-3 * time.Minute)}, {"Chrome", now.Add(-2 * time.Minute)}},
			wantLooping: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proc, _, looping := detectCrashloop(tt.crashes, now)
			if looping != tt.wantLooping {
				t.Fatalf("detectCrashloop looping = %v, want %v", looping, tt.wantLooping)
			}
			if looping && proc != tt.wantProc {
				t.Errorf("detectCrashloop proc = %q, want %q", proc, tt.wantProc)
			}
		})
	}
}

func TestCheckAppCrashes(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	// One recent Chrome crash, one stale (8d ago, excluded by cutoff), one
	// Jetsam (excluded by name — counted by the kernel/Jetsam check instead).
	mustWrite(t, dir, "Google Chrome-"+now.Format("2006-01-02-150405")+".ips")
	mustWrite(t, dir, "JetsamEvent-"+now.Format("2006-01-02-150405")+".ips")
	stale := now.AddDate(0, 0, -8)
	staleName := filepath.Join(dir, "OldApp-"+stale.Format("2006-01-02-150405")+".ips")
	mustWrite(t, dir, filepath.Base(staleName))
	if err := os.Chtimes(staleName, stale, stale); err != nil {
		t.Fatal(err)
	}

	old := appCrashDirsFn
	appCrashDirsFn = func() []string { return []string{dir} }
	defer func() { appCrashDirsFn = old }()

	report := &DoctorReport{}
	checkAppCrashes(report)
	if len(report.Findings) != 1 {
		t.Fatalf("want 1 finding, got %d", len(report.Findings))
	}
	f := report.Findings[0]
	if f.Severity != SeverityWarn {
		t.Errorf("severity = %v, want Warn", f.Severity)
	}
	if !strings.Contains(f.Message, "Google Chrome") {
		t.Errorf("message %q should name the crashing process", f.Message)
	}
}

func TestScanAppCrashes_DedupFragments(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().Format("2006-01-02-150405")
	// 5 crashpad fragments of ONE event (same process, same second) must collapse
	// to a single crash — otherwise they masquerade as a crashloop.
	for _, suffix := range []string{"", ".000", ".0002", ".0003", ".0004"} {
		mustWrite(t, dir, "chrome_crashpad_handler-"+now+suffix+".ips")
	}
	crashes := scanAppCrashes([]string{dir}, time.Now().AddDate(0, 0, -7))
	if len(crashes) != 1 {
		t.Fatalf("want 1 deduped crash, got %d", len(crashes))
	}
	if _, _, looping := detectCrashloop(crashes, time.Now()); looping {
		t.Error("single fragmented event must not register as a crashloop")
	}
}

func mustWrite(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCalculateScore(t *testing.T) {
	tests := []struct {
		name     string
		findings []DiagnosticFinding
		want     int
	}{
		{
			name:     "no findings",
			findings: nil,
			want:     100,
		},
		{
			name: "all OK",
			findings: []DiagnosticFinding{
				{Severity: SeverityOK},
				{Severity: SeverityOK},
				{Severity: SeverityOK},
			},
			want: 100,
		},
		{
			name: "one info (no penalty)",
			findings: []DiagnosticFinding{
				{Severity: SeverityInfo},
			},
			want: 100,
		},
		{
			name: "one warn",
			findings: []DiagnosticFinding{
				{Severity: SeverityWarn},
			},
			want: 94, // 100 - 6
		},
		{
			name: "one trend critical (historical, not live)",
			findings: []DiagnosticFinding{
				{Check: "Jetsam Events (7d)", Severity: SeverityCritical},
			},
			want: 92, // 100 - 8 (a 7-day pattern, not catastrophic)
		},
		{
			name: "one LIVE critical (act-now)",
			findings: []DiagnosticFinding{
				{Check: "RAM Pressure", Severity: SeverityCritical, Message: "critically high"},
			},
			want: 75, // 100 - 25
		},
		{
			name: "mixed severities",
			findings: []DiagnosticFinding{
				{Severity: SeverityOK},
				{Severity: SeverityInfo},
				{Severity: SeverityWarn},
				{Check: "Jetsam Events (7d)", Severity: SeverityCritical},
			},
			want: 86, // 100 - 0(OK) - 0(Info) - 6(Warn) - 8(trend-crit)
		},
		{
			name: "floors at zero (many live criticals)",
			findings: []DiagnosticFinding{
				{Check: "RAM Pressure", Severity: SeverityCritical, Message: "critically high"},
				{Check: "Disk Space", Severity: SeverityCritical, Message: "full"},
				{Check: "RAM Pressure", Severity: SeverityCritical, Message: "critically high"},
				{Check: "Disk Space", Severity: SeverityCritical, Message: "full"},
				{Check: "RAM Pressure", Severity: SeverityCritical, Message: "critically high"}, // 5 * 25 = 125
			},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateScore(tt.findings)
			if got != tt.want {
				t.Errorf("calculateScore() = %d, want %d", got, tt.want)
			}
		})
	}
}

// ── TestDiagnosticSeverity_Icon ──────────────────────────────────────────

func TestDiagnosticSeverity_Icon(t *testing.T) {
	tests := []struct {
		severity DiagnosticSeverity
		want     string
	}{
		{SeverityOK, "🟢"},
		{SeverityInfo, "🔵"},
		{SeverityWarn, "🟡"},
		{SeverityCritical, "🔴"},
		{DiagnosticSeverity(99), "⚪"}, // unknown
	}

	for _, tt := range tests {
		t.Run(tt.severity.String(), func(t *testing.T) {
			got := tt.severity.Icon()
			if got != tt.want {
				t.Errorf("DiagnosticSeverity(%d).Icon() = %q, want %q", tt.severity, got, tt.want)
			}
		})
	}
}

// ── TestDiagnosticSeverity_String ────────────────────────────────────────

func TestDiagnosticSeverity_String(t *testing.T) {
	tests := []struct {
		severity DiagnosticSeverity
		want     string
	}{
		{SeverityOK, "OK"},
		{SeverityInfo, "INFO"},
		{SeverityWarn, "WARN"},
		{SeverityCritical, "CRITICAL"},
		{DiagnosticSeverity(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.severity.String()
			if got != tt.want {
				t.Errorf("DiagnosticSeverity(%d).String() = %q, want %q", tt.severity, got, tt.want)
			}
		})
	}
}

// ── TestDoctorWith_MemoryHog ─────────────────────────────────────────────

func TestDoctorWith_MemoryHog(t *testing.T) {
	m := healthyMock()

	// Inject a process using > 4 GB RSS (4194304 KB = 4 GB in KB for ps output)
	m.CommandResults["ps -axo pid,rss,vsz,%cpu,user,comm"] = `  PID   RSS    VSZ  %CPU USER     COMM
  100  5242880  10485760  5.0 user     /usr/bin/node
  200  30720   61440  0.5 user     /usr/local/bin/gopls`

	report, err := DoctorWith(m)
	if err != nil {
		t.Fatalf("DoctorWith() error = %v", err)
	}

	memFinding := findByCheck(report.Findings, "Top Memory Consumers")
	if memFinding == nil {
		t.Fatal("missing Top Memory Consumers finding")
	}
	if memFinding.Severity != SeverityWarn {
		t.Errorf("Top Memory severity = %v, want WARN for >4GB process", memFinding.Severity)
	}
	if !strings.Contains(memFinding.Message, "Memory hog") {
		t.Errorf("message = %q, want 'Memory hog' substring", memFinding.Message)
	}
}

// ── TestDoctorWith_SirsiProcesses ────────────────────────────────────────

func TestDoctorWith_SirsiProcesses(t *testing.T) {
	m := healthyMock()

	// Inject sirsi processes in ps output
	m.CommandResults["ps -axo pid,rss,comm"] = `  PID   RSS COMM
  100  51200 /usr/bin/node
  900  20480 /usr/local/bin/sirsi-agent
  901  10240 /usr/local/bin/sirsi-guard`

	report, err := DoctorWith(m)
	if err != nil {
		t.Fatalf("DoctorWith() error = %v", err)
	}

	sirsiFinding := findByCheck(report.Findings, "Sirsi Processes")
	if sirsiFinding == nil {
		t.Fatal("missing Sirsi Processes finding")
	}
	if sirsiFinding.Severity != SeverityOK {
		t.Errorf("Sirsi severity = %v, want OK for small processes", sirsiFinding.Severity)
	}
	if !strings.Contains(sirsiFinding.Message, "2 Sirsi process") {
		t.Errorf("message = %q, want '2 Sirsi process' substring", sirsiFinding.Message)
	}
}

// ── TestDoctorWith_WarnRAM ───────────────────────────────────────────────

func TestDoctorWith_WarnRAM(t *testing.T) {
	m := healthyMock()

	// Set RAM usage to ~80% (between 75-90 triggers WARN)
	// active 400000 + wired 400000 = 800000 pages * 16384 = 13.1 GB / 16 GB = ~82%
	m.CommandResults["vm_stat"] = `Mach Virtual Memory Statistics: (page size of 16384 bytes)
Pages free:                               50000.
Pages active:                            400000.
Pages inactive:                           20000.
Pages speculative:                         5000.
Pages throttled:                              0.
Pages wired down:                        400000.
Pages purgeable:                           1000.
"Translation faults":                  12345678.
Pages copy-on-write:                    1234567.
Pages zero filled:                      9876543.
Pages reactivated:                        12345.
Pages purged:                              6789.
File-backed pages:                       150000.
Anonymous pages:                         200000.
Pages stored in compressor:               50000.
Pages occupied by compressor:             25000.`

	report, err := DoctorWith(m)
	if err != nil {
		t.Fatalf("DoctorWith() error = %v", err)
	}

	ramFinding := findByCheck(report.Findings, "RAM Pressure")
	if ramFinding == nil {
		t.Fatal("missing RAM Pressure finding")
	}
	if ramFinding.Severity != SeverityWarn {
		t.Errorf("RAM severity = %v, want WARN for ~80%% usage", ramFinding.Severity)
	}
	if !strings.Contains(ramFinding.Message, "elevated") {
		t.Errorf("RAM message = %q, want 'elevated' substring", ramFinding.Message)
	}
}

// ── helpers ──────────────────────────────────────────────────────────────

func findByCheck(findings []DiagnosticFinding, check string) *DiagnosticFinding {
	for i := range findings {
		if findings[i].Check == check {
			return &findings[i]
		}
	}
	return nil
}

func TestClassifyEventTrend(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	day := func(d int) time.Time { return now.AddDate(0, 0, -d) }

	tests := []struct {
		name      string
		times     []time.Time
		wantCount int
		wantDays  int
		wantTrend bool
	}{
		{"none", nil, 0, 0, false},
		{
			"transient spike — 5 events, all one day",
			[]time.Time{day(1), day(1).Add(time.Hour), day(1).Add(2 * time.Hour), day(1).Add(3 * time.Hour), day(1).Add(4 * time.Hour)},
			5, 1, false,
		},
		{
			"trend — 3 events across 3 days",
			[]time.Time{day(1), day(3), day(5)},
			3, 3, true,
		},
		{
			"out-of-window events excluded",
			[]time.Time{day(1), day(8), day(30)},
			1, 1, false,
		},
		{
			"future events ignored",
			[]time.Time{now.AddDate(0, 0, 1), day(2)},
			1, 1, false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, days, trend := classifyEventTrend(tt.times, now)
			if count != tt.wantCount || days != tt.wantDays || trend != tt.wantTrend {
				t.Errorf("got (count=%d days=%d trend=%v), want (count=%d days=%d trend=%v)",
					count, days, trend, tt.wantCount, tt.wantDays, tt.wantTrend)
			}
		})
	}
}

func TestCheckSpotlightStorm(t *testing.T) {
	psHeader := "  PID   RSS    VSZ  %CPU USER     COMM\n"

	tests := []struct {
		name     string
		ps       string
		wantSev  DiagnosticSeverity
		wantWord string
	}{
		{
			name:     "storm — mds_stores + mdworker pinning CPU",
			ps:       psHeader + "  50 102400 204800 38.0 user /System/Library/Frameworks/mds_stores\n  51  51200 102400 12.0 user mdworker_shared\n 100  20480  40960  1.0 user /usr/bin/node",
			wantSev:  SeverityWarn,
			wantWord: "Spotlight indexer busy",
		},
		{
			name:     "idle — mds_stores near zero",
			ps:       psHeader + "  50 102400 204800  0.2 user mds_stores\n 100  20480  40960  1.0 user /usr/bin/node",
			wantSev:  SeverityOK,
			wantWord: "idle",
		},
		{
			name:     "no spotlight processes at all",
			ps:       psHeader + " 100  20480  40960  1.0 user /usr/bin/node",
			wantSev:  SeverityOK,
			wantWord: "idle",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := healthyMock()
			m.CommandResults["ps -axo pid,rss,vsz,%cpu,user,comm"] = tt.ps

			report := &DoctorReport{}
			checkSpotlightStorm(m, report)
			f := findByCheck(report.Findings, "Spotlight Storm")
			if f == nil {
				t.Fatal("missing Spotlight Storm finding")
			}
			if f.Severity != tt.wantSev {
				t.Errorf("severity = %v, want %v (msg: %s)", f.Severity, tt.wantSev, f.Message)
			}
			if !strings.Contains(f.Message, tt.wantWord) {
				t.Errorf("message %q should contain %q", f.Message, tt.wantWord)
			}
		})
	}
}

func TestCheckRecentCrashLogs_TransientVsTrend(t *testing.T) {
	now := time.Now()
	day := func(d int) time.Time { return now.AddDate(0, 0, -d) }

	old := crashEventScanFn
	defer func() { crashEventScanFn = old }()

	// Jetsam clustered in one day = transient → Warn, not a trend.
	// Panics across 3 days = sustained → Critical trend.
	crashEventScanFn = func() (panics, jetsams []time.Time) {
		jetsams = []time.Time{day(1), day(1).Add(time.Hour), day(1).Add(2 * time.Hour)}
		panics = []time.Time{day(1), day(2), day(4)}
		return panics, jetsams
	}

	report := &DoctorReport{}
	checkRecentCrashLogs(report)

	jet := findByCheck(report.Findings, "Jetsam Events (7d)")
	if jet == nil {
		t.Fatal("missing Jetsam finding")
	}
	if jet.Severity != SeverityWarn || jet.Trend {
		t.Errorf("transient jetsam: got severity=%v trend=%v, want Warn/false", jet.Severity, jet.Trend)
	}
	if !strings.Contains(jet.Message, "transient") {
		t.Errorf("transient jetsam message %q should say transient", jet.Message)
	}

	pan := findByCheck(report.Findings, "Kernel Panics (7d)")
	if pan == nil {
		t.Fatal("missing panic finding")
	}
	if pan.Severity != SeverityCritical || !pan.Trend {
		t.Errorf("trend panics: got severity=%v trend=%v, want Critical/true", pan.Severity, pan.Trend)
	}
	if !strings.Contains(pan.Message, "sustained trend") {
		t.Errorf("trend panic message %q should say sustained trend", pan.Message)
	}
}

func TestCheckRecentCrashLogs_NoneIsHealthy(t *testing.T) {
	old := crashEventScanFn
	defer func() { crashEventScanFn = old }()
	crashEventScanFn = func() (panics, jetsams []time.Time) { return nil, nil }

	report := &DoctorReport{}
	checkRecentCrashLogs(report)
	for _, f := range report.Findings {
		if f.Severity != SeverityOK {
			t.Errorf("%s: severity=%v, want OK on a clean host", f.Check, f.Severity)
		}
	}
}

func TestIsHangReport(t *testing.T) {
	yes := []string{
		"WindowServer_2026-06-16-120000_Mac.hang",
		"Dock-2026-06-16-120000.spin",
		"spotlightknowledged_2026-06-14-095112_MacBook-Pro-2.cpu_resource.diag",
	}
	no := []string{
		"Chrome-2026-06-16-120000.ips",      // crash, counted elsewhere
		"Kernel-2026-06-16.panic",           // panic, counted elsewhere
		"JetsamEvent-2026-06-16-120000.ips", // jetsam, counted elsewhere
	}
	for _, n := range yes {
		if !isHangReport(n) {
			t.Errorf("isHangReport(%q) = false, want true", n)
		}
	}
	for _, n := range no {
		if isHangReport(n) {
			t.Errorf("isHangReport(%q) = true, want false", n)
		}
	}
}

func TestHangReportProcess(t *testing.T) {
	cases := map[string]string{
		"spotlightknowledged_2026-06-14-095112_MacBook-Pro-2.cpu_resource.diag": "spotlightknowledged",
		"WindowServer_2026-06-16-120000_Mac.hang":                               "WindowServer",
		"Google Chrome Helper-2026-06-16-120000.spin":                           "Google Chrome Helper",
		"FPCKService_2026-06-14-135557_MacBook-Pro-2.cpu_resource.diag":         "FPCKService",
	}
	for name, want := range cases {
		if got := hangReportProcess(name); got != want {
			t.Errorf("hangReportProcess(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestCheckAppHangs_TransientVsTrend(t *testing.T) {
	now := time.Now()
	day := func(d int) time.Time { return now.AddDate(0, 0, -d) }
	old := hangReportScanFn
	defer func() { hangReportScanFn = old }()

	// Spread across 6 distinct days (≥ trendDayThreshold) → sustained Critical
	// trend, with spotlightknowledged as the named worst offender.
	hangReportScanFn = func() (times []time.Time, byProcess map[string]int) {
		times = []time.Time{day(0), day(1), day(2), day(3), day(4), day(5)}
		byProcess = map[string]int{"spotlightknowledged": 4, "Chrome": 2}
		return times, byProcess
	}
	report := &DoctorReport{}
	checkAppHangs(report)
	f := findByCheck(report.Findings, "App Hangs (7d)")
	if f == nil {
		t.Fatal("missing App Hangs finding")
	}
	if f.Severity != SeverityCritical || !f.Trend {
		t.Errorf("trend hangs: got severity=%v trend=%v, want Critical/true", f.Severity, f.Trend)
	}
	if !strings.Contains(f.Message, "sustained main-thread saturation") {
		t.Errorf("trend message %q should name the saturation", f.Message)
	}
	if !strings.Contains(f.Detail, "spotlightknowledged ×4") {
		t.Errorf("detail %q should name the worst offender first", f.Detail)
	}

	// Clustered in a single day → transient Warn.
	hangReportScanFn = func() (times []time.Time, byProcess map[string]int) {
		times = []time.Time{day(1), day(1).Add(time.Hour), day(1).Add(2 * time.Hour)}
		byProcess = map[string]int{"Dock": 3}
		return times, byProcess
	}
	report = &DoctorReport{}
	checkAppHangs(report)
	f = findByCheck(report.Findings, "App Hangs (7d)")
	if f.Severity != SeverityWarn || f.Trend {
		t.Errorf("transient hangs: got severity=%v trend=%v, want Warn/false", f.Severity, f.Trend)
	}
}

func TestCheckAppHangs_NoneIsHealthy(t *testing.T) {
	old := hangReportScanFn
	defer func() { hangReportScanFn = old }()
	hangReportScanFn = func() (times []time.Time, byProcess map[string]int) {
		return nil, map[string]int{}
	}
	report := &DoctorReport{}
	checkAppHangs(report)
	f := findByCheck(report.Findings, "App Hangs (7d)")
	if f == nil || f.Severity != SeverityOK {
		t.Errorf("clean host: got %+v, want a single OK App Hangs finding", f)
	}
}

func TestClassifyHealth(t *testing.T) {
	crit := func(check, msg string) DiagnosticFinding {
		return DiagnosticFinding{Check: check, Severity: SeverityCritical, Message: msg}
	}
	cases := []struct {
		name string
		f    []DiagnosticFinding
		want HealthStatus
	}{
		{"all clean → green", []DiagnosticFinding{{Check: "RAM Pressure", Severity: SeverityOK}}, HealthGreen},
		{"only historical trends → amber", []DiagnosticFinding{crit("Jetsam Events (7d)", "12 kills"), crit("App Hangs (7d)", "saturation")}, HealthAmber},
		{"a warning → amber", []DiagnosticFinding{{Check: "Swap Usage", Severity: SeverityWarn}}, HealthAmber},
		{"live RAM critical → red", []DiagnosticFinding{crit("RAM Pressure", "critically high")}, HealthRed},
		{"disk full → red", []DiagnosticFinding{crit("Disk Space", "full")}, HealthRed},
		{"active crashloop → red", []DiagnosticFinding{crit("App Crashes (7d)", "Chrome crashloop detected")}, HealthRed},
	}
	for _, c := range cases {
		if got := classifyHealth(c.f); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestCalculateScore_TrendsDoNotZero(t *testing.T) {
	trends := []DiagnosticFinding{
		{Check: "Jetsam Events (7d)", Severity: SeverityCritical},
		{Check: "App Crashes (7d)", Severity: SeverityCritical},
		{Check: "App Hangs (7d)", Severity: SeverityCritical},
		{Check: "Kernel Panics (7d)", Severity: SeverityCritical},
	}
	if s := calculateScore(trends); s < 60 {
		t.Errorf("4 historical trend criticals scored %d, want >= 60 (trends are not catastrophic)", s)
	}
	live := []DiagnosticFinding{{Check: "RAM Pressure", Severity: SeverityCritical, Message: "critically high"}}
	if calculateScore(live) >= calculateScore(trends[:1]) {
		t.Errorf("a live-critical should score lower than a single trend-critical")
	}
}

func TestRemediationCommand(t *testing.T) {
	cases := []struct {
		check, detail string
		sev           DiagnosticSeverity
		want          string
	}{
		{"App Crashes (7d)", "", SeverityWarn, "sirsi clean"},
		{"App Hangs (7d)", "spotlightknowledged ×11", SeverityCritical, "sirsi spotlight-exclude ~/Development"},
		{"App Hangs (7d)", "Chrome ×4", SeverityWarn, "sirsi guard"},
		{"Jetsam Events (7d)", "", SeverityCritical, "sirsi guard"},
		{"Disk Space", "", SeverityCritical, "sirsi clean --include-caution"},
		{"Swap Usage", "", SeverityWarn, ""}, // warn swap → no one-click fix
		{"Swap Usage", "", SeverityCritical, "sirsi guard"},
		{"RAM Pressure", "", SeverityOK, ""}, // healthy → no fix
		{"Spotlight Storm", "", SeverityWarn, "sirsi spotlight-exclude ~/Development"},
		{"Thread Leaks", "", SeverityCritical, "sirsi guard"},
		{"Sirsi Processes", "", SeverityInfo, ""}, // informational → no fix
	}
	for _, c := range cases {
		f := DiagnosticFinding{Check: c.check, Detail: c.detail, Severity: c.sev}
		if got := remediationCommand(f); got != c.want {
			t.Errorf("remediationCommand(%q/%v) = %q, want %q", c.check, c.sev, got, c.want)
		}
	}
}
