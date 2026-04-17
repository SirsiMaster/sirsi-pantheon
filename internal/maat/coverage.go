package maat

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
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

// PackageProgress reports the result of a single package test as it completes.
type PackageProgress struct {
	Package  string  // Package name (e.g., "cleaner", "jackal")
	Coverage float64 // Coverage percentage (0 if no tests)
	NoTests  bool    // True if [no test files]
	Failed   bool    // True if FAIL
	Current  int     // 1-based index of this package in the run
	Total    int     // Total expected packages (0 if unknown)
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

	// DiffOnly when true, only tests packages with changed .go files
	// since the last remote HEAD. Uses cached coverage for unchanged packages.
	DiffOnly bool

	// DiffBase is the git ref to diff against. Defaults to "origin/HEAD".
	DiffBase string

	// CachePath overrides the default coverage cache location.
	// Default: ~/.config/pantheon/maat/coverage-cache.json
	CachePath string

	// SkipTests when true, skips running go test and uses only cached results.
	// Useful for fast governance checks that don't need fresh coverage.
	SkipTests bool

	// ProgressFn is called for each package as test results stream in.
	// If nil, no progress is reported.
	ProgressFn func(PackageProgress)
}

// safetyCriticalModules are modules where coverage is paramount (delete/kill operations).
var safetyCriticalModules = map[string]bool{
	"cleaner": true,
	"guard":   true,
}

// elevatedThresholds are modules with higher-than-default coverage requirements.
var elevatedThresholds = map[string]float64{
	"cleaner": 80,
	"guard":   80,
	"scales":  80,
}

// DefaultThresholds dynamically discovers all internal/* packages and returns
// coverage thresholds for each. Modules not in elevatedThresholds get a default
// minimum of 50%. This ensures new modules are never invisible to Ma'at.
func DefaultThresholds() []CoverageThreshold {
	return DefaultThresholdsFromDir("")
}

// DefaultThresholdsFromDir discovers internal packages from a given project root.
// If root is empty, it uses the current working directory.
func DefaultThresholdsFromDir(root string) []CoverageThreshold {
	internalDir := filepath.Join(root, "internal")
	entries, err := os.ReadDir(internalDir)
	if err != nil {
		// Fallback: return a minimal set so tests and CI don't break.
		return fallbackThresholds()
	}

	var thresholds []CoverageThreshold
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		minCov := 50.0 // default threshold
		if elevated, ok := elevatedThresholds[name]; ok {
			minCov = elevated
		}
		thresholds = append(thresholds, CoverageThreshold{
			Module:         name,
			MinCoverage:    minCov,
			SafetyCritical: safetyCriticalModules[name],
		})
	}

	if len(thresholds) == 0 {
		return fallbackThresholds()
	}
	return thresholds
}

