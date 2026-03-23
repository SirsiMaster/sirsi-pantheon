package maat

import (
	"strings"
	"testing"
)

// --- ParseCoverageOutput tests ---

func TestParseCoverageOutputBasic(t *testing.T) {
	input := `ok  	github.com/SirsiMaster/sirsi-anubis/internal/cleaner	0.234s	coverage: 77.3% of statements
ok  	github.com/SirsiMaster/sirsi-anubis/internal/ka	1.002s	coverage: 42.7% of statements
?   	github.com/SirsiMaster/sirsi-anubis/internal/mapper	[no test files]`

	results := ParseCoverageOutput(input)

	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}

	// Check cleaner
	if results[0].Package != "cleaner" || results[0].Coverage != 77.3 {
		t.Errorf("cleaner: got pkg=%q cov=%.1f, want cleaner/77.3", results[0].Package, results[0].Coverage)
	}

	// Check ka
	if results[1].Package != "ka" || results[1].Coverage != 42.7 {
		t.Errorf("ka: got pkg=%q cov=%.1f, want ka/42.7", results[1].Package, results[1].Coverage)
	}

	// Check mapper (no tests)
	if results[2].Package != "mapper" || !results[2].NoTests {
		t.Errorf("mapper: got pkg=%q noTests=%v, want mapper/true", results[2].Package, results[2].NoTests)
	}
}

func TestParseCoverageOutputSubPackages(t *testing.T) {
	input := `ok  	github.com/SirsiMaster/sirsi-anubis/internal/jackal/rules	0.123s	coverage: 55.0% of statements
ok  	github.com/SirsiMaster/sirsi-anubis/internal/jackal	0.456s	coverage: 60.0% of statements`

	results := ParseCoverageOutput(input)

	// Sub-packages should be normalized to the parent.
	// "jackal/rules" → "jackal", but the first one seen wins.
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1 (sub-package should merge)", len(results))
	}
	if results[0].Package != "jackal" {
		t.Errorf("got pkg=%q, want jackal", results[0].Package)
	}
}

func TestParseCoverageOutputFailedTests(t *testing.T) {
	input := `FAIL	github.com/SirsiMaster/sirsi-anubis/internal/mirror	0.789s	coverage: 60.0% of statements`

	results := ParseCoverageOutput(input)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Package != "mirror" || results[0].Coverage != 60.0 {
		t.Errorf("mirror: got pkg=%q cov=%.1f", results[0].Package, results[0].Coverage)
	}
}

func TestParseCoverageOutputEmpty(t *testing.T) {
	results := ParseCoverageOutput("")
	if len(results) != 0 {
		t.Errorf("empty input should produce 0 results, got %d", len(results))
	}
}

// --- CoverageAssessor tests ---

func TestCoverageAssessorPass(t *testing.T) {
	ca := &CoverageAssessor{
		Thresholds: []CoverageThreshold{
			{Module: "cleaner", MinCoverage: 80, SafetyCritical: true},
		},
		Runner: func() (string, error) {
			return `ok  	github.com/SirsiMaster/sirsi-anubis/internal/cleaner	0.234s	coverage: 85.0% of statements`, nil
		},
	}

	assessments, err := ca.Assess()
	if err != nil {
		t.Fatalf("Assess() error = %v", err)
	}

	if len(assessments) != 1 {
		t.Fatalf("got %d assessments, want 1", len(assessments))
	}

	if assessments[0].Verdict != VerdictPass {
		t.Errorf("verdict = %v, want pass", assessments[0].Verdict)
	}
	if assessments[0].FeatherWeight != 85 {
		t.Errorf("weight = %d, want 85", assessments[0].FeatherWeight)
	}
}

func TestCoverageAssessorFail(t *testing.T) {
	ca := &CoverageAssessor{
		Thresholds: []CoverageThreshold{
			{Module: "cleaner", MinCoverage: 80, SafetyCritical: true},
		},
		Runner: func() (string, error) {
			return `ok  	github.com/SirsiMaster/sirsi-anubis/internal/cleaner	0.234s	coverage: 50.0% of statements`, nil
		},
	}

	assessments, err := ca.Assess()
	if err != nil {
		t.Fatalf("Assess() error = %v", err)
	}

	if assessments[0].Verdict != VerdictFail {
		t.Errorf("verdict = %v, want fail (50%% < 80%% and not within 80%% threshold)", assessments[0].Verdict)
	}
}

