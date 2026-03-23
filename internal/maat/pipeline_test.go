package maat

import (
	"fmt"
	"testing"
)

// --- ParseRunList tests ---

func TestParseRunListBasic(t *testing.T) {
	input := "12345\tcompleted\tsuccess\tCI\tmain\tabc123\n67890\tcompleted\tfailure\tCI\tmain\tdef456"

	runs := ParseRunList(input)
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want 2", len(runs))
	}

	if runs[0].ID != "12345" || runs[0].Status != RunStatusSuccess {
		t.Errorf("run 0: id=%q status=%v", runs[0].ID, runs[0].Status)
	}
	if runs[1].ID != "67890" || runs[1].Status != RunStatusFailure {
		t.Errorf("run 1: id=%q status=%v", runs[1].ID, runs[1].Status)
	}
}

func TestParseRunListInProgress(t *testing.T) {
	input := "12345\tin_progress\t\tCI\tmain\tabc123"

	runs := ParseRunList(input)
	if len(runs) != 1 {
		t.Fatalf("got %d runs, want 1", len(runs))
	}
	if runs[0].Status != RunStatusInProgress {
		t.Errorf("status = %v, want in_progress", runs[0].Status)
	}
}

func TestParseRunListCancelled(t *testing.T) {
	input := "12345\tcompleted\tcanceled\tCI\tmain\tabc123"

	runs := ParseRunList(input)
	if len(runs) != 1 {
		t.Fatalf("got %d runs, want 1", len(runs))
	}
	if runs[0].Status != RunStatusCanceled {
		t.Errorf("status = %v, want canceled", runs[0].Status)
	}
}

func TestParseRunListEmpty(t *testing.T) {
	runs := ParseRunList("")
	if len(runs) != 0 {
		t.Errorf("empty input should produce 0 runs, got %d", len(runs))
	}
}

func TestParseRunListEmptyJSON(t *testing.T) {
	runs := ParseRunList("[]")
	if len(runs) != 0 {
		t.Errorf("empty JSON should produce 0 runs, got %d", len(runs))
	}
}

// --- CategorizeFailureLog tests ---

func TestCategorizeFailureLogLint(t *testing.T) {
	log := `Running golangci-lint run ./...
main.go:5:1: gofmt: File is not gofmt-ed (gofmt)
Error: Process completed with exit code 1.`

	detail := CategorizeFailureLog("123", log)
	if detail.Category != FailureLint {
		t.Errorf("category = %v, want lint", detail.Category)
	}
	if !detail.AutoFix {
		t.Error("lint failures should be auto-fixable")
	}
}

func TestCategorizeFailureLogTest(t *testing.T) {
	log := `--- FAIL: TestSomething (0.01s)
    foo_test.go:42: expected 1, got 2
FAIL	github.com/SirsiMaster/sirsi-anubis/internal/foo	0.234s`

	detail := CategorizeFailureLog("123", log)
	if detail.Category != FailureTest {
		t.Errorf("category = %v, want test", detail.Category)
	}
	if detail.AutoFix {
		t.Error("test failures should not be auto-fixable")
	}
}

func TestCategorizeFailureLogBuild(t *testing.T) {
	log := `internal/foo/bar.go:10:5: undefined: SomeFunc
Error: Process completed with exit code 2.`

	detail := CategorizeFailureLog("123", log)
	if detail.Category != FailureBuild {
		t.Errorf("category = %v, want build", detail.Category)
	}
}

func TestCategorizeFailureLogInfra(t *testing.T) {
	log := `Error: rate limit exceeded for API calls
Please wait before retrying.`

	detail := CategorizeFailureLog("123", log)
	if detail.Category != FailureInfra {
		t.Errorf("category = %v, want infra", detail.Category)
	}
}

func TestCategorizeFailureLogOther(t *testing.T) {
	log := `Something unexpected happened without any known pattern.`

	detail := CategorizeFailureLog("123", log)
	if detail.Category != FailureOther {
		t.Errorf("category = %v, want other", detail.Category)
	}
}

// --- PipelineAssessor tests ---