// fallbackThresholds returns a hardcoded minimum set for when directory scanning fails.
func fallbackThresholds() []CoverageThreshold {
	return []CoverageThreshold{
		{Module: "cleaner", MinCoverage: 80, SafetyCritical: true},
		{Module: "guard", MinCoverage: 80, SafetyCritical: true},
		{Module: "jackal", MinCoverage: 50},
		{Module: "ka", MinCoverage: 50},
		{Module: "mirror", MinCoverage: 50},
		{Module: "scales", MinCoverage: 80},
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

	// Save fresh results to cache for future diff runs.
	if cache := c.coverageCachePath(); cache != "" {
		_ = saveCoverageCache(cache, results)
	}

	return c.evaluate(results), nil
}

// coverageCachePath returns the path to the coverage cache file.
func (c *CoverageAssessor) coverageCachePath() string {
	if c.CachePath != "" {
		return c.CachePath
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "sirsi", "maat", "coverage-cache.json")
}

// runCoverage executes coverage — either skip, diff-only, or full scan.
func (c *CoverageAssessor) runCoverage() (string, error) {
	if c.SkipTests {
		return c.runSkipTests()
	}

	if c.Runner != nil {
		return c.Runner()
	}

	if c.DiffOnly {
		return c.runDiffCoverage()
	}

	return c.runFullCoverage()
}

// runSkipTests returns cached coverage without running any tests.
func (c *CoverageAssessor) runSkipTests() (string, error) {
	cache := c.coverageCachePath()
	cached, err := loadCoverageCache(cache)
	if err == nil && len(cached) > 0 {
		var lines []string
		for _, r := range cached {
			if r.NoTests {
				lines = append(lines, fmt.Sprintf("?\tgithub.com/SirsiMaster/sirsi-pantheon/internal/%s\t[no test files]", r.Package))
			} else {
				lines = append(lines, fmt.Sprintf("ok\tgithub.com/SirsiMaster/sirsi-pantheon/internal/%s\t(cached)\tcoverage: %.1f%% of statements", r.Package, r.Coverage))
			}
		}
		return strings.Join(lines, "\n"), nil
	}
	// No cache available — return empty so evaluate() gives warnings, not crashes.
	return "", nil
}

// runFullCoverage runs go test -cover on all packages, streaming results
// line-by-line and calling ProgressFn for each completed package.
func (c *CoverageAssessor) runFullCoverage() (string, error) {
	cmd := exec.Command("go", "test", "-cover", "./...")
	if c.ProjectRoot != "" {
		cmd.Dir = c.ProjectRoot
	}

	// If no progress callback, use simple buffered mode.
	if c.ProgressFn == nil {
		out, err := cmd.CombinedOutput()
		if err != nil && len(out) == 0 {
			return "", fmt.Errorf("go test -cover: %w", err)
		}
		return string(out), nil
	}

	// Streaming mode: pipe stdout and read line-by-line.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = cmd.Stdout // merge stderr into stdout pipe

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start go test: %w", err)
	}

	// Count expected packages from thresholds for progress denominator.
	total := len(c.Thresholds)

	var lines []string
	count := 0
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)

		// Detect package result lines and report progress.
		if matches := coverageRegex.FindStringSubmatch(line); len(matches) == 3 {
			count++
			pkg := normalizePackageName(matches[1])
			cov, _ := strconv.ParseFloat(matches[2], 64)
			c.ProgressFn(PackageProgress{
				Package:  pkg,
				Coverage: cov,
				Current:  count,
				Total:    total,
				Failed:   strings.HasPrefix(line, "FAIL"),
			})
		} else if matches := noTestRegex.FindStringSubmatch(line); len(matches) == 2 {
			count++
			pkg := normalizePackageName(matches[1])
			c.ProgressFn(PackageProgress{
				Package: pkg,
				NoTests: true,
				Current: count,
				Total:   total,
			})
		}
	}

	// Wait for command to finish — don't fail on non-zero exit if we got output.
	_ = cmd.Wait()

	return strings.Join(lines, "\n"), nil
}

