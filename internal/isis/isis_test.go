package isis

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

// ─── Healer Tests ───

func TestNewHealer(t *testing.T) {
	h := NewHealer("/tmp/test-project")
	if h.ProjectRoot != "/tmp/test-project" {
		t.Errorf("ProjectRoot = %q, want /tmp/test-project", h.ProjectRoot)
	}
	if len(h.Strategies) != 4 {
		t.Errorf("got %d strategies, want 4", len(h.Strategies))
	}
}

func TestHeal_EmptyFindings(t *testing.T) {
	h := NewHealer("/tmp/test-project")
	report := h.Heal(nil, true)
	if report.TotalFindings != 0 {
		t.Errorf("TotalFindings = %d, want 0", report.TotalFindings)
	}
	if report.Healed != 0 {
		t.Errorf("Healed = %d, want 0", report.Healed)
	}
}

func TestHeal_DryRun_NoMatchingStrategy(t *testing.T) {
	h := &Healer{
		ProjectRoot: "/tmp/test-project",
		Strategies:  nil, // No strategies registered
	}

	findings := []Finding{
		{Domain: "unknown", Subject: "foo", Message: "bar"},
	}

	report := h.Heal(findings, true)
	if report.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", report.Skipped)
	}
}

func TestHeal_DispatchesToCorrectStrategy(t *testing.T) {
	called := false
	stub := &stubStrategy{
		name:    "stub",
		canHeal: true,
		healFn: func(f Finding, dryRun bool) HealResult {
			called = true
			return HealResult{Finding: f, Strategy: "stub", Healed: true, Action: "stubbed"}
		},
	}

	h := &Healer{
		ProjectRoot: "/tmp/test-project",
		Strategies:  []Strategy{stub},
	}

	findings := []Finding{
		{Domain: "test", Subject: "x", Message: "y"},
	}

	report := h.Heal(findings, false)
	if !called {
		t.Error("expected stub strategy to be called")
	}
	if report.Healed != 1 {
		t.Errorf("Healed = %d, want 1", report.Healed)
	}
}

// ─── Report Tests ───

func TestReport_Format_Empty(t *testing.T) {
	r := &Report{DryRun: true}
	out := r.Format()
	if !strings.Contains(out, "DRY RUN") {
		t.Error("expected DRY RUN label")
	}
	if !strings.Contains(out, "feather is already balanced") {
		t.Error("expected balanced message for empty report")
	}
}

func TestReport_Format_WithResults(t *testing.T) {
	r := &Report{
		TotalFindings: 2,
		Healed:        1,
		Skipped:       1,
		Results: []HealResult{
			{Finding: Finding{Domain: "lint", Subject: "test"}, Healed: true, Action: "fixed"},
			{Finding: Finding{Domain: "vet", Subject: "test2"}, Action: "skipped"},
		},
	}
	out := r.Format()
	if !strings.Contains(out, "Healed: 1") {
		t.Error("expected healed count")
	}
	if !strings.Contains(out, "restored the weight") {
		t.Error("expected restoration message")
	}
}

func TestReport_Format_WithFailures(t *testing.T) {
	r := &Report{
		TotalFindings: 1,
		Failed:        1,
		Results: []HealResult{
			{Finding: Finding{Domain: "lint", Subject: "test"}, Error: os.ErrPermission, Action: "failed"},
		},
	}
	out := r.Format()
	if !strings.Contains(out, "wounds remain") {
		t.Error("expected failure message")
	}
}

// ─── LintStrategy Tests ───

func TestLintStrategy_Name(t *testing.T) {
	s := NewLintStrategy("/tmp")
	if s.Name() != "lint" {
		t.Errorf("Name = %q, want lint", s.Name())
	}
}

func TestLintStrategy_CanHeal(t *testing.T) {
	s := NewLintStrategy("/tmp")

	tests := []struct {
		finding   Finding
		wantMatch bool
	}{
		{Finding{Domain: "lint", Message: "gofmt diff"}, true},
		{Finding{Domain: "pipeline", Message: "lint errors"}, true},
		{Finding{Message: "goimports issues"}, true},
		{Finding{Domain: "coverage", Message: "test coverage low"}, false},
	}

	for _, tt := range tests {
		got := s.CanHeal(tt.finding)
		if got != tt.wantMatch {
			t.Errorf("CanHeal(%v) = %v, want %v", tt.finding, got, tt.wantMatch)
		}
	}
}