func TestPipelineAssessorAllGreen(t *testing.T) {
	pa := &PipelineAssessor{
		RunCount: 3,
		RunListRunner: func(count int) (string, error) {
			return "1\tcompleted\tsuccess\tCI\tmain\tabc\n2\tcompleted\tsuccess\tCI\tmain\tdef\n3\tcompleted\tsuccess\tCI\tmain\tghi", nil
		},
	}

	assessments, err := pa.Assess()
	if err != nil {
		t.Fatalf("Assess() error = %v", err)
	}

	// Should have latest run assessment + summary.
	if len(assessments) != 2 {
		t.Fatalf("got %d assessments, want 2", len(assessments))
	}

	// Latest run should pass.
	if assessments[0].Verdict != VerdictPass {
		t.Errorf("latest run verdict = %v, want pass", assessments[0].Verdict)
	}

	// Summary should pass.
	if assessments[1].Verdict != VerdictPass {
		t.Errorf("summary verdict = %v, want pass", assessments[1].Verdict)
	}
}

func TestPipelineAssessorLatestFailed(t *testing.T) {
	pa := &PipelineAssessor{
		RunCount: 3,
		RunListRunner: func(count int) (string, error) {
			return "1\tcompleted\tfailure\tCI\tmain\tabc\n2\tcompleted\tsuccess\tCI\tmain\tdef\n3\tcompleted\tsuccess\tCI\tmain\tghi", nil
		},
		RunLogRunner: func(runID string) (string, error) {
			return "golangci-lint: some error", nil
		},
	}

	assessments, err := pa.Assess()
	if err != nil {
		t.Fatalf("Assess() error = %v", err)
	}

	if assessments[0].Verdict != VerdictFail {
		t.Errorf("latest run verdict = %v, want fail", assessments[0].Verdict)
	}
}

func TestPipelineAssessorNoRuns(t *testing.T) {
	pa := &PipelineAssessor{
		RunListRunner: func(count int) (string, error) {
			return "", nil
		},
	}

	assessments, err := pa.Assess()
	if err != nil {
		t.Fatalf("Assess() error = %v", err)
	}

	if len(assessments) != 1 {
		t.Fatalf("got %d assessments, want 1", len(assessments))
	}
	if assessments[0].Verdict != VerdictWarning {
		t.Errorf("verdict = %v, want warning", assessments[0].Verdict)
	}
}

func TestPipelineAssessorRunListError(t *testing.T) {
	pa := &PipelineAssessor{
		RunListRunner: func(count int) (string, error) {
			return "", fmt.Errorf("gh not installed")
		},
	}

	_, err := pa.Assess()
	if err == nil {
		t.Error("should return error when runner fails")
	}
}

func TestPipelineAssessorDomain(t *testing.T) {
	pa := &PipelineAssessor{}
	if pa.Domain() != DomainPipeline {
		t.Errorf("domain = %v, want pipeline", pa.Domain())
	}
}

func TestPipelineAssessorHighFailRate(t *testing.T) {
	pa := &PipelineAssessor{
		RunCount: 5,
		RunListRunner: func(count int) (string, error) {
			return "1\tcompleted\tfailure\tCI\tmain\ta\n2\tcompleted\tfailure\tCI\tmain\tb\n3\tcompleted\tfailure\tCI\tmain\tc\n4\tcompleted\tsuccess\tCI\tmain\td\n5\tcompleted\tfailure\tCI\tmain\te", nil
		},
		RunLogRunner: func(runID string) (string, error) {
			return "FAIL	github.com/foo	0.1s", nil
		},
	}

	assessments, err := pa.Assess()
	if err != nil {
		t.Fatalf("Assess() error = %v", err)
	}

	// Summary should fail (80% failure rate).
	summary := assessments[len(assessments)-1]
	if summary.Verdict != VerdictFail {
		t.Errorf("summary verdict = %v, want fail (80%% failure rate)", summary.Verdict)
	}
}

// --- remediationForCategory tests ---

func TestRemediationForCategory(t *testing.T) {
	tests := []struct {
		cat  FailureCategory
		want string
	}{
		{FailureLint, "gofmt"},
		{FailureTest, "go test"},
		{FailureBuild, "go build"},
		{FailureInfra, "Retry"},
		{FailureOther, "logs"},
	}
	for _, tt := range tests {
		got := remediationForCategory(tt.cat)
		if !searchString(got, tt.want) {
			t.Errorf("remediation(%v) = %q, should contain %q", tt.cat, got, tt.want)
		}
	}
}