func TestCoverageAssessorWarning(t *testing.T) {
	ca := &CoverageAssessor{
		Thresholds: []CoverageThreshold{
			{Module: "cleaner", MinCoverage: 80, SafetyCritical: true},
		},
		Runner: func() (string, error) {
			// 70% is within 80% of 80% (64%), so it should be a warning, not a fail.
			return `ok  	github.com/SirsiMaster/sirsi-anubis/internal/cleaner	0.234s	coverage: 70.0% of statements`, nil
		},
	}

	assessments, err := ca.Assess()
	if err != nil {
		t.Fatalf("Assess() error = %v", err)
	}

	if assessments[0].Verdict != VerdictWarning {
		t.Errorf("verdict = %v, want warning (70%% is close to 80%%)", assessments[0].Verdict)
	}
}

func TestCoverageAssessorNoTests(t *testing.T) {
	ca := &CoverageAssessor{
		Thresholds: []CoverageThreshold{
			{Module: "mapper", MinCoverage: 50},
		},
		Runner: func() (string, error) {
			return `?   	github.com/SirsiMaster/sirsi-anubis/internal/mapper	[no test files]`, nil
		},
	}

	assessments, err := ca.Assess()
	if err != nil {
		t.Fatalf("Assess() error = %v", err)
	}

	if assessments[0].Verdict != VerdictFail {
		t.Errorf("verdict = %v, want fail (no test files)", assessments[0].Verdict)
	}
	if assessments[0].FeatherWeight != 0 {
		t.Errorf("weight = %d, want 0 (no tests)", assessments[0].FeatherWeight)
	}
}

func TestCoverageAssessorDomain(t *testing.T) {
	ca := &CoverageAssessor{}
	if ca.Domain() != DomainCoverage {
		t.Errorf("domain = %v, want coverage", ca.Domain())
	}
}

func TestCoverageAssessorMultipleModules(t *testing.T) {
	ca := &CoverageAssessor{
		Thresholds: []CoverageThreshold{
			{Module: "cleaner", MinCoverage: 80, SafetyCritical: true},
			{Module: "ka", MinCoverage: 50},
			{Module: "mirror", MinCoverage: 50},
		},
		Runner: func() (string, error) {
			return strings.Join([]string{
				`ok  	github.com/SirsiMaster/sirsi-anubis/internal/cleaner	0.234s	coverage: 85.0% of statements`,
				`ok  	github.com/SirsiMaster/sirsi-anubis/internal/ka	1.002s	coverage: 42.7% of statements`,
				`ok  	github.com/SirsiMaster/sirsi-anubis/internal/mirror	0.345s	coverage: 60.0% of statements`,
			}, "\n"), nil
		},
	}

	assessments, err := ca.Assess()
	if err != nil {
		t.Fatalf("Assess() error = %v", err)
	}

	if len(assessments) != 3 {
		t.Fatalf("got %d assessments, want 3", len(assessments))
	}

	// cleaner: 85% >= 80% → pass
	if assessments[0].Verdict != VerdictPass {
		t.Errorf("cleaner verdict = %v, want pass", assessments[0].Verdict)
	}
	// ka: 42.7% < 50%*0.8=40% → warning (42.7 >= 40)
	if assessments[1].Verdict != VerdictWarning {
		t.Errorf("ka verdict = %v, want warning", assessments[1].Verdict)
	}
	// mirror: 60% >= 50% → pass
	if assessments[2].Verdict != VerdictPass {
		t.Errorf("mirror verdict = %v, want pass", assessments[2].Verdict)
	}
}

// --- normalizePackageName tests ---

func TestNormalizePackageName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"cleaner", "cleaner"},
		{"jackal/rules", "jackal"},
		{"hapi/gpu", "hapi"},
		{"a/b/c", "a"},
	}
	for _, tt := range tests {
		got := normalizePackageName(tt.input)
		if got != tt.want {
			t.Errorf("normalizePackageName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// --- clampWeight tests ---

func TestClampWeight(t *testing.T) {
	tests := []struct {
		input int
		want  int
	}{
		{50, 50},
		{0, 0},
		{100, 100},
		{-10, 0},
		{200, 100},
	}
	for _, tt := range tests {
		got := clampWeight(tt.input)
		if got != tt.want {
			t.Errorf("clampWeight(%d) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