func TestLintStrategy_ResolveTarget(t *testing.T) {
	s := NewLintStrategy("/project")

	tests := []struct {
		subject string
		want    string
	}{
		{"internal/foo/bar.go", "/project/internal/foo/bar.go"},
		{"mirror", "/project/internal/mirror"},
		{"", "/project"},
	}

	for _, tt := range tests {
		got := s.resolveTarget(Finding{Subject: tt.subject})
		if got != tt.want {
			t.Errorf("resolveTarget(%q) = %q, want %q", tt.subject, got, tt.want)
		}
	}
}

func TestLintStrategy_Heal_DryRun(t *testing.T) {
	s := NewLintStrategy("/tmp")
	// Stub RunCmd to return empty (no dirty files)
	s.RunCmd = func(name string, args ...string) *exec.Cmd {
		return exec.Command("echo", "")
	}

	result := s.Heal(Finding{Domain: "lint", Subject: "test", Message: "gofmt"}, true)
	if result.DryRun != true {
		t.Error("expected DryRun = true")
	}
}

// ─── VetStrategy Tests ───

func TestVetStrategy_Name(t *testing.T) {
	s := NewVetStrategy("/tmp")
	if s.Name() != "vet" {
		t.Errorf("Name = %q, want vet", s.Name())
	}
}

func TestVetStrategy_CanHeal(t *testing.T) {
	s := NewVetStrategy("/tmp")

	tests := []struct {
		finding   Finding
		wantMatch bool
	}{
		{Finding{Message: "go vet violations"}, true},
		{Finding{Message: "errcheck findings"}, true},
		{Finding{Message: "unusedwrite detected"}, true},
		{Finding{Domain: "pipeline", Message: "vet failed"}, true},
		{Finding{Domain: "coverage", Message: "test coverage"}, false},
	}

	for _, tt := range tests {
		got := s.CanHeal(tt.finding)
		if got != tt.wantMatch {
			t.Errorf("CanHeal(%v) = %v, want %v", tt.finding, got, tt.wantMatch)
		}
	}
}

func TestVetStrategy_Heal_Clean(t *testing.T) {
	s := NewVetStrategy("/tmp")
	// Stub to return no output (clean)
	s.RunCmd = func(name string, args ...string) *exec.Cmd {
		return exec.Command("echo", "")
	}

	result := s.Heal(Finding{Domain: "vet", Message: "vet issues"}, false)
	if !result.Healed {
		t.Error("expected Healed = true for clean vet")
	}
}

// ─── CoverageStrategy Tests ───

func TestCoverageStrategy_Name(t *testing.T) {
	s := NewCoverageStrategy("/tmp")
	if s.Name() != "coverage" {
		t.Errorf("Name = %q, want coverage", s.Name())
	}
}

func TestCoverageStrategy_CanHeal(t *testing.T) {
	s := NewCoverageStrategy("/tmp")

	tests := []struct {
		finding   Finding
		wantMatch bool
	}{
		{Finding{Domain: "coverage", Message: "low"}, true},
		{Finding{Domain: "pipeline", Remediation: "Add tests to mirror"}, true},
		{Finding{Domain: "pipeline", Message: "vet issues"}, false},
	}

	for _, tt := range tests {
		got := s.CanHeal(tt.finding)
		if got != tt.wantMatch {
			t.Errorf("CanHeal(%v) = %v, want %v", tt.finding, got, tt.wantMatch)
		}
	}
}

func TestCoverageStrategy_Heal_MissingModule(t *testing.T) {
	s := NewCoverageStrategy("/tmp/nonexistent")
	result := s.Heal(Finding{
		Domain:  "coverage",
		Subject: "fakepkg",
		Message: "coverage too low",
	}, true)
	if result.Healed {
		t.Error("expected Healed = false for missing module")
	}
}

func TestCoverageStrategy_Heal_WithExports(t *testing.T) {
	// Create a temporary Go module with an exported func and no tests
	dir := t.TempDir()
	modDir := filepath.Join(dir, "internal", "testmod")
	_ = os.MkdirAll(modDir, 0o755)

	_ = os.WriteFile(filepath.Join(modDir, "api.go"), []byte(`package testmod

func PublicFunc() string { return "hello" }
func privateFunc() string { return "private" }
func AnotherPublic() int { return 42 }
`), 0o644)

	s := NewCoverageStrategy(dir)
	result := s.Heal(Finding{
		Domain:  "coverage",
		Subject: "testmod",
		Message: "coverage too low",
	}, true)

	if !strings.Contains(result.Action, "untested exported function") {
		t.Errorf("Action = %q, expected untested exports message", result.Action)
	}
}

