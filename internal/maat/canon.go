package maat

import (
	"bufio"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// CanonDocument is a recognized canon source in the project.
type CanonDocument struct {
	// Name is the short identifier (e.g., "ADR-001", "A14").
	Name string `json:"name"`

	// Path is the file path relative to the project root.
	Path string `json:"path"`
}

// DefaultCanonDocuments returns the recognized canon sources for Anubis.
func DefaultCanonDocuments() []CanonDocument {
	return []CanonDocument{
		{Name: "ADR-001", Path: "docs/ADR-001-FOUNDING-ARCHITECTURE.md"},
		{Name: "ADR-002", Path: "docs/ADR-002-KA-GHOST-DETECTION.md"},
		{Name: "ADR-003", Path: "docs/ADR-003-BUILD-IN-PUBLIC.md"},
		{Name: "ADR-004", Path: "docs/ADR-004-MAAT-QA-GOVERNANCE.md"},
		{Name: "ANUBIS_RULES", Path: "ANUBIS_RULES.md"},
		{Name: "SAFETY_DESIGN", Path: "docs/SAFETY_DESIGN.md"},
		{Name: "ARCHITECTURE_DESIGN", Path: "docs/ARCHITECTURE_DESIGN.md"},
		{Name: "CONTINUATION-PROMPT", Path: "docs/CONTINUATION-PROMPT.md"},
	}
}

// canonPatterns are regex patterns that indicate a canon reference in commit messages.
var canonPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)ADR[-‐]?\d{3}`),
	regexp.MustCompile(`(?i)ANUBIS_RULES`),
	regexp.MustCompile(`(?i)Rule\s+A\d+`),
	regexp.MustCompile(`(?i)SAFETY_DESIGN`),
	regexp.MustCompile(`(?i)ARCHITECTURE_DESIGN`),
	regexp.MustCompile(`(?i)CONTINUATION[-_]PROMPT`),
	regexp.MustCompile(`(?i)Refs:\s*\S+`),
	regexp.MustCompile(`(?i)Changelog:\s*\S+`),
}

// CanonAssessor assesses whether recent commits link to canon documents.
type CanonAssessor struct {
	// CommitCount is the number of recent commits to check.
	CommitCount int

	// ProjectRoot is the root directory of the git repository.
	ProjectRoot string

	// Runner executes git log and returns the output.
	// Defaults to running `git log` if nil.
	Runner func(count int) (string, error)
}

// Domain returns the quality domain for this assessor.
func (c *CanonAssessor) Domain() Domain {
	return DomainCanon
}

// Assess checks recent commits for canon references.
func (c *CanonAssessor) Assess() ([]Assessment, error) {
	count := c.CommitCount
	if count <= 0 {
		count = 10
	}

	output, err := c.getCommitLog(count)
	if err != nil {
		return nil, fmt.Errorf("get commit log: %w", err)
	}

	commits := ParseCommitLog(output)
	return c.evaluate(commits), nil
}

// getCommitLog runs git log or uses the custom runner.
func (c *CanonAssessor) getCommitLog(count int) (string, error) {
	if c.Runner != nil {
		return c.Runner(count)
	}

	cmd := exec.Command("git", "log", "--format=%H%n%s%n%b%n---END---",
		fmt.Sprintf("-n%d", count))
	if c.ProjectRoot != "" {
		cmd.Dir = c.ProjectRoot
	}

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git log: %w", err)
	}

	return string(out), nil
}

// CommitInfo holds parsed commit data for canon checking.
type CommitInfo struct {
	SHA     string   `json:"sha"`
	Subject string   `json:"subject"`
	Body    string   `json:"body"`
	Refs    []string `json:"refs,omitempty"`
}

// ParseCommitLog parses git log output into structured commit info.
func ParseCommitLog(output string) []CommitInfo {
	var commits []CommitInfo

	scanner := bufio.NewScanner(strings.NewReader(output))
	var current *CommitInfo
	var bodyLines []string
	lineNum := 0

	for scanner.Scan() {
		line := scanner.Text()

		if line == "---END---" {
			if current != nil {
				current.Body = strings.TrimSpace(strings.Join(bodyLines, "\n"))
				current.Refs = findCanonRefs(current.Subject + "\n" + current.Body)
				commits = append(commits, *current)
			}
			current = nil
			bodyLines = nil
			lineNum = 0
			continue
		}

		lineNum++
		switch lineNum {
		case 1:
			current = &CommitInfo{SHA: line}
		case 2:
			if current != nil {
				current.Subject = line
			}
		default:
			bodyLines = append(bodyLines, line)
		}
	}

	// Handle last commit if no trailing ---END---
	if current != nil {
		current.Body = strings.TrimSpace(strings.Join(bodyLines, "\n"))
		current.Refs = findCanonRefs(current.Subject + "\n" + current.Body)
		commits = append(commits, *current)
	}

	return commits
}

// findCanonRefs searches text for canon references.
func findCanonRefs(text string) []string {
	seen := make(map[string]bool)
	var refs []string

	for _, pattern := range canonPatterns {
		matches := pattern.FindAllString(text, -1)
		for _, m := range matches {
			normalized := strings.TrimSpace(m)
			if !seen[normalized] {
				refs = append(refs, normalized)
				seen[normalized] = true
			}
		}
	}

	return refs
}

// evaluate produces assessments for each commit.
func (c *CanonAssessor) evaluate(commits []CommitInfo) []Assessment {
	if len(commits) == 0 {
		return []Assessment{{
			Domain:        DomainCanon,
			Subject:       "commits",
			Standard:      "at least one commit to assess",
			Verdict:       VerdictWarning,
			FeatherWeight: 50,
			Message:       "no commits found to assess",
		}}
	}

	var assessments []Assessment
	linked := 0
	unlinked := 0

	for _, commit := range commits {
		shortSHA := commit.SHA
		if len(shortSHA) > 7 {
			shortSHA = shortSHA[:7]
		}

		a := Assessment{
			Domain:   DomainCanon,
			Subject:  fmt.Sprintf("%s %s", shortSHA, truncate(commit.Subject, 50)),
			Standard: "commit must reference canon (ADR, Rule, Refs:)",
		}

		if len(commit.Refs) > 0 {
			a.Verdict = VerdictPass
			a.FeatherWeight = 100
			a.Message = fmt.Sprintf("linked to: %s", strings.Join(commit.Refs, ", "))
			linked++
		} else {
			a.Verdict = VerdictWarning
			a.FeatherWeight = 30
			a.Message = "no canon reference found"
			a.Remediation = "Add Refs: footer to commit message (e.g., Refs: ADR-001, ANUBIS_RULES)"
			unlinked++
		}

		assessments = append(assessments, a)
	}

	// Summary assessment
	total := linked + unlinked
	pct := float64(linked) / float64(total) * 100
	summary := Assessment{
		Domain:   DomainCanon,
		Subject:  "canon linkage summary",
		Standard: "all commits should reference canon",
	}

	switch {
	case pct >= 80:
		summary.Verdict = VerdictPass
		summary.FeatherWeight = clampWeight(int(pct))
		summary.Message = fmt.Sprintf("%.0f%% of commits linked to canon (%d/%d)", pct, linked, total)
	case pct >= 50:
		summary.Verdict = VerdictWarning
		summary.FeatherWeight = clampWeight(int(pct))
		summary.Message = fmt.Sprintf("%.0f%% of commits linked to canon (%d/%d)", pct, linked, total)
		summary.Remediation = "Improve commit traceability per Rule A7"
	default:
		summary.Verdict = VerdictFail
		summary.FeatherWeight = clampWeight(int(pct))
		summary.Message = fmt.Sprintf("%.0f%% of commits linked to canon (%d/%d)", pct, linked, total)
		summary.Remediation = "Most commits lack canon references. Review Rule A7 (Commit Traceability)"
	}

	assessments = append(assessments, summary)
	return assessments
}

// truncate shortens a string to maxLen characters, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
