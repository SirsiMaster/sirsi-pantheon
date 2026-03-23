package maat

import (
	"bufio"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// CoverageThreshold defines the minimum coverage required for a module.
type CoverageThreshold struct {
	// Module is the Go package path suffix (e.g., "cleaner", "ka").
	Module string `json:"module" yaml:"module"`

	// MinCoverage is the minimum required coverage percentage (0-100).
	MinCoverage float64 `json:"min_coverage" yaml:"min_coverage"`

	// SafetyCritical marks modules where coverage is paramount.
	SafetyCritical bool `json:"safety_critical" yaml:"safety_critical"`
}

// CoverageResult is the measured coverage for a single package.
type CoverageResult struct {
	Package  string  `json:"package"`
	Coverage float64 `json:"coverage"`
	// NoTests is true if the package has no test files.
	NoTests bool `json:"no_tests,omitempty"`
}

// CoverageAssessor assesses test coverage against declared thresholds.
type CoverageAssessor struct {
	// Thresholds defines per-module coverage requirements.
	Thresholds []CoverageThreshold

	// Runner executes go test and returns the output.
	// Defaults to running `go test -cover ./...` if nil.
	Runner func() (string, error)

	// ProjectRoot is the root directory for running go test.
	ProjectRoot string
}

// DefaultThresholds returns the canonical coverage thresholds for Anubis.
func DefaultThresholds() []CoverageThreshold {
	return []CoverageThreshold{
		{Module: "cleaner", MinCoverage: 80, SafetyCritical: true},
		{Module: "guard", MinCoverage: 60, SafetyCritical: true},
		{Module: "ka", MinCoverage: 50, SafetyCritical: false},
		{Module: "mirror", MinCoverage: 50, SafetyCritical: false},
		{Module: "jackal", MinCoverage: 50, SafetyCritical: false},
		{Module: "brain", MinCoverage: 50, SafetyCritical: false},
		{Module: "hapi", MinCoverage: 50, SafetyCritical: false},
		{Module: "scales", MinCoverage: 50, SafetyCritical: false},
		{Module: "scarab", MinCoverage: 50, SafetyCritical: false},
		{Module: "sight", MinCoverage: 50, SafetyCritical: false},
		{Module: "profile", MinCoverage: 50, SafetyCritical: false},
		{Module: "stealth", MinCoverage: 50, SafetyCritical: false},
		{Module: "ignore", MinCoverage: 50, SafetyCritical: false},
		{Module: "logging", MinCoverage: 50, SafetyCritical: false},
		{Module: "platform", MinCoverage: 50, SafetyCritical: false},
		{Module: "mcp", MinCoverage: 50, SafetyCritical: false},
		{Module: "updater", MinCoverage: 50, SafetyCritical: false},
	}
}

// Domain returns the quality domain for this assessor.
func (c *CoverageAssessor) Domain() Domain {
	return DomainCoverage
}

// Assess runs coverage analysis and compares against thresholds.
func (c *CoverageAssessor) Assess() ([]Assessment, error) {
	output, err := c.runCoverage()
	if err != nil {
		return nil, fmt.Errorf("run coverage: %w", err)
	}

	results := ParseCoverageOutput(output)
	return c.evaluate(results), nil
}

// runCoverage executes the coverage command or uses the custom runner.
func (c *CoverageAssessor) runCoverage() (string, error) {
	if c.Runner != nil {
		return c.Runner()
	}

	cmd := exec.Command("go", "test", "-cover", "./...")
	if c.ProjectRoot != "" {
		cmd.Dir = c.ProjectRoot
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		// go test returns non-zero on test failure, but we still want the output.
		// Only fail if there's no output at all.
		if len(out) == 0 {
			return "", fmt.Errorf("go test -cover: %w", err)
		}
	}

	return string(out), nil
}

// coverageRegex matches lines like:
// ok  	github.com/SirsiMaster/sirsi-anubis/internal/cleaner	0.234s	coverage: 77.3% of statements
// or lines with [no test files]:
// ?   	github.com/SirsiMaster/sirsi-anubis/internal/mapper	[no test files]
var coverageRegex = regexp.MustCompile(
	`(?:ok|FAIL)\s+\S+/internal/(\S+)\s+\S+\s+coverage:\s+([\d.]+)%`,
)

var noTestRegex = regexp.MustCompile(
	`\?\s+\S+/internal/(\S+)\s+\[no test files\]`,
)

// ParseCoverageOutput extracts coverage results from go test -cover output.
func ParseCoverageOutput(output string) []CoverageResult {
	var results []CoverageResult
	seen := make(map[string]bool)

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()

		// Match coverage lines
		if matches := coverageRegex.FindStringSubmatch(line); len(matches) == 3 {
			pkg := normalizePackageName(matches[1])
			cov, _ := strconv.ParseFloat(matches[2], 64)
			if !seen[pkg] {
				results = append(results, CoverageResult{
					Package:  pkg,
					Coverage: cov,
				})
				seen[pkg] = true
			}
			continue
		}

		// Match no-test-files lines
		if matches := noTestRegex.FindStringSubmatch(line); len(matches) == 2 {
			pkg := normalizePackageName(matches[1])
			if !seen[pkg] {
				results = append(results, CoverageResult{
					Package: pkg,
					NoTests: true,
				})
				seen[pkg] = true
			}
		}
	}

	return results
}

