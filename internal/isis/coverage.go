package isis

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// CoverageStrategy identifies untested exported functions and generates
// a structured report of coverage gaps. It does not write tests — it
// tells you exactly WHERE tests are needed so you can write them.
type CoverageStrategy struct {
	ProjectRoot string
}

// NewCoverageStrategy creates a CoverageStrategy.
func NewCoverageStrategy(projectRoot string) *CoverageStrategy {
	return &CoverageStrategy{ProjectRoot: projectRoot}
}

// Name returns the strategy name.
func (s *CoverageStrategy) Name() string { return "coverage" }

// CanHeal returns true for coverage-related findings.
func (s *CoverageStrategy) CanHeal(finding Finding) bool {
	msg := strings.ToLower(finding.Message)
	domain := strings.ToLower(finding.Domain)

	return domain == "coverage" ||
		strings.Contains(msg, "coverage") ||
		strings.Contains(msg, "test") ||
		strings.Contains(finding.Remediation, "Add tests")
}

// UncoveredExport represents an exported function that lacks a corresponding test.
type UncoveredExport struct {
	Package  string
	Function string
	File     string
	Line     int
}

// Heal identifies untested exported functions in the target module.
func (s *CoverageStrategy) Heal(finding Finding, dryRun bool) HealResult {
	result := HealResult{
		Finding:  finding,
		Strategy: s.Name(),
		DryRun:   dryRun,
	}

	// Extract module name from subject (e.g., "mirror: 45.2% coverage...")
	module := extractModule(finding.Subject)
	if module == "" {
		result.Action = "could not determine target module"
		return result
	}

	moduleDir := filepath.Join(s.ProjectRoot, "internal", module)
	if _, err := os.Stat(moduleDir); os.IsNotExist(err) {
		result.Action = fmt.Sprintf("module directory not found: internal/%s", module)
		return result
	}

	uncovered := s.findUncoveredExports(moduleDir)
	if len(uncovered) == 0 {
		result.Healed = true
		result.Action = fmt.Sprintf("all exported functions in %s have corresponding tests", module)
		return result
	}

	// Build the report
	var files []string
	seen := make(map[string]bool)
	for _, u := range uncovered {
		if !seen[u.File] {
			files = append(files, u.File)
			seen[u.File] = true
		}
	}

	result.FilesChanged = files
	result.Action = fmt.Sprintf("found %d untested exported function(s) in %s — test scaffold recommended", len(uncovered), module)

	return result
}

// findUncoveredExports compares exported functions in .go files against
// test functions in _test.go files to find the gaps.
func (s *CoverageStrategy) findUncoveredExports(moduleDir string) []UncoveredExport {
	// Phase 1: Collect all exported function names from source files
	exports := s.collectExports(moduleDir)

	// Phase 2: Collect all test function names
	testTargets := s.collectTestTargets(moduleDir)

	// Phase 3: Find exports without tests
	var uncovered []UncoveredExport
	for _, e := range exports {
		if !testTargets[e.Function] {
			uncovered = append(uncovered, e)
		}
	}
	return uncovered
}

// collectExports parses Go source files and extracts exported function/method names.
func (s *CoverageStrategy) collectExports(dir string) []UncoveredExport {
	var exports []UncoveredExport
	fset := token.NewFileSet()

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		path := filepath.Join(dir, name)
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			continue
		}

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || !fn.Name.IsExported() {
				continue
			}
			exports = append(exports, UncoveredExport{
				Package:  file.Name.Name,
				Function: fn.Name.Name,
				File:     name,
				Line:     fset.Position(fn.Pos()).Line,
			})
		}
	}

	return exports
}

// collectTestTargets scans _test.go files for function names referenced in tests.
// Uses heuristic: if "FuncName" appears in any test file, we consider it tested.
func (s *CoverageStrategy) collectTestTargets(dir string) map[string]bool {
	targets := make(map[string]bool)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return targets
	}

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, "_test.go") {
			continue
		}

		path := filepath.Join(dir, name)
		f, err := os.Open(path)
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			// Extract Test function targets: TestFoo → Foo
			if strings.HasPrefix(line, "func Test") {
				if idx := strings.Index(line, "("); idx > 0 {
					testName := line[len("func Test"):idx]
					// Strip underscores (TestFoo_EdgeCase → Foo)
					if uidx := strings.Index(testName, "_"); uidx > 0 {
						testName = testName[:uidx]
					}
					if testName != "" {
						targets[testName] = true
					}
				}
			}
		}
		f.Close()
	}

	return targets
}

// extractModule pulls a module name from the subject string.
// Handles formats like "mirror", "mirror: 45.2% coverage", etc.
func extractModule(subject string) string {
	s := strings.TrimSpace(subject)
	if idx := strings.Index(s, ":"); idx > 0 {
		s = s[:idx]
	}
	s = strings.TrimSpace(s)
	// Validate it looks like a package name (alphanumeric, no spaces)
	for _, c := range s {
		if c != '_' && (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return ""
		}
	}
	return s
}
