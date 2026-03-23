package maat

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// RunStatus represents the status of a CI pipeline run.
type RunStatus string

const (
	RunStatusSuccess    RunStatus = "success"
	RunStatusFailure    RunStatus = "failure"
	RunStatusInProgress RunStatus = "in_progress"
	RunStatusCanceled   RunStatus = "canceled"
	RunStatusUnknown    RunStatus = "unknown"
)

// FailureCategory classifies a CI failure for remediation.
type FailureCategory string

const (
	FailureLint  FailureCategory = "lint"
	FailureTest  FailureCategory = "test"
	FailureBuild FailureCategory = "build"
	FailureInfra FailureCategory = "infra"
	FailureOther FailureCategory = "other"
)

// PipelineRun is a parsed CI run from gh run list.
type PipelineRun struct {
	ID         string    `json:"id"`
	Status     RunStatus `json:"status"`
	Conclusion string    `json:"conclusion"`
	Name       string    `json:"name"`
	Branch     string    `json:"branch"`
	HeadSHA    string    `json:"head_sha"`
}

// FailureDetail provides categorized failure information.
type FailureDetail struct {
	RunID    string          `json:"run_id"`
	Category FailureCategory `json:"category"`
	Message  string          `json:"message"`
	AutoFix  bool            `json:"auto_fix"`
	FixCmd   string          `json:"fix_cmd,omitempty"`
}

// PipelineAssessor assesses CI pipeline health.
type PipelineAssessor struct {
	// RunCount is the number of recent runs to check.
	RunCount int

	// ProjectRoot is the root of the git repository.
	ProjectRoot string

	// RunListRunner executes gh run list and returns the output.
	// If nil, uses the real gh CLI.
	RunListRunner func(count int) (string, error)

	// RunLogRunner executes gh run view --log-failed and returns the output.
	// If nil, uses the real gh CLI.
	RunLogRunner func(runID string) (string, error)
}

// Domain returns the quality domain for this assessor.
func (p *PipelineAssessor) Domain() Domain {
	return DomainPipeline
}

// Assess checks recent CI pipeline runs and produces assessments.
func (p *PipelineAssessor) Assess() ([]Assessment, error) {
	count := p.RunCount
	if count <= 0 {
		count = 5
	}

	output, err := p.getRunList(count)
	if err != nil {
		return nil, fmt.Errorf("get run list: %w", err)
	}

	runs := ParseRunList(output)
	if len(runs) == 0 {
		return []Assessment{{
			Domain:        DomainPipeline,
			Subject:       "CI runs",
			Standard:      "at least one CI run should exist",
			Verdict:       VerdictWarning,
			FeatherWeight: 50,
			Message:       "no CI runs found (is gh CLI configured?)",
			Remediation:   "Run `gh auth login` to configure GitHub CLI",
		}}, nil
	}

	var assessments []Assessment

	// Assess the most recent run in detail.
	latest := runs[0]
	a := Assessment{
		Domain:   DomainPipeline,
		Subject:  fmt.Sprintf("run %s (%s)", latest.ID, latest.Branch),
		Standard: "CI must pass",
	}

	switch latest.Status {
	case RunStatusSuccess:
		a.Verdict = VerdictPass
		a.FeatherWeight = 100
		a.Message = fmt.Sprintf("latest CI run passed: %s", latest.Name)

	case RunStatusInProgress:
		a.Verdict = VerdictWarning
		a.FeatherWeight = 70
		a.Message = fmt.Sprintf("CI run in progress: %s", latest.Name)

	case RunStatusFailure:
		a.Verdict = VerdictFail
		a.FeatherWeight = 0
		a.Message = fmt.Sprintf("latest CI run FAILED: %s", latest.Name)

		// Try to get failure details.
		detail := p.categorizeFailure(latest.ID)
		if detail != nil {
			a.Message = fmt.Sprintf("CI FAILED (%s): %s", detail.Category, detail.Message)
			if detail.AutoFix {
				a.Remediation = fmt.Sprintf("Auto-fixable: run `%s`", detail.FixCmd)
			} else {
				a.Remediation = remediationForCategory(detail.Category)
			}
		} else {
			a.Remediation = fmt.Sprintf("Run `gh run view %s --log-failed` for details", latest.ID)
		}

	default:
		a.Verdict = VerdictWarning
		a.FeatherWeight = 50
		a.Message = fmt.Sprintf("CI run status: %s", latest.Status)
	}

	assessments = append(assessments, a)

	// Summary: track recent failure rate.
	failures := 0
	for _, run := range runs {
		if run.Status == RunStatusFailure {
			failures++
		}
	}

	summary := Assessment{
		Domain:   DomainPipeline,
		Subject:  "pipeline health",
		Standard: "CI should be consistently green",
	}

	failRate := float64(failures) / float64(len(runs)) * 100
	switch {
	case failures == 0:
		summary.Verdict = VerdictPass
		summary.FeatherWeight = 100
		summary.Message = fmt.Sprintf("all %d recent runs passed", len(runs))
	case failRate <= 40:
		summary.Verdict = VerdictWarning
		summary.FeatherWeight = clampWeight(100 - int(failRate*2))
		summary.Message = fmt.Sprintf("%d/%d recent runs failed (%.0f%%)", failures, len(runs), failRate)
		summary.Remediation = "Investigate recurring failures"
	default:
		summary.Verdict = VerdictFail
		summary.FeatherWeight = clampWeight(100 - int(failRate*2))
		summary.Message = fmt.Sprintf("%d/%d recent runs failed (%.0f%%)", failures, len(runs), failRate)
		summary.Remediation = "Pipeline is unhealthy — fix the root cause before shipping"
	}

	assessments = append(assessments, summary)
	return assessments, nil
}

