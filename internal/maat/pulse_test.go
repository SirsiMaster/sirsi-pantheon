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

	// Create minimal project structure
	cmdDir := filepath.Join(tmpDir, "cmd", "sirsi")
	os.MkdirAll(cmdDir, 0o755)
	for _, deity := range []string{"anubis.go", "maat.go", "thoth.go", "hapi.go", "seba.go", "seshat.go"} {
		os.WriteFile(filepath.Join(cmdDir, deity), []byte("package main"), 0o644)
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
	if m.SourceLines != 32825 {
		t.Errorf("expected source lines 32825, got %d", m.SourceLines)
	}
	if m.SourceFiles != 250 {
		t.Errorf("expected 250 source files, got %d", m.SourceFiles)
	}
	if m.GoSourceLines != 17110 {
		t.Errorf("expected 17110 Go source lines, got %d", m.GoSourceLines)
	}
	if m.Deities != 6 {
		t.Errorf("expected 6 deities, got %d", m.Deities)
	}
	if m.Modules != 5 {
		t.Errorf("expected 5 modules, got %d", m.Modules)
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

	tmpDir := t.TempDir()
	cmdDir := filepath.Join(tmpDir, "cmd", "sirsi")
	os.MkdirAll(cmdDir, 0o755)

	// Create exactly 3 deity files
	for _, f := range []string{"anubis.go", "maat.go", "thoth.go"} {
		os.WriteFile(filepath.Join(cmdDir, f), []byte("package main"), 0o644)
	}

	count := countDeities(tmpDir)
	if count != 3 {
		t.Errorf("expected 3 deities, got %d", count)
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
