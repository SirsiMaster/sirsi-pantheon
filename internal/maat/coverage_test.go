package maat

import (
	"path/filepath"
	"strings"
	"testing"
)

// --- ParseCoverageOutput tests ---

func TestParseCoverageOutputBasic(t *testing.T) {
	input := `ok  	github.com/SirsiMaster/sirsi-pantheon/internal/cleaner	0.234s	coverage: 77.3% of statements
ok  	github.com/SirsiMaster/sirsi-pantheon/internal/ka	1.002s	coverage: 42.7% of statements
?   	github.com/SirsiMaster/sirsi-pantheon/internal/mapper	[no test files]`

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
	input := `ok  	github.com/SirsiMaster/sirsi-pantheon/internal/jackal/rules	0.123s	coverage: 55.0% of statements
ok  	github.com/SirsiMaster/sirsi-pantheon/internal/jackal	0.456s	coverage: 60.0% of statements`

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
	input := `FAIL	github.com/SirsiMaster/sirsi-pantheon/internal/mirror	0.789s	coverage: 60.0% of statements`

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
			return `ok  	github.com/SirsiMaster/sirsi-pantheon/internal/cleaner	0.234s	coverage: 85.0% of statements`, nil
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
	// Weight is threshold-normalized compliance: 85% against an 80%
	// threshold fully meets the declared standard → 100.
	if assessments[0].FeatherWeight != 100 {
		t.Errorf("weight = %d, want 100 (compliant with threshold)", assessments[0].FeatherWeight)
	}
}

