package maat

import (
	"testing"
)

// ── GenerateProof Tests ─────────────────────────────────────────────────

func TestGenerateProof(t *testing.T) {
	t.Parallel()
	cert, err := GenerateProof()
	if err != nil {
		t.Fatalf("GenerateProof error: %v", err)
	}
	if cert == nil {
		t.Fatal("cert should not be nil")
	}
	if cert.Entity == "" {
		t.Error("Entity should not be empty")
	}
	if cert.Version == "" {
		t.Error("Version should not be empty")
	}
	if cert.Platform == "" {
		t.Error("Platform should not be empty")
	}
	if cert.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
	if cert.WeightedCoverage <= 0 {
		t.Error("WeightedCoverage should be positive")
	}
	if cert.TestCount <= 0 {
		t.Error("TestCount should be positive")
	}
	if len(cert.Modules) == 0 {
		t.Error("Modules should not be empty")
	}
}

func TestGenerateProof_ModuleFields(t *testing.T) {
	t.Parallel()
	cert, _ := GenerateProof()
	for _, mod := range cert.Modules {
		if mod.Name == "" {
			t.Error("module Name should not be empty")
		}
		if mod.Coverage < 0 || mod.Coverage > 100 {
			t.Errorf("module %s Coverage = %f, should be 0-100", mod.Name, mod.Coverage)
		}
	}
}

func TestGenerateProof_RuleValidation(t *testing.T) {
	t.Parallel()
	cert, _ := GenerateProof()
	if !cert.RuleA16Validated {
		t.Error("RuleA16 should be validated")
	}
	if !cert.RuleA17Validated {
		t.Error("RuleA17 should be validated")
	}
}

func TestModuleStatus_Fields(t *testing.T) {
	t.Parallel()
	ms := ModuleStatus{
		Name:     "test_module",
		Coverage: 95.2,
		Mocked:   true,
	}
	if ms.Name != "test_module" {
		t.Errorf("Name = %q", ms.Name)
	}
	if ms.Coverage != 95.2 {
		t.Errorf("Coverage = %f", ms.Coverage)
	}
	if !ms.Mocked {
		t.Error("Mocked should be true")
	}
}

func TestHardeningCertificate_Fields(t *testing.T) {
	t.Parallel()
	cert := HardeningCertificate{
		Entity:           "Test Entity",
		Version:          "v1.0.0",
		WeightedCoverage: 86.2,
		TestCount:        1324,
	}
	if cert.Entity != "Test Entity" {
		t.Errorf("Entity = %q", cert.Entity)
	}
	if cert.TestCount != 1324 {
		t.Errorf("TestCount = %d", cert.TestCount)
	}
}

// ── ExportProof Tests ───────────────────────────────────────────────────

func TestExportProof(t *testing.T) {
	// ExportProof writes to stdout — just verify it doesn't panic
	ExportProof()
}

// ── Assessment.Format edge cases ────────────────────────────────────────

func TestAssessment_Format_WithRemediation(t *testing.T) {
	t.Parallel()
	a := Assessment{
		Domain:        DomainCoverage,
		Subject:       "hapi",
		Standard:      "85% coverage",
		Verdict:       VerdictWarning,
		FeatherWeight: 72,
		Message:       "Coverage below threshold",
		Remediation:   "Add tokenize tests",
	}
	s := a.Format()
	if s == "" {
		t.Error("Format should not be empty")
	}
	if !containsStr(s, "Fix:") {
		t.Error("warning with remediation should include 'Fix:' line")
	}
	if !containsStr(s, "72/100") {
		t.Error("should include feather weight")
	}
}

func TestAssessment_Format_PassNoRemediation(t *testing.T) {
	t.Parallel()
	a := Assessment{
		Domain:        DomainPipeline,
		Subject:       "CI",
		Verdict:       VerdictPass,
		FeatherWeight: 100,
		Message:       "All green",
		Remediation:   "Should be ignored for pass",
	}
	s := a.Format()
	if containsStr(s, "Fix:") {
		t.Error("passing verdict should NOT include remediation line")
	}
}

func TestAssessment_Format_FailWithRemediation(t *testing.T) {
	t.Parallel()
	a := Assessment{
		Domain:        DomainCanon,
		Subject:       "ADR-001",
		Verdict:       VerdictFail,
		FeatherWeight: 0,
		Message:       "Missing canon link",
		Remediation:   "Add ADR reference to commit",
	}
	s := a.Format()
	if !containsStr(s, "Fix:") {
		t.Error("failing verdict with remediation should include 'Fix:' line")
	}
	if !containsStr(s, "0/100") {
		t.Error("should include feather weight 0")
	}
}

// ── Verdict edge cases ──────────────────────────────────────────────────

func TestVerdict_Unknown(t *testing.T) {
	t.Parallel()
	v := Verdict(99)
	if v.String() != "unknown" {
		t.Errorf("Verdict(99).String() = %q, want 'unknown'", v.String())
	}
	if v.Icon() != "❓" {
		t.Errorf("Verdict(99).Icon() = %q, want '❓'", v.Icon())
	}
}

// ── CanonLink ───────────────────────────────────────────────────────────

func TestCanonLink_Fields(t *testing.T) {
	t.Parallel()
	cl := CanonLink{
		Feature:   "FastTokenize",
		Canon:     "ADR-009",
		Linked:    true,
		CommitSHA: "abc1234",
	}
	if cl.Feature != "FastTokenize" {
		t.Errorf("Feature = %q", cl.Feature)
	}
	if !cl.Linked {
		t.Error("Linked should be true")
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
