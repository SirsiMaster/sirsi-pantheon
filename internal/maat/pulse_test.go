package maat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── Pulse Test Suite ────────────────────────────────────────────────
// Tests for the Ma'at Pulse dynamic measurement engine.
// All tests use injectable runners (Rule A21) — no live go test calls.

func TestPulse_WithMockRunners(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// The selected source roster is authoritative, including inline-command Isis.
	registryPath := filepath.Join(tmpDir, "internal", "deity", "registry.go")
	if err := os.MkdirAll(filepath.Dir(registryPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(registryPath, []byte(pulseRosterFixture), 0o644); err != nil {
		t.Fatal(err)
	}

	internalDir := filepath.Join(tmpDir, "internal")
	for _, mod := range []string{"cleaner", "guard", "ka", "mirror", "jackal"} {
		os.MkdirAll(filepath.Join(internalDir, mod), 0o755)
	}

	// Create a fake binary
	binPath := filepath.Join(tmpDir, "sirsi")
	os.WriteFile(binPath, make([]byte, 12*1024*1024), 0o755) // 12 MB

	cfg := &PulseConfig{
		ProjectRoot: tmpDir,
		OutputPath:  filepath.Join(tmpDir, ".pantheon", "metrics.json"),
		BinaryPath:  binPath,
		Version:     "v1.0.0-test",
		TestRunner: func(root string) (string, error) {
			return sampleTestOutput, nil
		},
		LineCounter: func(root string) (int, int, int, int, error) {
			return 32825, 250, 85, 17110, nil
		},
		BinarySizer: defaultBinarySizer,
	}

	m, err := Pulse(cfg)
	if err != nil {
		t.Fatalf("Pulse() error: %v", err)
	}

	// ── Assertions ──────────────────────────────────────────────
	if m.Tests == 0 {
		t.Error("expected non-zero test count")
	}
	if m.TestsPassed == 0 {
		t.Error("expected non-zero passed count")
	}
	if m.Coverage == 0 {
		t.Error("expected non-zero coverage")
	}
	if !m.TestsMeasured || !m.CoverageMeasured || m.TestMeasurement != "measured" {
		t.Errorf("expected measured test and coverage metrics, got tests=%t coverage=%t state=%q", m.TestsMeasured, m.CoverageMeasured, m.TestMeasurement)
	}
	if m.SourceLines != 32825 {
		t.Errorf("expected source lines 32825, got %d", m.SourceLines)
	}
	if m.SourceFiles != 250 {
		t.Errorf("expected 250 source files, got %d", m.SourceFiles)
	}
	if m.GoSourceLines != 17110 {
		t.Errorf("expected 17110 Go source lines, got %d", m.GoSourceLines)
	}
	if m.Deities != 10 {
		t.Errorf("expected 10 source roster declarations, got %d", m.Deities)
	}
	if m.Modules != 6 {
		t.Errorf("expected 6 immediate internal directories including deity, got %d", m.Modules)
	}
	if m.BinarySize != 12*1024*1024 {
		t.Errorf("expected 12 MB binary, got %d", m.BinarySize)
	}
	if m.BinarySizeHuman == "" {
		t.Error("expected non-empty binary size human")
	}
	if m.Version != "v1.0.0-test" {
		t.Errorf("expected version v1.0.0-test, got %s", m.Version)
	}
	if m.ElapsedMs < 0 {
		t.Error("elapsed should be non-negative")
	}
	if m.Timestamp == "" {
		t.Error("expected non-empty timestamp")
	}

	// ── Verify written file ─────────────────────────────────────
	data, err := os.ReadFile(cfg.OutputPath)
	if err != nil {
		t.Fatalf("metrics.json not written: %v", err)
	}

	var loaded PulseMetrics
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("invalid JSON in metrics.json: %v", err)
	}

	if loaded.Tests != m.Tests {
		t.Errorf("loaded tests %d != expected %d", loaded.Tests, m.Tests)
	}
	if loaded.Coverage != m.Coverage {
		t.Errorf("loaded coverage %.1f != expected %.1f", loaded.Coverage, m.Coverage)
	}
}

func TestPulse_SkipTests(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	cfg := &PulseConfig{
		ProjectRoot: tmpDir,
		SkipTests:   true,
		OutputPath:  filepath.Join(tmpDir, ".pantheon", "metrics.json"),
		Version:     "v1.0.0-test",
		LineCounter: func(root string) (int, int, int, int, error) {
			return 100, 10, 5, 80, nil
		},
	}

	m, err := Pulse(cfg)
	if err != nil {
		t.Fatalf("Pulse(skip) error: %v", err)
	}

	if m.Tests != 0 {
		t.Errorf("expected 0 tests when skipped, got %d", m.Tests)
	}
	if m.TestsMeasured || m.CoverageMeasured || m.TestMeasurement != "skipped" {
		t.Errorf("skipped tests must remain explicitly unmeasured, got tests=%t coverage=%t state=%q", m.TestsMeasured, m.CoverageMeasured, m.TestMeasurement)
	}
	if m.SourceLines != 100 {
		t.Errorf("expected 100 source lines, got %d", m.SourceLines)
	}
}

func TestPulse_NoBinary(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	cfg := &PulseConfig{
		ProjectRoot: tmpDir,
		SkipTests:   true,
		OutputPath:  filepath.Join(tmpDir, ".pantheon", "metrics.json"),
		Version:     "v1.0.0-test",
	}

	m, err := Pulse(cfg)
	if err != nil {
		t.Fatalf("Pulse(noBin) error: %v", err)
	}

	if m.BinarySize != 0 {
		t.Errorf("expected 0 binary size with no binary, got %d", m.BinarySize)
	}
}

func TestParseTestCounts(t *testing.T) {
	t.Parallel()

	passed, failed, skipped := parseTestCounts(sampleTestOutput)

	if passed < 5 {
		t.Errorf("expected at least 5 passed, got %d", passed)
	}
	if failed != 0 {
		t.Errorf("expected 0 failed, got %d", failed)
	}
	if skipped < 1 {
		t.Errorf("expected at least 1 skipped, got %d", skipped)
	}
}

func TestParseTotalCoverage(t *testing.T) {
	t.Parallel()

	cov := parseTotalCoverage(sampleTestOutput)
	if cov < 50 || cov > 100 {
		t.Errorf("expected coverage between 50-100%%, got %.1f%%", cov)
	}
}

func TestPulse_NoCoverageOutputRemainsUnmeasured(t *testing.T) {
	t.Parallel()

	cfg := &PulseConfig{
		ProjectRoot: t.TempDir(),
		OutputPath:  filepath.Join(t.TempDir(), "metrics.json"),
		TestRunner: func(string) (string, error) {
			return "=== RUN   TestExample\n--- PASS: TestExample (0.00s)\nPASS\n", nil
		},
	}

	m, err := Pulse(cfg)
	if err != nil {
		t.Fatalf("Pulse() error: %v", err)
	}
	if !m.TestsMeasured || m.CoverageMeasured || m.Coverage != 0 || m.TestMeasurement != "measured" {
		t.Errorf("coverage without a coverage record must be unmeasured, got tests=%t coverage=%t value=%.1f state=%q", m.TestsMeasured, m.CoverageMeasured, m.Coverage, m.TestMeasurement)
	}
}

func TestLoadMetrics(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "metrics.json")

	m := &PulseMetrics{
		Tests:    42,
		Coverage: 85.5,
		Version:  "v1.0.0",
	}

	if err := writeMetrics(path, m); err != nil {
		t.Fatalf("writeMetrics: %v", err)
	}

	loaded, err := LoadMetrics(path)
	if err != nil {
		t.Fatalf("LoadMetrics: %v", err)
	}

	if loaded.Tests != 42 {
		t.Errorf("expected 42 tests, got %d", loaded.Tests)
	}
	if loaded.Coverage != 85.5 {
		t.Errorf("expected 85.5 coverage, got %.1f", loaded.Coverage)
	}
}

func TestFormatBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{512, "512.0 B"},
		{1024, "1.0 KB"},
		{1048576, "1.0 MB"},
		{12582912, "12.0 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, tt := range tests {
		got := formatBytes(tt.input)
		if got != tt.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCountDeities(t *testing.T) {
	t.Parallel()

	const prefix = "package deity\ntype Info struct { Key, Glyph, Name, Role string }\n"
	cases := []struct {
		name   string
		source string
		want   int
	}{
		{"canonical roster with inline Isis", pulseRosterFixture, 10},
		{"selected historical subset", prefix + `var Roster = []Info{{"thoth", "", "", ""}, {"maat", "", "", ""}, {"isis", "", "", ""}}`, 3},
		{"keyed literals", prefix + `var Roster = []Info{{Name: "Isis", Key: "isis"}, {Key: "maat"}}`, 2},
		{"missing roster file", "", 0},
		{"empty file", " ", 0},
		{"malformed source", prefix + `var Roster = []Info{`, 0},
		{"empty roster", prefix + `var Roster = []Info{}`, 0},
		{"duplicate keys", prefix + `var Roster = []Info{{Key: "isis"}, {Key: "isis"}}`, 0},
		{"duplicate declarations", prefix + `var Roster = []Info{{Key: "isis"}}; var Roster = []Info{{Key: "maat"}}`, 0},
		{"nonliteral roster", prefix + `var Roster = loadRoster()`, 0},
		{"nonliteral key", prefix + `var Roster = []Info{{Key: key}}`, 0},
		{"empty key", prefix + `var Roster = []Info{{Key: ""}}`, 0},
		{"unrelated variable", prefix + `var Other = []Info{{Key: "isis"}}`, 0},
		{"local variable", prefix + `func f() { var Roster = []Info{{Key: "isis"}}; _ = Roster }`, 0},
		{"wrong roster type", prefix + `var Roster = []string{"isis"}`, 0},
		{"oversized source", strings.Repeat(" ", 256*1024) + pulseRosterFixture, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			// Command layout, legacy files, and unrelated files do not define the roster.
			cmdDir := filepath.Join(root, "cmd", "sirsi")
			if err := os.MkdirAll(cmdDir, 0o755); err != nil {
				t.Fatal(err)
			}
			for file, source := range map[string]string{
				"main.go":         "package main\nvar isisCmd = &cobra.Command{Use: \"isis\"}\nfunc init() { rootCmd.AddCommand(isisCmd) }\n",
				"hapi.go":         "package main",
				"unregistered.go": "package main",
			} {
				if err := os.WriteFile(filepath.Join(cmdDir, file), []byte(source), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if tc.source != "" {
				path := filepath.Join(root, "internal", "deity", "registry.go")
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(tc.source), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if got := countDeities(root); got != tc.want {
				t.Errorf("countDeities() = %d, want %d selected-source roster declarations", got, tc.want)
			}
		})
	}
	if got := countDeities(filepath.Join(t.TempDir(), "missing")); got != 0 {
		t.Errorf("missing root: countDeities() = %d, want 0", got)
	}
}

func TestCountModules(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	internalDir := filepath.Join(tmpDir, "internal")
	os.MkdirAll(internalDir, 0o755)

	for _, m := range []string{"cleaner", "guard", "ka"} {
		os.MkdirAll(filepath.Join(internalDir, m), 0o755)
	}

	// Also create a file (should not count)
	os.WriteFile(filepath.Join(internalDir, "README.md"), []byte("# internal"), 0o644)

	count := countModules(tmpDir)
	if count != 3 {
		t.Errorf("expected 3 modules, got %d", count)
	}
}

func TestDefaultMetricsPath(t *testing.T) {
	t.Parallel()

	path := DefaultMetricsPath("/home/user/project")
	if !strings.HasSuffix(path, ".pantheon/metrics.json") {
		t.Errorf("expected path ending with .pantheon/metrics.json, got %s", path)
	}
}

// ── Sample Test Output Fixture ──────────────────────────────────────

const sampleTestOutput = `=== RUN   TestCleanerSafety
--- PASS: TestCleanerSafety (0.00s)
=== RUN   TestGuardWatchdog
--- PASS: TestGuardWatchdog (0.01s)
=== RUN   TestKaGhostHunter
--- PASS: TestKaGhostHunter (0.00s)
=== RUN   TestMirrorDedup
--- PASS: TestMirrorDedup (0.02s)
=== RUN   TestJackalEngine
--- PASS: TestJackalEngine (0.00s)
=== RUN   TestPlatformSpecific
--- SKIP: TestPlatformSpecific (0.00s)
    platform_test.go:12: skipping on non-darwin
=== RUN   TestScalesPolicy
--- PASS: TestScalesPolicy (0.00s)
ok  	github.com/SirsiMaster/sirsi-pantheon/internal/cleaner	0.234s	coverage: 85.3% of statements
ok  	github.com/SirsiMaster/sirsi-pantheon/internal/guard	0.189s	coverage: 91.2% of statements
ok  	github.com/SirsiMaster/sirsi-pantheon/internal/ka	0.102s	coverage: 95.1% of statements
ok  	github.com/SirsiMaster/sirsi-pantheon/internal/mirror	0.345s	coverage: 88.7% of statements
ok  	github.com/SirsiMaster/sirsi-pantheon/internal/jackal	0.156s	coverage: 92.4% of statements
?   	github.com/SirsiMaster/sirsi-pantheon/internal/thoth	[no test files]
ok  	github.com/SirsiMaster/sirsi-pantheon/internal/scales	0.078s	coverage: 94.6% of statements
`

// Source fixture mirrors the canonical registry declaration, not command-file layout.
const pulseRosterFixture = `// Package deity provides the canonical deity registry for the Sirsi Pantheon.
// All surfaces (TUI, menubar, dashboard, CLI) import from here to ensure
// glyphs, names, and roles are consistent. See ADR-025 (Deity Registry).
package deity

// Info describes a single deity in the Pantheon hierarchy.
type Info struct {
	Key   string // lowercase identifier used in CLI args and state keys
	Glyph string // Egyptian hieroglyph for display
	Name  string // display name (PascalCase)
	Role  string // two-word role description
}

// Roster is the canonical deity list, ordered by hierarchy (Rule D6).
// Horus → Ra → Net → Thoth → Ma'at → Isis → Seshat → Anubis → Seba → Osiris
var Roster = []Info{
	{"horus", "𓂀", "Horus", "Workstation Lord"},
	{"ra", "𓇶", "Ra", "Fleet Orchestrator"},
	{"net", "𓁯", "Net", "Universal Weaver"},
	{"thoth", "𓁟", "Thoth", "Local Memory"},
	{"maat", "𓆄", "Ma'at", "Quality Gate"},
	{"isis", "𓁐", "Isis", "Health & Remedy"},
	{"seshat", "𓁆", "Seshat", "Local Knowledge"},
	{"anubis", "𓃣", "Anubis", "System Jackal"},
	{"seba", "𓇽", "Seba", "Infra & Hardware"},
	{"osiris", "𓁹", "Osiris", "State Keeper"},
}
`