// runDiffCoverage only tests packages with changed .go files.
// Unchanged packages use cached coverage values.
// Falls back to full coverage when cache is incomplete.
func (c *CoverageAssessor) runDiffCoverage() (string, error) {
	changedPkgs := c.changedPackages()

	// Load cached coverage for unchanged packages.
	cache := c.coverageCachePath()
	cachedResults, _ := loadCoverageCache(cache)

	// If the cache is incomplete (doesn't cover all threshold modules),
	// fall back to a full scan to populate it. This prevents "no coverage
	// data found" verdicts from dragging down the feather weight.
	if !c.cacheCoversThresholds(cachedResults) {
		return c.runFullCoverage()
	}

	cachedMap := make(map[string]CoverageResult)
	for _, r := range cachedResults {
		cachedMap[r.Package] = r
	}

	// If no packages changed, use 100% cached data.
	if len(changedPkgs) == 0 {
		var lines []string
		for _, r := range cachedResults {
			if r.NoTests {
				lines = append(lines, fmt.Sprintf("?\tgithub.com/SirsiMaster/sirsi-pantheon/internal/%s\t[no test files]", r.Package))
			} else {
				lines = append(lines, fmt.Sprintf("ok\tgithub.com/SirsiMaster/sirsi-pantheon/internal/%s\t(cached)\tcoverage: %.1f%% of statements", r.Package, r.Coverage))
			}
		}
		return strings.Join(lines, "\n"), nil
	}

	// Build the package list for go test.
	var pkgArgs []string
	for _, pkg := range changedPkgs {
		pkgArgs = append(pkgArgs, "./internal/"+pkg+"/...")
	}

	// Run go test only on changed packages.
	args := append([]string{"test", "-cover"}, pkgArgs...)
	cmd := exec.Command("go", args...)
	if c.ProjectRoot != "" {
		cmd.Dir = c.ProjectRoot
	}

	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		return "", fmt.Errorf("go test -cover (diff): %w", err)
	}

	// Merge: fresh results for changed packages + cached for unchanged.
	freshResults := ParseCoverageOutput(string(out))
	freshMap := make(map[string]bool)
	for _, r := range freshResults {
		freshMap[r.Package] = true
	}

	// Add cached results for unchanged packages.
	var mergedLines []string
	mergedLines = append(mergedLines, string(out))

	for _, r := range cachedResults {
		if freshMap[r.Package] {
			continue // Already in fresh results.
		}
		if r.NoTests {
			mergedLines = append(mergedLines, fmt.Sprintf("?\tgithub.com/SirsiMaster/sirsi-pantheon/internal/%s\t[no test files]", r.Package))
		} else {
			mergedLines = append(mergedLines, fmt.Sprintf("ok\tgithub.com/SirsiMaster/sirsi-pantheon/internal/%s\t(cached)\tcoverage: %.1f%% of statements", r.Package, r.Coverage))
		}
	}

	return strings.Join(mergedLines, "\n"), nil
}

// cacheCoversThresholds checks if the cache has data for all threshold modules.
func (c *CoverageAssessor) cacheCoversThresholds(cached []CoverageResult) bool {
	if len(cached) == 0 {
		return false
	}
	cachedSet := make(map[string]bool)
	for _, r := range cached {
		cachedSet[r.Package] = true
	}
	for _, t := range c.Thresholds {
		if !cachedSet[t.Module] {
			return false
		}
	}
	return true
}

// changedPackages uses git diff to find which internal/ packages have changed.
func (c *CoverageAssessor) changedPackages() []string {
	base := c.DiffBase
	if base == "" {
		base = "origin/HEAD"
	}

	cmd := exec.Command("git", "diff", "--name-only", base)
	if c.ProjectRoot != "" {
		cmd.Dir = c.ProjectRoot
	}

	out, err := cmd.Output()
	if err != nil {
		// If diff fails (e.g., no remote), fall back to all packages.
		return nil
	}

	pkgSet := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if !strings.HasSuffix(line, ".go") {
			continue
		}
		// Extract package: internal/cleaner/safety.go → cleaner
		if strings.HasPrefix(line, "internal/") {
			parts := strings.SplitN(line[len("internal/"):], "/", 2)
			if len(parts) >= 1 && parts[0] != "" {
				pkgSet[parts[0]] = true
			}
		}
		// cmd/ changes affect the build but not package coverage
	}

	var pkgs []string
	for pkg := range pkgSet {
		pkgs = append(pkgs, pkg)
	}
	return pkgs
}

// coverageCacheEntry is the JSON-serializable cache format.
type coverageCacheEntry struct {
	Results   []CoverageResult `json:"results"`
	Timestamp string           `json:"timestamp"`
}

// saveCoverageCache persists coverage results to disk.
func saveCoverageCache(path string, results []CoverageResult) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	entry := coverageCacheEntry{
		Results:   results,
		Timestamp: time.Now().Format(time.RFC3339),
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// loadCoverageCache reads cached coverage results.
func loadCoverageCache(path string) ([]CoverageResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var entry coverageCacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, err
	}
	return entry.Results, nil
}

// coverageRegex matches lines like:
// ok  	github.com/SirsiMaster/sirsi-pantheon/internal/cleaner	0.234s	coverage: 77.3% of statements
// or lines with [no test files]:
// ?   	github.com/SirsiMaster/sirsi-pantheon/internal/mapper	[no test files]
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