// normalizePackageName strips sub-package paths to get the module name.
// e.g., "jackal/rules" → "jackal"
func normalizePackageName(pkg string) string {
	if idx := strings.Index(pkg, "/"); idx != -1 {
		return pkg[:idx]
	}
	return pkg
}

// evaluate compares measured coverage against thresholds.
func (c *CoverageAssessor) evaluate(results []CoverageResult) []Assessment {
	// Build a map of measured coverage (take highest if sub-packages exist).
	measured := make(map[string]float64)
	noTests := make(map[string]bool)
	for _, r := range results {
		if r.NoTests {
			if _, exists := measured[r.Package]; !exists {
				noTests[r.Package] = true
			}
			continue
		}
		if r.Coverage > measured[r.Package] {
			measured[r.Package] = r.Coverage
			delete(noTests, r.Package)
		}
	}

	var assessments []Assessment
	for _, t := range c.Thresholds {
		cov, hasCov := measured[t.Module]
		_, hasNoTests := noTests[t.Module]

		var a Assessment
		a.Domain = DomainCoverage
		a.Subject = t.Module

		critLabel := ""
		if t.SafetyCritical {
			critLabel = " [safety-critical]"
		}
		a.Standard = fmt.Sprintf("%.0f%% minimum%s", t.MinCoverage, critLabel)

		switch {
		case hasNoTests:
			a.Verdict = VerdictFail
			a.FeatherWeight = 0
			a.Message = fmt.Sprintf("%s: no test files", t.Module)
			a.Remediation = fmt.Sprintf("Add tests to internal/%s/", t.Module)

		case !hasCov:
			// Module not found in the output — might not exist or might be skipped.
			a.Verdict = VerdictWarning
			a.FeatherWeight = 50
			a.Message = fmt.Sprintf("%s: no coverage data found", t.Module)
			a.Remediation = "Verify module exists and has test files"

		case cov >= t.MinCoverage:
			a.Verdict = VerdictPass
			a.FeatherWeight = clampWeight(int(cov))
			a.Message = fmt.Sprintf("%s: %.1f%% coverage (threshold: %.0f%%)", t.Module, cov, t.MinCoverage)

		case cov >= t.MinCoverage*0.8:
			// Within 80% of the threshold — warning.
			a.Verdict = VerdictWarning
			a.FeatherWeight = clampWeight(int(cov))
			a.Message = fmt.Sprintf("%s: %.1f%% coverage (threshold: %.0f%%)", t.Module, cov, t.MinCoverage)
			a.Remediation = fmt.Sprintf("Add tests to bring %s from %.1f%% to %.0f%%", t.Module, cov, t.MinCoverage)

		default:
			a.Verdict = VerdictFail
			a.FeatherWeight = clampWeight(int(cov))
			a.Message = fmt.Sprintf("%s: %.1f%% coverage (threshold: %.0f%%)", t.Module, cov, t.MinCoverage)
			a.Remediation = fmt.Sprintf("Add tests to bring %s from %.1f%% to %.0f%%", t.Module, cov, t.MinCoverage)
		}

		assessments = append(assessments, a)
	}

	return assessments
}

// clampWeight ensures a weight is between 0 and 100.
func clampWeight(w int) int {
	if w < 0 {
		return 0
	}
	if w > 100 {
		return 100
	}
	return w
}
