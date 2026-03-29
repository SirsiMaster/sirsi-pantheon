package isis

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"
)

// VetStrategy remediates go vet findings.
// Go vet issues are often structural and cannot be auto-fixed, so this
// strategy produces a structured report of violations for manual intervention.
type VetStrategy struct {
	ProjectRoot string

	// RunCmd is injectable for testing. Defaults to exec.Command.
	RunCmd func(name string, args ...string) *exec.Cmd
}

// NewVetStrategy creates a VetStrategy for the given project root.
func NewVetStrategy(projectRoot string) *VetStrategy {
	return &VetStrategy{
		ProjectRoot: projectRoot,
		RunCmd:      exec.Command,
	}
}

// Name returns the strategy name.
func (s *VetStrategy) Name() string { return "vet" }

// CanHeal returns true for vet-related findings.
func (s *VetStrategy) CanHeal(finding Finding) bool {
	msg := strings.ToLower(finding.Message)
	domain := strings.ToLower(finding.Domain)

	for _, keyword := range []string{"vet", "govet", "unusedwrite", "shadow", "errcheck"} {
		if strings.Contains(msg, keyword) || strings.Contains(domain, keyword) {
			return true
		}
	}

	if domain == "pipeline" && strings.Contains(msg, "vet") {
		return true
	}

	return false
}

// VetFinding is a parsed go vet violation.
type VetFinding struct {
	File    string
	Line    int
	Message string
}

// Heal runs go vet and reports violations. Go vet issues are informational —
// they require human judgment to fix, so this strategy reports rather than patches.
func (s *VetStrategy) Heal(finding Finding, dryRun bool) HealResult {
	result := HealResult{
		Finding:  finding,
		Strategy: s.Name(),
		DryRun:   dryRun,
	}

	violations := s.runVet()
	if len(violations) == 0 {
		result.Healed = true
		result.Action = "go vet clean — no violations"
		return result
	}

	var files []string
	seen := make(map[string]bool)
	for _, v := range violations {
		if !seen[v.File] {
			files = append(files, v.File)
			seen[v.File] = true
		}
	}

	result.FilesChanged = files
	if dryRun {
		result.Action = fmt.Sprintf("found %d vet violation(s) in %d file(s) — manual review needed", len(violations), len(files))
	} else {
		result.Action = fmt.Sprintf("identified %d vet violation(s) in %d file(s) — requires manual fix", len(violations), len(files))
	}
	// Vet issues can't be auto-fixed, so healed stays false
	return result
}

// runVet executes go vet ./... and parses the output.
func (s *VetStrategy) runVet() []VetFinding {
	cmd := s.RunCmd("go", "vet", "./...")
	cmd.Dir = s.ProjectRoot
	out, _ := cmd.CombinedOutput() // go vet returns non-zero on findings

	var findings []VetFinding
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		// go vet output format: file.go:line:col: message
		if parts := strings.SplitN(line, ":", 4); len(parts) >= 4 {
			findings = append(findings, VetFinding{
				File:    parts[0],
				Message: strings.TrimSpace(parts[3]),
			})
		}
	}
	return findings
}
