package maat

import (
	"testing"
)

// ── PipelineAssessor with mock runners ──────────────────────────────────

func TestPipelineAssess_LatestSuccess(t *testing.T) {
	pa := &PipelineAssessor{
		RunListRunner: func(count int) (string, error) {
			return "123\tcompleted\tsuccess\tCI\tmain\tabc123", nil
		},
	}

	assessments, err := pa.Assess()
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if len(assessments) < 2 {
		t.Fatalf("Expected >= 2 assessments, got %d", len(assessments))
	}
	if assessments[0].Verdict != VerdictPass {
		t.Errorf("Latest run should pass, got %q", assessments[0].Verdict)
	}
}

func TestPipelineAssess_LatestFailure_WithLog(t *testing.T) {
	pa := &PipelineAssessor{
		RunListRunner: func(count int) (string, error) {
			return "456\tcompleted\tfailure\tCI\tmain\tdef456", nil
		},
		RunLogRunner: func(runID string) (string, error) {
			return "golangci-lint: gofmt diff detected", nil
		},
	}

	assessments, err := pa.Assess()
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if assessments[0].Verdict != VerdictFail {
		t.Errorf("Failed run should be VerdictFail, got %q", assessments[0].Verdict)
	}
	if assessments[0].Remediation == "" {
		t.Error("Failed run should have remediation")
	}
}

func TestPipelineAssess_InProgress(t *testing.T) {
	pa := &PipelineAssessor{
		RunListRunner: func(count int) (string, error) {
			return "789\tin_progress\t\tCI\tmain\tghi789", nil
		},
	}

	assessments, err := pa.Assess()
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if assessments[0].Verdict != VerdictWarning {
		t.Errorf("In-progress should be warning, got %q", assessments[0].Verdict)
	}
}

func TestPipelineAssess_NoRuns(t *testing.T) {
	pa := &PipelineAssessor{
		RunListRunner: func(count int) (string, error) {
			return "", nil
		},
	}

	assessments, err := pa.Assess()
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if len(assessments) != 1 {
		t.Fatalf("Expected 1 assessment for no runs, got %d", len(assessments))
	}
	if assessments[0].Verdict != VerdictWarning {
		t.Errorf("No runs should warn, got %q", assessments[0].Verdict)
	}
}

func TestPipelineAssess_MultipleRuns_FailRate(t *testing.T) {
	pa := &PipelineAssessor{
		RunListRunner: func(count int) (string, error) {
			return "1\tcompleted\tsuccess\tCI\tmain\taaa\n" +
				"2\tcompleted\tfailure\tCI\tmain\tbbb\n" +
				"3\tcompleted\tfailure\tCI\tmain\tccc\n" +
				"4\tcompleted\tfailure\tCI\tmain\tddd\n" +
				"5\tcompleted\tsuccess\tCI\tmain\teee", nil
		},
		RunLogRunner: func(runID string) (string, error) {
			return "FAIL test/foo_test.go", nil
		},
	}

	assessments, err := pa.Assess()
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}

	// Find the summary assessment
	for _, a := range assessments {
		if a.Subject == "pipeline health" {
			if a.Verdict == VerdictPass {
				t.Error("60% failure rate should not pass")
			}
		}
	}
}

// ── CategorizeFailureLog ────────────────────────────────────────────────

func TestCategorizeFailureLog_Lint(t *testing.T) {
	detail := CategorizeFailureLog("123", "golangci-lint: File not gofmt-formatted")
	if detail.Category != FailureLint {
		t.Errorf("Category = %q, want lint", detail.Category)
	}
	if !detail.AutoFix {
		t.Error("Lint failures should be auto-fixable")
	}
}

func TestCategorizeFailureLog_Test(t *testing.T) {
	detail := CategorizeFailureLog("123", "--- FAIL: TestSomething")
	if detail.Category != FailureTest {
		t.Errorf("Category = %q, want test", detail.Category)
	}
}

func TestCategorizeFailureLog_Build(t *testing.T) {
	detail := CategorizeFailureLog("123", "cannot find package \"foo/bar\"")
	if detail.Category != FailureBuild {
		t.Errorf("Category = %q, want build", detail.Category)
	}
}

func TestCategorizeFailureLog_Infra(t *testing.T) {
	detail := CategorizeFailureLog("123", "rate limit exceeded")
	if detail.Category != FailureInfra {
		t.Errorf("Category = %q, want infra", detail.Category)
	}
}

func TestCategorizeFailureLog_Other(t *testing.T) {
	detail := CategorizeFailureLog("123", "something weird happened")
	if detail.Category != FailureOther {
		t.Errorf("Category = %q, want other", detail.Category)
	}
}

func TestCategorizeFailureLog_Empty(t *testing.T) {
	detail := CategorizeFailureLog("123", "")
	if detail.Category != FailureOther {
		t.Errorf("Category = %q, want other for empty", detail.Category)
	}
}

// ── extractFirstMatch ───────────────────────────────────────────────────

func TestExtractFirstMatch_Found(t *testing.T) {
	result := extractFirstMatch(lintPattern, "error: golangci-lint found issues")
	if result != "golangci-lint" {
		t.Errorf("extractFirstMatch = %q, want golangci-lint", result)
	}
}

func TestExtractFirstMatch_NotFound(t *testing.T) {
	result := extractFirstMatch(lintPattern, "no lint match here")
	if result != "unknown" {
		t.Errorf("extractFirstMatch should return 'unknown', got %q", result)
	}
}

// ── remediationForCategory ──────────────────────────────────────────────

func TestRemediationForCategory_AllCategories(t *testing.T) {
	tests := []struct {
		cat    FailureCategory
		expect string
	}{
		{FailureLint, "gofmt"},
		{FailureTest, "go test"},
		{FailureBuild, "go build"},
		{FailureInfra, "Retry"},
		{FailureOther, "logs"},
	}
	for _, tt := range tests {
		remediation := remediationForCategory(tt.cat)
		if remediation == "" {
			t.Errorf("No remediation for %s", tt.cat)
		}
	}
}
