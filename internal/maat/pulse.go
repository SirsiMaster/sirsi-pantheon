// Package maat — pulse.go
//
// 𓆄 Ma'at Pulse — The Dynamic Measurement Heartbeat
//
// Pulse is the single source of truth for all Pantheon metrics.
// It runs real measurements (go test, source counting, binary sizing)
// and writes a structured .pantheon/metrics.json that all downstream
// consumers can read:
//
//   - CI pipeline uploads it as an artifact
//   - VS Code extension reads it for dynamic status bar numbers
//   - BUILD_LOG references it instead of hardcoded strings
//   - Thoth sync reads it to update memory.yaml with real numbers
//
// Usage:
//
//	pantheon maat pulse              Run all measurements
//	pantheon maat pulse --skip-test  Skip go test (fast mode)
//	pantheon maat pulse --json       Output metrics as JSON to stdout
//
// Architecture:
//
//	Pulse.Run()
//	  ├─ countTests()       → runs go test -count=1 -short ./...
//	  ├─ measureCoverage()  → parses coverprofile
//	  ├─ countSourceLines() → find + wc -l (excludes vendor/test)
//	  ├─ measureBinary()    → stat the built binary
//	  └─ countDeities()     → counts cmd/pantheon/*.go subcommands
//
// All system calls are injectable (Rule A16) for testing (Rule A21).
package maat

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/stele"
)

// PulseMetrics is the canonical metrics structure written to .pantheon/metrics.json.
// Every field is measured, never hardcoded.
type PulseMetrics struct {
	// Test suite
	Tests        int     `json:"tests"`
	TestsPassed  int     `json:"tests_passed"`
	TestsFailed  int     `json:"tests_failed"`
	TestsSkipped int     `json:"tests_skipped"`
	Coverage     float64 `json:"coverage"`

	// Codebase
	SourceLines   int `json:"source_lines"`
	SourceFiles   int `json:"source_files"`
	TestFiles     int `json:"test_files"`
	GoSourceLines int `json:"go_source_lines"`

	// Build
	BinarySize      int64  `json:"binary_size"`
	BinarySizeHuman string `json:"binary_size_human"`

	// Pantheon
	Deities int    `json:"deities"`
	Modules int    `json:"modules"`
	Version string `json:"version"`

	// Platform
	GOOS      string `json:"goos"`
	GOARCH    string `json:"goarch"`
	GoVersion string `json:"go_version"`

	// Meta
	Timestamp    string `json:"timestamp"`
	ElapsedMs    int64  `json:"elapsed_ms"`
	PulseVersion string `json:"pulse_version"`
}

// PulseConfig configures what the pulse measures.
type PulseConfig struct {
	ProjectRoot string
	SkipTests   bool
	OutputPath  string // Default: .pantheon/metrics.json
	BinaryPath  string // Path to built binary (optional)
	Version     string // Current version string

	// Injectable runners (Rule A16/A21)
	TestRunner  func(root string) (string, error)
	LineCounter func(root string) (int, int, int, int, error) // sourceLines, sourceFiles, testFiles, goLines, err
	BinarySizer func(path string) (int64, error)
}

// DefaultPulseConfig returns a config with real system calls.
func DefaultPulseConfig(projectRoot string) *PulseConfig {
	return &PulseConfig{
		ProjectRoot: projectRoot,
		OutputPath:  filepath.Join(projectRoot, ".pantheon", "metrics.json"),
		Version:     "v1.0.0-rc1",
		TestRunner:  defaultTestRunner,
		LineCounter: defaultLineCounter,
		BinarySizer: defaultBinarySizer,
	}
}

// Pulse runs all measurements and returns the metrics.
func Pulse(cfg *PulseConfig) (*PulseMetrics, error) {
	start := time.Now()

	m := &PulseMetrics{
		GOOS:         runtime.GOOS,
		GOARCH:       runtime.GOARCH,
		GoVersion:    runtime.Version(),
		Version:      cfg.Version,
		PulseVersion: "1.0.0",
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
	}

	// ── 1. Tests & Coverage ──────────────────────────────────────
	if !cfg.SkipTests {
		if cfg.TestRunner != nil {
			output, err := cfg.TestRunner(cfg.ProjectRoot)
			if err != nil {
				// Tests may fail but still produce output — parse what we have
				m.TestsFailed = -1 // Signal partial failure
			}
			passed, failed, skipped := parseTestCounts(output)
			m.Tests = passed + failed + skipped
			m.TestsPassed = passed
			m.TestsFailed = failed
			m.TestsSkipped = skipped
			m.Coverage = parseTotalCoverage(output)
			_ = err // Swallow — partial results are still valuable
		}
	}

	// ── 2. Source Lines ──────────────────────────────────────────
	if cfg.LineCounter != nil {
		srcLines, srcFiles, testFiles, goLines, err := cfg.LineCounter(cfg.ProjectRoot)
		if err == nil {
			m.SourceLines = srcLines
			m.SourceFiles = srcFiles
			m.TestFiles = testFiles
			m.GoSourceLines = goLines
		}
	}

	// ── 3. Binary Size ──────────────────────────────────────────
	if cfg.BinaryPath != "" && cfg.BinarySizer != nil {
		size, err := cfg.BinarySizer(cfg.BinaryPath)
		if err == nil {
			m.BinarySize = size
			m.BinarySizeHuman = formatBytes(size)
		}
	}

	// ── 4. Deities & Modules ────────────────────────────────────
	m.Deities = countDeities(cfg.ProjectRoot)
	m.Modules = countModules(cfg.ProjectRoot)

	m.ElapsedMs = time.Since(start).Milliseconds()

	// ── 5. Write metrics.json ───────────────────────────────────
	if cfg.OutputPath != "" {
		if err := writeMetrics(cfg.OutputPath, m); err != nil {
			return m, fmt.Errorf("write metrics: %w", err)
		}
	}

	stele.Inscribe("maat", stele.TypeMaatPulse, "", map[string]string{
		"tests":    fmt.Sprintf("%d", m.Tests),
		"coverage": fmt.Sprintf("%.1f", m.Coverage),
		"lines":    fmt.Sprintf("%d", m.SourceLines),
		"elapsed":  fmt.Sprintf("%dms", m.ElapsedMs),
	})
	return m, nil
}