func TestCoverageStrategy_Heal_AllTested(t *testing.T) {
	dir := t.TempDir()
	modDir := filepath.Join(dir, "internal", "testmod")
	_ = os.MkdirAll(modDir, 0o755)

	_ = os.WriteFile(filepath.Join(modDir, "api.go"), []byte(`package testmod

func Hello() string { return "hello" }
`), 0o644)

	_ = os.WriteFile(filepath.Join(modDir, "api_test.go"), []byte(`package testmod

import "testing"

func TestHello(t *testing.T) {
	Hello()
}
`), 0o644)

	s := NewCoverageStrategy(dir)
	result := s.Heal(Finding{
		Domain:  "coverage",
		Subject: "testmod",
		Message: "coverage too low",
	}, true)

	if !result.Healed {
		t.Errorf("expected Healed = true, got false. Action: %s", result.Action)
	}
}

// ─── CanonStrategy Tests ───

func TestCanonStrategy_Name(t *testing.T) {
	s := NewCanonStrategy("/tmp")
	if s.Name() != "canon" {
		t.Errorf("Name = %q, want canon", s.Name())
	}
}

func TestCanonStrategy_CanHeal(t *testing.T) {
	s := NewCanonStrategy("/tmp")

	tests := []struct {
		finding   Finding
		wantMatch bool
	}{
		{Finding{Domain: "canon", Message: "stale"}, true},
		{Finding{Message: "thoth memory drift"}, true},
		{Finding{Message: "journal not updated"}, true},
		{Finding{Domain: "coverage", Message: "test"}, false},
	}

	for _, tt := range tests {
		got := s.CanHeal(tt.finding)
		if got != tt.wantMatch {
			t.Errorf("CanHeal(%v) = %v, want %v", tt.finding, got, tt.wantMatch)
		}
	}
}

func TestCanonStrategy_Heal_NoMemory(t *testing.T) {
	s := NewCanonStrategy("/tmp/nonexistent")
	result := s.Heal(Finding{Domain: "canon", Message: "stale"}, true)
	if strings.Contains(result.Action, "would fix") {
		t.Error("expected no-op for missing .thoth/memory.yaml")
	}
}

// ─── Bridge Tests ───

func TestFromMaatReport_FiltersPassingAssessments(t *testing.T) {
	report := maat.NewReport([]maat.Assessment{
		{Domain: "coverage", Subject: "mirror", Verdict: maat.VerdictPass, FeatherWeight: 90},
		{Domain: "coverage", Subject: "brain", Verdict: maat.VerdictFail, FeatherWeight: 40, Message: "low", Remediation: "Add tests"},
		{Domain: "canon", Subject: "commit", Verdict: maat.VerdictWarning, FeatherWeight: 60, Message: "drift"},
	})

	findings := FromMaatReport(report)
	if len(findings) != 2 {
		t.Fatalf("got %d findings, want 2 (pass should be filtered)", len(findings))
	}

	if findings[0].Subject != "brain" {
		t.Errorf("findings[0].Subject = %q, want brain", findings[0].Subject)
	}
	if findings[0].Severity != "fail" {
		t.Errorf("findings[0].Severity = %q, want fail", findings[0].Severity)
	}
	if findings[1].Subject != "commit" {
		t.Errorf("findings[1].Subject = %q, want commit", findings[1].Subject)
	}
}

// ─── extractModule Tests ───

func TestExtractModule(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"mirror", "mirror"},
		{"mirror: 45.2% coverage", "mirror"},
		{"brain", "brain"},
		{"  ka  ", "ka"},
		{"UPPER", ""},     // Not valid Go package
		{"has space", ""}, // Not valid
		{"", ""},
	}

	for _, tt := range tests {
		got := extractModule(tt.input)
		if got != tt.want {
			t.Errorf("extractModule(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ─── Stub Strategy ───

type stubStrategy struct {
	name    string
	canHeal bool
	healFn  func(Finding, bool) HealResult
}

func (s *stubStrategy) Name() string           { return s.name }
func (s *stubStrategy) CanHeal(f Finding) bool { return s.canHeal }
func (s *stubStrategy) Heal(f Finding, d bool) HealResult {
	if s.healFn != nil {
		return s.healFn(f, d)
	}
	return HealResult{Strategy: s.name}
}
