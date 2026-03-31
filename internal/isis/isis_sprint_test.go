package isis

import (
	"os/exec"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

// ── FromMaatReport Tests ────────────────────────────────────────────────

func TestFromMaatReport_Empty(t *testing.T) {
	t.Parallel()
	report := &maat.Report{Assessments: []maat.Assessment{}}
	findings := FromMaatReport(report)
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for empty report, got %d", len(findings))
	}
}

func TestFromMaatReport_PassingSkipped(t *testing.T) {
	t.Parallel()
	report := &maat.Report{
		Assessments: []maat.Assessment{
			{Verdict: maat.VerdictPass, Subject: "guard", Message: "all good"},
		},
	}
	findings := FromMaatReport(report)
	if len(findings) != 0 {
		t.Errorf("passing assessments should be skipped, got %d findings", len(findings))
	}
}

func TestFromMaatReport_WarningAndFail(t *testing.T) {
	t.Parallel()
	report := &maat.Report{
		Assessments: []maat.Assessment{
			{Verdict: maat.VerdictWarning, Domain: maat.DomainCoverage, Subject: "hapi", Message: "below 85%", Remediation: "add tests", FeatherWeight: 72},
			{Verdict: maat.VerdictFail, Domain: maat.DomainPipeline, Subject: "CI", Message: "build failed", FeatherWeight: 0},
			{Verdict: maat.VerdictPass, Subject: "guard", Message: "ok"},
		},
	}
	findings := FromMaatReport(report)
	if len(findings) != 2 {
		t.Fatalf("expected 2 findings (skip pass), got %d", len(findings))
	}
	if findings[0].Subject != "hapi" {
		t.Errorf("first finding subject = %q, want 'hapi'", findings[0].Subject)
	}
	if findings[0].Weight != 72 {
		t.Errorf("weight = %d, want 72", findings[0].Weight)
	}
	if findings[1].Severity != "fail" {
		t.Errorf("second finding severity = %q, want 'fail'", findings[1].Severity)
	}
}

// ── LintStrategy Tests ──────────────────────────────────────────────────

func TestLintStrategy_Name_Sprint2(t *testing.T) {
	t.Parallel()
	s := NewLintStrategy("/tmp/project")
	if s.Name() != "lint" {
		t.Errorf("Name() = %q, want 'lint'", s.Name())
	}
}

func TestLintStrategy_CanHeal_Sprint2(t *testing.T) {
	t.Parallel()
	s := NewLintStrategy("/tmp")
	tests := []struct {
		finding Finding
		want    bool
	}{
		{Finding{Message: "gofmt check failed"}, true},
		{Finding{Message: "goimports needed"}, true},
		{Finding{Domain: "lint", Message: "anything"}, true},
		{Finding{Domain: "pipeline", Message: "lint failure"}, true},
		{Finding{Domain: "pipeline", Message: "format drift"}, true},
		{Finding{Message: "misspell detected"}, true},
		{Finding{Message: "coverage below 80%"}, false},
		{Finding{Domain: "coverage", Message: "test gap"}, false},
	}
	for _, tt := range tests {
		got := s.CanHeal(tt.finding)
		if got != tt.want {
			t.Errorf("CanHeal(%q/%q) = %v, want %v", tt.finding.Domain, tt.finding.Message, got, tt.want)
		}
	}
}

func TestLintStrategy_ResolveTarget_Sprint2(t *testing.T) {
	t.Parallel()
	s := NewLintStrategy("/project")

	tests := []struct {
		subject string
		want    string
	}{
		{"", "/project"},
		{"main.go", "/project/main.go"},
		{"guard", "/project/internal/guard"},
		{"some/path/thing", "/project"},
	}
	for _, tt := range tests {
		got := s.resolveTarget(Finding{Subject: tt.subject})
		if got != tt.want {
			t.Errorf("resolveTarget(%q) = %q, want %q", tt.subject, got, tt.want)
		}
	}
}

func TestLintStrategy_Heal_DryRun_Sprint2(t *testing.T) {
	t.Parallel()
	s := NewLintStrategy("/tmp/project")
	// Mock RunCmd to return no dirty files
	s.RunCmd = func(name string, args ...string) *exec.Cmd {
		return exec.Command("echo", "")
	}

	result := s.Heal(Finding{Message: "lint needed"}, true)
	if result.Strategy != "lint" {
		t.Errorf("Strategy = %q", result.Strategy)
	}
	if !result.DryRun {
		t.Error("DryRun should be true")
	}
}