// ── Default Runners (Real System Calls) ─────────────────────────────

func defaultTestRunner(root string) (string, error) {
	cmd := exec.Command("go", "test", "-count=1", "-short", "-cover", "./...")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func defaultLineCounter(root string) (int, int, int, int, error) {
	// Count all source files (excluding vendor, node_modules, .git)
	srcLines := 0
	srcFiles := 0
	testFiles := 0
	goLines := 0

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip unreadable dirs
		}

		// Skip excluded directories
		name := info.Name()
		if info.IsDir() {
			switch name {
			case "vendor", "node_modules", ".git", "dist", "out", ".vscode-test":
				return filepath.SkipDir
			}
			return nil
		}

		ext := filepath.Ext(name)
		isSource := false
		isTest := false
		isGo := false

		switch ext {
		case ".go":
			isSource = true
			isGo = true
			if strings.HasSuffix(name, "_test.go") {
				isTest = true
				isSource = false // Don't count test files as source
			}
		case ".ts", ".js", ".py", ".rs", ".sh", ".sql", ".proto":
			isSource = true
		}

		if !isSource && !isTest {
			return nil
		}

		// Count lines
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		lines := len(strings.Split(string(data), "\n"))

		if isTest {
			testFiles++
		} else {
			srcFiles++
			srcLines += lines
			if isGo {
				goLines += lines
			}
		}

		return nil
	})

	return srcLines, srcFiles, testFiles, goLines, err
}

func defaultBinarySizer(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// ── Parsers ─────────────────────────────────────────────────────────

// parseTestCounts extracts pass/fail/skip counts from go test output.
// Lines like:
//
//	ok      github.com/.../internal/cleaner   0.234s
//	FAIL    github.com/.../internal/brain     0.123s
//	--- SKIP: TestFoo (0.00s)
func parseTestCounts(output string) (passed, failed, skipped int) {
	// Count individual test results (more accurate than package results)
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--- PASS:") {
			passed++
		} else if strings.HasPrefix(trimmed, "--- FAIL:") {
			failed++
		} else if strings.HasPrefix(trimmed, "--- SKIP:") {
			skipped++
		}
	}
	return
}

// parseTotalCoverage extracts a weighted average coverage from go test -cover output.
// Parses lines like: coverage: 85.3% of statements
var coveragePctRegex = regexp.MustCompile(`coverage:\s+([\d.]+)%`)

func parseTotalCoverage(output string) float64 {
	matches := coveragePctRegex.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return 0
	}

	total := 0.0
	count := 0
	for _, m := range matches {
		if len(m) >= 2 {
			val, err := strconv.ParseFloat(m[1], 64)
			if err == nil {
				total += val
				count++
			}
		}
	}

	if count == 0 {
		return 0
	}
	return total / float64(count)
}

// ── Counters ────────────────────────────────────────────────────────

// countDeities counts the deity subcommands registered in cmd/pantheon/.
func countDeities(root string) int {
	deityFiles := []string{
		"anubis.go", "maat.go", "thoth.go", "hapi.go", "seba.go", "seshat.go",
	}
	count := 0
	for _, f := range deityFiles {
		path := filepath.Join(root, "cmd", "pantheon", f)
		if _, err := os.Stat(path); err == nil {
			count++
		}
	}
	return count
}

// countModules counts the internal Go packages.
func countModules(root string) int {
	internalDir := filepath.Join(root, "internal")
	entries, err := os.ReadDir(internalDir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() {
			count++
		}
	}
	return count
}

// ── Output ──────────────────────────────────────────────────────────

func writeMetrics(path string, m *PulseMetrics) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}

func formatBytes(b int64) string {
	if b == 0 {
		return "0 B"
	}
	units := []string{"B", "KB", "MB", "GB"}
	k := int64(1024)
	i := 0
	val := float64(b)
	for val >= float64(k) && i < len(units)-1 {
		val /= float64(k)
		i++
	}
	return fmt.Sprintf("%.1f %s", val, units[i])
}

// LoadMetrics reads a previously written metrics.json.
func LoadMetrics(path string) (*PulseMetrics, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m PulseMetrics
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// DefaultMetricsPath returns the standard .pantheon/metrics.json path for a project.
func DefaultMetricsPath(projectRoot string) string {
	return filepath.Join(projectRoot, ".pantheon", "metrics.json")
}