// getRunList executes gh run list or uses the custom runner.
func (p *PipelineAssessor) getRunList(count int) (string, error) {
	if p.RunListRunner != nil {
		return p.RunListRunner(count)
	}

	cmd := exec.Command("gh", "run", "list",
		"--limit", fmt.Sprintf("%d", count),
		"--json", "databaseId,status,conclusion,name,headBranch,headSha",
	)
	if p.ProjectRoot != "" {
		cmd.Dir = p.ProjectRoot
	}

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("gh run list: %w", err)
	}

	return string(out), nil
}

// ParseRunList parses gh run list tab-separated output.
// Expected format from --json: JSON array.
// For simplicity we also support tab-separated fallback.
func ParseRunList(output string) []PipelineRun {
	output = strings.TrimSpace(output)
	if output == "" || output == "[]" {
		return nil
	}

	// Parse simple tab-separated format:
	// ID\tSTATUS\tCONCLUSION\tNAME\tBRANCH\tSHA
	var runs []PipelineRun
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "[" || line == "]" {
			continue
		}

		fields := strings.Split(line, "\t")
		if len(fields) >= 4 {
			run := PipelineRun{
				ID:   fields[0],
				Name: fields[3],
			}
			if len(fields) >= 5 {
				run.Branch = fields[4]
			}
			if len(fields) >= 6 {
				run.HeadSHA = fields[5]
			}

			// Parse status
			status := strings.ToLower(fields[1])
			conclusion := ""
			if len(fields) >= 3 {
				conclusion = strings.ToLower(fields[2])
			}
			run.Conclusion = conclusion

			switch {
			case status == "completed" && conclusion == "success":
				run.Status = RunStatusSuccess
			case status == "completed" && conclusion == "failure":
				run.Status = RunStatusFailure
			case status == "completed" && conclusion == "canceled":
				run.Status = RunStatusCanceled
			case status == "in_progress":
				run.Status = RunStatusInProgress
			default:
				run.Status = RunStatusUnknown
			}

			runs = append(runs, run)
		}
	}

	return runs
}

// categorizeFailure fetches failure logs and categorizes the failure.
func (p *PipelineAssessor) categorizeFailure(runID string) *FailureDetail {
	logOutput, err := p.getRunLog(runID)
	if err != nil {
		return nil
	}

	return CategorizeFailureLog(runID, logOutput)
}

// getRunLog fetches the failed log for a run.
func (p *PipelineAssessor) getRunLog(runID string) (string, error) {
	if p.RunLogRunner != nil {
		return p.RunLogRunner(runID)
	}

	cmd := exec.Command("gh", "run", "view", runID, "--log-failed")
	if p.ProjectRoot != "" {
		cmd.Dir = p.ProjectRoot
	}

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("gh run view --log-failed: %w", err)
	}

	return string(out), nil
}

// Failure detection patterns.
var (
	lintPattern  = regexp.MustCompile(`(?i)(golangci-lint|gofmt|goimports|misspell|govet|staticcheck)`)
	testPattern  = regexp.MustCompile(`(?i)(FAIL\s+\S+|--- FAIL:|panic:.*test)`)
	buildPattern = regexp.MustCompile(`(?i)(cannot find package|undefined:|could not import|compilation failed|go build)`)
	infraPattern = regexp.MustCompile(`(?i)(rate limit|timeout|connection refused|no space|permission denied)`)
)

// CategorizeFailureLog analyzes a failure log and categorizes it.
func CategorizeFailureLog(runID, logOutput string) *FailureDetail {
	detail := &FailureDetail{RunID: runID}

	switch {
	case lintPattern.MatchString(logOutput):
		detail.Category = FailureLint
		detail.Message = extractFirstMatch(lintPattern, logOutput)
		detail.AutoFix = true
		detail.FixCmd = "gofmt -w . && goimports -w . && golangci-lint run --fix ./..."

	case testPattern.MatchString(logOutput):
		detail.Category = FailureTest
		detail.Message = extractFirstMatch(testPattern, logOutput)

	case buildPattern.MatchString(logOutput):
		detail.Category = FailureBuild
		detail.Message = extractFirstMatch(buildPattern, logOutput)

	case infraPattern.MatchString(logOutput):
		detail.Category = FailureInfra
		detail.Message = extractFirstMatch(infraPattern, logOutput)

	default:
		detail.Category = FailureOther
		// Take the first non-empty line as the message.
		for _, line := range strings.Split(logOutput, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				detail.Message = truncate(line, 120)
				break
			}
		}
	}

	return detail
}

// extractFirstMatch returns the first regex match from the text.
func extractFirstMatch(re *regexp.Regexp, text string) string {
	match := re.FindString(text)
	if match == "" {
		return "unknown"
	}
	return match
}

// remediationForCategory returns generic remediation advice for a failure category.
func remediationForCategory(cat FailureCategory) string {
	switch cat {
	case FailureLint:
		return "Run `gofmt -w . && golangci-lint run --fix ./...`"
	case FailureTest:
		return "Run `go test ./...` locally to reproduce"
	case FailureBuild:
		return "Run `go build ./cmd/anubis/` to check compilation"
	case FailureInfra:
		return "Retry the run — this may be a transient infrastructure issue"
	default:
		return "Check the CI logs for details"
	}
}