func TestLintStrategy_Heal_Live(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	s := NewLintStrategy(tmp)
	// Mock RunCmd to simulate success
	s.RunCmd = func(name string, args ...string) *exec.Cmd {
		return exec.Command("echo", "ok")
	}

	result := s.Heal(Finding{Message: "lint needed"}, false)
	if !result.Healed {
		t.Errorf("expected Healed=true, got error: %v", result.Error)
	}
	if result.Action != "applied goimports + gofmt" {
		t.Errorf("Action = %q", result.Action)
	}
}

func TestLintStrategy_Heal_GoimportsFails(t *testing.T) {
	t.Parallel()
	s := NewLintStrategy("/tmp/project")
	s.RunCmd = func(name string, args ...string) *exec.Cmd {
		return exec.Command("false") // always fails
	}

	result := s.Heal(Finding{Message: "lint needed"}, false)
	if result.Healed {
		t.Error("should not be healed when goimports fails")
	}
	if result.Error == nil {
		t.Error("expected error when goimports fails")
	}
}

// ── VetStrategy Tests ───────────────────────────────────────────────────

func TestVetStrategy_Name_Sprint2(t *testing.T) {
	t.Parallel()
	s := NewVetStrategy("/tmp/project")
	if s.Name() != "vet" {
		t.Errorf("Name() = %q, want 'vet'", s.Name())
	}
}

func TestVetStrategy_CanHeal_Sprint2(t *testing.T) {
	t.Parallel()
	s := NewVetStrategy("/tmp")
	tests := []struct {
		finding Finding
		want    bool
	}{
		{Finding{Message: "go vet failed"}, true},
		{Finding{Domain: "govet", Message: "anything"}, true},
		{Finding{Message: "unusedwrite detected"}, true},
		{Finding{Message: "shadow variable"}, true},
		{Finding{Message: "errcheck violation"}, true},
		{Finding{Domain: "pipeline", Message: "vet failure"}, true},
		{Finding{Message: "coverage gap"}, false},
	}
	for _, tt := range tests {
		got := s.CanHeal(tt.finding)
		if got != tt.want {
			t.Errorf("CanHeal(%q/%q) = %v, want %v", tt.finding.Domain, tt.finding.Message, got, tt.want)
		}
	}
}

func TestVetStrategy_Heal_Clean_Sprint2(t *testing.T) {
	t.Parallel()
	s := NewVetStrategy("/tmp/project")
	// Mock RunCmd to return clean (no output)
	s.RunCmd = func(name string, args ...string) *exec.Cmd {
		return exec.Command("echo", "")
	}

	result := s.Heal(Finding{Message: "vet check"}, false)
	if !result.Healed {
		t.Error("clean vet should report healed=true")
	}
	if result.Action != "go vet clean — no violations" {
		t.Errorf("Action = %q", result.Action)
	}
}

func TestVetStrategy_Heal_WithViolations(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	s := NewVetStrategy(tmp)
	// Mock RunCmd to return vet violations via bash
	s.RunCmd = func(name string, args ...string) *exec.Cmd {
		return exec.Command("bash", "-c", `echo "main.go:10:5: unused variable"; echo "utils.go:22:3: shadow"`)
	}

	result := s.Heal(Finding{Message: "vet check"}, false)
	if result.Healed {
		t.Error("vet violations should not be healed (requires manual fix)")
	}
	if len(result.FilesChanged) != 2 {
		t.Errorf("expected 2 files with violations, got %d", len(result.FilesChanged))
	}
}

func TestVetStrategy_Heal_DryRun(t *testing.T) {
	t.Parallel()
	s := NewVetStrategy("/tmp/project")
	s.RunCmd = func(name string, args ...string) *exec.Cmd {
		return exec.Command("printf", "file.go:1:1: issue")
	}

	result := s.Heal(Finding{Message: "vet check"}, true)
	if result.DryRun != true {
		t.Error("DryRun should be true")
	}
}

// ── VetFinding struct ───────────────────────────────────────────────────

func TestVetFinding_Fields(t *testing.T) {
	t.Parallel()
	vf := VetFinding{
		File: "main.go",
		Line: 42,
	}
	if vf.File != "main.go" {
		t.Errorf("File = %q", vf.File)
	}
	if vf.Line != 42 {
		t.Errorf("Line = %d", vf.Line)
	}
}
