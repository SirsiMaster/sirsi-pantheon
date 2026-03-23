package maat

import (
	"fmt"
	"testing"
	"time"
)

// --- Verdict tests ---

func TestVerdictString(t *testing.T) {
	tests := []struct {
		v    Verdict
		want string
	}{
		{VerdictPass, "pass"},
		{VerdictWarning, "warning"},
		{VerdictFail, "fail"},
		{Verdict(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.v.String(); got != tt.want {
			t.Errorf("Verdict(%d).String() = %q, want %q", tt.v, got, tt.want)
		}
	}
}

func TestVerdictIcon(t *testing.T) {
	tests := []struct {
		v    Verdict
		want string
	}{
		{VerdictPass, "✅"},
		{VerdictWarning, "⚠️"},
		{VerdictFail, "❌"},
		{Verdict(99), "❓"},
	}
	for _, tt := range tests {
		if got := tt.v.Icon(); got != tt.want {
			t.Errorf("Verdict(%d).Icon() = %q, want %q", tt.v, got, tt.want)
		}
	}
}

// --- Assessment tests ---

func TestAssessmentFormat(t *testing.T) {
	a := Assessment{
		Domain:        DomainCoverage,
		Subject:       "cleaner",
		Standard:      "80% minimum",
		Verdict:       VerdictPass,
		FeatherWeight: 85,
		Message:       "cleaner: 85.0% coverage (threshold: 80%)",
	}

	got := a.Format()
	if got == "" {
		t.Error("Assessment.Format() returned empty string")
	}
	// Should contain the icon and the message.
	if !contains(got, "✅") {
		t.Error("passing assessment should contain ✅")
	}
	if !contains(got, "85/100") {
		t.Error("should contain feather weight")
	}
}

func TestAssessmentFormatWithRemediation(t *testing.T) {
	a := Assessment{
		Domain:        DomainCoverage,
		Subject:       "mapper",
		Standard:      "50% minimum",
		Verdict:       VerdictFail,
		FeatherWeight: 0,
		Message:       "mapper: no test files",
		Remediation:   "Add tests to internal/mapper/",
	}

	got := a.Format()
	if !contains(got, "Fix:") {
		t.Error("failing assessment should contain remediation")
	}
}

func TestAssessmentFormatNoRemediationOnPass(t *testing.T) {
	a := Assessment{
		Domain:        DomainCoverage,
		Subject:       "cleaner",
		Verdict:       VerdictPass,
		FeatherWeight: 90,
		Message:       "all good",
		Remediation:   "this should not appear",
	}

	got := a.Format()
	if contains(got, "Fix:") {
		t.Error("passing assessment should not show remediation")
	}
}

// --- Report tests ---

func TestNewReportEmpty(t *testing.T) {
	r := NewReport(nil)
	if r.OverallVerdict != VerdictPass {
		t.Errorf("empty report verdict = %v, want pass", r.OverallVerdict)
	}
	if r.OverallWeight != 100 {
		t.Errorf("empty report weight = %d, want 100", r.OverallWeight)
	}
}

func TestNewReportAllPass(t *testing.T) {
	assessments := []Assessment{
		{Verdict: VerdictPass, FeatherWeight: 90},
		{Verdict: VerdictPass, FeatherWeight: 80},
	}

	r := NewReport(assessments)
	if r.OverallVerdict != VerdictPass {
		t.Errorf("all-pass report verdict = %v, want pass", r.OverallVerdict)
	}
	if r.Passes != 2 {
		t.Errorf("passes = %d, want 2", r.Passes)
	}
	if r.OverallWeight != 85 {
		t.Errorf("average weight = %d, want 85", r.OverallWeight)
	}
}

func TestNewReportWithWarning(t *testing.T) {
	assessments := []Assessment{
		{Verdict: VerdictPass, FeatherWeight: 90},
		{Verdict: VerdictWarning, FeatherWeight: 50},
	}

	r := NewReport(assessments)
	if r.OverallVerdict != VerdictWarning {
		t.Errorf("report with warning verdict = %v, want warning", r.OverallVerdict)
	}
	if r.Warnings != 1 {
		t.Errorf("warnings = %d, want 1", r.Warnings)
	}
}

func TestNewReportWithFailure(t *testing.T) {
	assessments := []Assessment{
		{Verdict: VerdictPass, FeatherWeight: 90},
		{Verdict: VerdictWarning, FeatherWeight: 50},
		{Verdict: VerdictFail, FeatherWeight: 10},
	}

	r := NewReport(assessments)
	if r.OverallVerdict != VerdictFail {
		t.Errorf("report with failure verdict = %v, want fail", r.OverallVerdict)
	}
	if r.Failures != 1 {
		t.Errorf("failures = %d, want 1", r.Failures)
	}
	// Fail should dominate even when warnings exist.
	if r.OverallVerdict != VerdictFail {
		t.Error("fail should dominate over warning")
	}
}

func TestNewReportTimestamp(t *testing.T) {
	before := time.Now()
	r := NewReport([]Assessment{{Verdict: VerdictPass, FeatherWeight: 100}})
	after := time.Now()

	if r.AssessedAt.Before(before) || r.AssessedAt.After(after) {
		t.Error("assessed_at should be between before and after")
	}
}

// --- Weigh tests ---

type mockAssessor struct {
	domain      Domain
	assessments []Assessment
	err         error
}

func (m *mockAssessor) Assess() ([]Assessment, error) {
	return m.assessments, m.err
}

func (m *mockAssessor) Domain() Domain {
	return m.domain
}

func TestWeighSingleAssessor(t *testing.T) {
	a := &mockAssessor{
		domain: DomainCoverage,
		assessments: []Assessment{
			{Verdict: VerdictPass, FeatherWeight: 100, Domain: DomainCoverage},
		},
	}

	report, err := Weigh(a)
	if err != nil {
		t.Fatalf("Weigh() error = %v", err)
	}
	if report.Passes != 1 {
		t.Errorf("passes = %d, want 1", report.Passes)
	}
}

func TestWeighMultipleAssessors(t *testing.T) {
	a1 := &mockAssessor{
		domain: DomainPipeline,
		assessments: []Assessment{
			{Verdict: VerdictPass, FeatherWeight: 100, Domain: DomainPipeline},
		},
	}
	a2 := &mockAssessor{
		domain: DomainCoverage,
		assessments: []Assessment{
			{Verdict: VerdictWarning, FeatherWeight: 50, Domain: DomainCoverage},
		},
	}

	report, err := Weigh(a1, a2)
	if err != nil {
		t.Fatalf("Weigh() error = %v", err)
	}
	if len(report.Assessments) != 2 {
		t.Errorf("assessments = %d, want 2", len(report.Assessments))
	}
	if report.OverallVerdict != VerdictWarning {
		t.Errorf("verdict = %v, want warning", report.OverallVerdict)
	}
}

func TestWeighAssessorError(t *testing.T) {
	a := &mockAssessor{
		domain: DomainPipeline,
		err:    fmt.Errorf("gh not installed"),
	}

	report, err := Weigh(a)
	if err != nil {
		t.Fatalf("Weigh() should not return error for individual assessor failure, got %v", err)
	}
	if report.Failures != 1 {
		t.Errorf("failures = %d, want 1 (assessor error should be a failure)", report.Failures)
	}
}

func TestWeighNoAssessors(t *testing.T) {
	report, err := Weigh()
	if err != nil {
		t.Fatalf("Weigh() error = %v", err)
	}
	if report.OverallVerdict != VerdictPass {
		t.Errorf("empty weigh verdict = %v, want pass", report.OverallVerdict)
	}
	if report.OverallWeight != 100 {
		t.Errorf("empty weigh weight = %d, want 100", report.OverallWeight)
	}
}

// --- Helpers ---

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