func TestCoverageAssessorFail(t *testing.T) {
	ca := &CoverageAssessor{
		Thresholds: []CoverageThreshold{
			{Module: "cleaner", MinCoverage: 80, SafetyCritical: true},
		},
		Runner: func() (string, error) {
			return `ok  	github.com/SirsiMaster/sirsi-pantheon/internal/cleaner	0.234s	coverage: 50.0% of statements`, nil
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
			return `ok  	github.com/SirsiMaster/sirsi-pantheon/internal/cleaner	0.234s	coverage: 70.0% of statements`, nil
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
			return `?   	github.com/SirsiMaster/sirsi-pantheon/internal/mapper	[no test files]`, nil
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
				`ok  	github.com/SirsiMaster/sirsi-pantheon/internal/cleaner	0.234s	coverage: 85.0% of statements`,
				`ok  	github.com/SirsiMaster/sirsi-pantheon/internal/ka	1.002s	coverage: 42.7% of statements`,
				`ok  	github.com/SirsiMaster/sirsi-pantheon/internal/mirror	0.345s	coverage: 60.0% of statements`,
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

// --- Go 1.20+ bare no-test-with-cover format ---

func TestParseCoverageOutputBareNoTestLines(t *testing.T) {
	// Under -cover, modern Go emits a bare status-less line for packages
	// with no test files. These must be detected as NoTests, not dropped.
	input := "ok  \tgithub.com/SirsiMaster/sirsi-pantheon/internal/gemma\t1.377s\tcoverage: 53.7% of statements\n" +
		"\tgithub.com/SirsiMaster/sirsi-pantheon/internal/help\t\tcoverage: 0.0% of statements\n" +
		"\tgithub.com/SirsiMaster/sirsi-pantheon/internal/vitals\t\tcoverage: 0.0% of statements\n" +
		"ok  \tgithub.com/SirsiMaster/sirsi-pantheon/internal/horus\t2.345s\tcoverage: 88.6% of statements"

	results := ParseCoverageOutput(input)
	if len(results) != 4 {
		t.Fatalf("got %d results, want 4", len(results))
	}

	byPkg := make(map[string]CoverageResult)
	for _, r := range results {
		byPkg[r.Package] = r
	}
	if r := byPkg["help"]; !r.NoTests {
		t.Errorf("help: NoTests = false, want true (bare -cover no-test line)")
	}
	if r := byPkg["vitals"]; !r.NoTests {
		t.Errorf("vitals: NoTests = false, want true (bare -cover no-test line)")
	}
	if r := byPkg["gemma"]; r.NoTests || r.Coverage != 53.7 {
		t.Errorf("gemma: got noTests=%v cov=%.1f, want tested/53.7", r.NoTests, r.Coverage)
	}
}

func TestParseCoverageOutputZeroCoverageWithTests(t *testing.T) {
	// A package WITH tests that covers 0.0% still has an "ok" status column
	// and must be reported as measured 0.0, not NoTests.
	input := "ok  \tgithub.com/SirsiMaster/sirsi-pantheon/internal/foo\t0.1s\tcoverage: 0.0% of statements"
	results := ParseCoverageOutput(input)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].NoTests {
		t.Error("foo: NoTests = true, want false (has tests, zero coverage)")
	}
	if results[0].Coverage != 0.0 {
		t.Errorf("foo: coverage = %.1f, want 0.0", results[0].Coverage)
	}
}

// --- SkipTests cache-fallback tests ---

func TestSkipTestsFallsBackWhenCacheIncomplete(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "coverage-cache.json")
	// Cache only knows about "cleaner" — "ka" is missing.
	if err := saveCoverageCache(cachePath, []CoverageResult{
		{Package: "cleaner", Coverage: 85.0},
	}); err != nil {
		t.Fatalf("saveCoverageCache: %v", err)
	}

	runnerCalled := false
	ca := &CoverageAssessor{
		Thresholds: []CoverageThreshold{
			{Module: "cleaner", MinCoverage: 80},
			{Module: "ka", MinCoverage: 80},
		},
		CachePath: cachePath,
		SkipTests: true,
		Runner: func() (string, error) {
			runnerCalled = true
			return "ok  \tgithub.com/SirsiMaster/sirsi-pantheon/internal/cleaner\t0.2s\tcoverage: 85.0% of statements\n" +
				"ok  \tgithub.com/SirsiMaster/sirsi-pantheon/internal/ka\t0.3s\tcoverage: 90.0% of statements", nil
		},
	}

	assessments, err := ca.Assess()
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if !runnerCalled {
		t.Error("expected fallback to a real scan when cache does not cover all thresholds")
	}
	for _, a := range assessments {
		if strings.Contains(a.Message, "no coverage data found") {
			t.Errorf("module %s reported no coverage data despite fallback", a.Subject)
		}
	}
}

func TestSkipTestsUsesCompleteCache(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "coverage-cache.json")
	if err := saveCoverageCache(cachePath, []CoverageResult{
		{Package: "cleaner", Coverage: 85.0},
		{Package: "ka", Coverage: 90.0},
	}); err != nil {
		t.Fatalf("saveCoverageCache: %v", err)
	}

	ca := &CoverageAssessor{
		Thresholds: []CoverageThreshold{
			{Module: "cleaner", MinCoverage: 80},
			{Module: "ka", MinCoverage: 80},
		},
		CachePath: cachePath,
		SkipTests: true,
		Runner: func() (string, error) {
			t.Error("Runner must not be called when the cache covers all thresholds")
			return "", nil
		},
	}

	assessments, err := ca.Assess()
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if len(assessments) != 2 {
		t.Fatalf("got %d assessments, want 2", len(assessments))
	}
	for _, a := range assessments {
		if a.Verdict != VerdictPass {
			t.Errorf("module %s verdict = %v, want pass (from cache)", a.Subject, a.Verdict)
		}
	}
}

// --- weightAgainst (threshold-normalized feather weight) ---

func TestWeightAgainst(t *testing.T) {
	tests := []struct {
		name string
		cov  float64
		min  float64
		want int
	}{
		{"tier C compliant weighs 100", 45, 30, 100},
		{"exactly at threshold weighs 100", 80, 80, 100},
		{"tier A shortfall is proportional", 45, 80, 56},
		{"warning band lands 80-99", 70, 80, 87},
		{"zero coverage weighs 0", 0, 80, 0},
		{"zero threshold weighs 100", 12, 0, 100},
	}
	for _, tt := range tests {
		if got := weightAgainst(tt.cov, tt.min); got != tt.want {
			t.Errorf("%s: weightAgainst(%.0f, %.0f) = %d, want %d", tt.name, tt.cov, tt.min, got, tt.want)
		}
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

// ── Tier System Tests ───────────────────────────────────────────────

func TestTierThreshold(t *testing.T) {
	tests := []struct {
		tier CoverageTier
		want float64
	}{
		{TierA, 80},
		{TierB, 50},
		{TierC, 30},
	}
	for _, tt := range tests {
		got := TierThreshold(tt.tier)
		if got != tt.want {
			t.Errorf("TierThreshold(%s) = %.0f, want %.0f", tt.tier, got, tt.want)
		}
	}
}

func TestModuleTier_SafetyCriticalAlwaysTierA(t *testing.T) {
	// Safety-critical modules must always be Tier A
	for mod := range safetyCriticalModules {
		tier := ModuleTier(mod)
		if tier != TierA {
			t.Errorf("ModuleTier(%q) = %s, want A (safety-critical)", mod, tier)
		}
	}
}

func TestModuleTier_ExplicitAssignments(t *testing.T) {
	tests := []struct {
		module string
		want   CoverageTier
	}{
		{"cleaner", TierA},
		{"guard", TierA},
		{"scales", TierA},
		{"ka", TierA},
		{"mirror", TierA},
		{"jackal", TierB},
		{"mcp", TierB},
		{"maat", TierB},
		{"ra", TierB},
		{"output", TierC},
		{"dashboard", TierC},
	}
	for _, tt := range tests {
		got := ModuleTier(tt.module)
		if got != tt.want {
			t.Errorf("ModuleTier(%q) = %s, want %s", tt.module, got, tt.want)
		}
	}
}

func TestModuleTier_UnknownDefaultsTierB(t *testing.T) {
	tier := ModuleTier("unknown_new_module")
	if tier != TierB {
		t.Errorf("ModuleTier(unknown) = %s, want B (default)", tier)
	}
}

func TestDefaultThresholds_TierAware(t *testing.T) {
	thresholds := DefaultThresholds()
	if len(thresholds) == 0 {
		t.Fatal("DefaultThresholds() returned empty")
	}

	for _, th := range thresholds {
		tier := ModuleTier(th.Module)
		expected := TierThreshold(tier)
		if th.MinCoverage != expected {
			t.Errorf("module %q: threshold=%.0f%%, expected=%.0f%% (Tier %s)",
				th.Module, th.MinCoverage, expected, tier)
		}
	}
}

func TestDefaultThresholds_OutputIsTierC(t *testing.T) {
	thresholds := DefaultThresholds()
	for _, th := range thresholds {
		if th.Module == "output" {
			if th.MinCoverage != 30 {
				t.Errorf("output threshold = %.0f, want 30 (Tier C)", th.MinCoverage)
			}
			return
		}
	}
	t.Error("output module not found in thresholds")
}

func TestDefaultThresholds_CleanerIsTierA(t *testing.T) {
	thresholds := DefaultThresholds()
	for _, th := range thresholds {
		if th.Module == "cleaner" {
			if th.MinCoverage != 80 {
				t.Errorf("cleaner threshold = %.0f, want 80 (Tier A, safety-critical)", th.MinCoverage)
			}
			if !th.SafetyCritical {
				t.Error("cleaner should be marked safety-critical")
			}
			return
		}
	}
	t.Error("cleaner module not found in thresholds")
}

func TestEvaluate_TierInfoInStandard(t *testing.T) {
	ca := &CoverageAssessor{
		Thresholds: []CoverageThreshold{
			{Module: "output", MinCoverage: 30},
		},
	}
	results := []CoverageResult{{Package: "output", Coverage: 30.5}}
	assessments := ca.evaluate(results)
	if len(assessments) != 1 {
		t.Fatalf("expected 1 assessment, got %d", len(assessments))
	}
	if !strings.Contains(assessments[0].Standard, "Tier C") {
		t.Errorf("standard should mention Tier C: %q", assessments[0].Standard)
	}
}
