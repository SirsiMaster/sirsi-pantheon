package isis

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// LintStrategy remediates formatting drift using goimports and gofmt.
// These are deterministic, safe auto-fixes — the gold standard of auto-healing.
type LintStrategy struct {
	ProjectRoot string

	// RunCmd is injectable for testing. Defaults to exec.Command.
	RunCmd func(name string, args ...string) *exec.Cmd
}

// NewLintStrategy creates a LintStrategy for the given project root.
func NewLintStrategy(projectRoot string) *LintStrategy {
	return &LintStrategy{
		ProjectRoot: projectRoot,
		RunCmd:      exec.Command,
	}
}

// Name returns the strategy name.
func (s *LintStrategy) Name() string { return "lint" }

// CanHeal returns true for lint-related findings (gofmt, goimports, misspell).
func (s *LintStrategy) CanHeal(finding Finding) bool {
	msg := strings.ToLower(finding.Message)
	domain := strings.ToLower(finding.Domain)

	// Match lint-related keywords
	for _, keyword := range []string{"gofmt", "goimports", "format", "lint", "misspell"} {
		if strings.Contains(msg, keyword) || strings.Contains(domain, keyword) {
			return true
		}
	}

	// Match pipeline domain with lint-related messages
	if domain == "pipeline" && (strings.Contains(msg, "lint") || strings.Contains(msg, "format")) {
		return true
	}

	return false
}

// Heal runs goimports -w and gofmt -w on the project.
func (s *LintStrategy) Heal(finding Finding, dryRun bool) HealResult {
	result := HealResult{
		Finding:  finding,
		Strategy: s.Name(),
		DryRun:   dryRun,
	}

	// Determine target: specific file from subject, or whole project
	target := s.resolveTarget(finding)

	if dryRun {
		// goimports -l lists files that would change
		changed := s.listDirtyFiles(target)
		result.FilesChanged = changed
		if len(changed) > 0 {
			result.Healed = true
			result.Action = fmt.Sprintf("would fix %d file(s) with goimports + gofmt", len(changed))
		} else {
			result.Action = "no formatting issues detected"
		}
		return result
	}

	// Run goimports -w
	cmd := s.RunCmd("goimports", "-w", target)
	cmd.Dir = s.ProjectRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		result.Error = fmt.Errorf("goimports: %w: %s", err, string(out))
		result.Action = "goimports failed"
		return result
	}

	// Run gofmt -w
	cmd = s.RunCmd("gofmt", "-w", target)
	cmd.Dir = s.ProjectRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		result.Error = fmt.Errorf("gofmt: %w: %s", err, string(out))
		result.Action = "gofmt failed"
		return result
	}

	result.Healed = true
	result.Action = "applied goimports + gofmt"
	return result
}

// resolveTarget determines the file/directory to lint.
func (s *LintStrategy) resolveTarget(finding Finding) string {
	subject := finding.Subject
	if subject == "" {
		return s.ProjectRoot
	}
	// If subject looks like a file path, use it directly
	if strings.HasSuffix(subject, ".go") {
		return filepath.Join(s.ProjectRoot, subject)
	}
	// If subject looks like a module name, target its directory
	if !strings.Contains(subject, "/") && !strings.Contains(subject, ".") {
		return filepath.Join(s.ProjectRoot, "internal", subject)
	}
	// Default: lint the whole project
	return s.ProjectRoot
}

// listDirtyFiles uses goimports -l to find files with formatting issues.
func (s *LintStrategy) listDirtyFiles(target string) []string {
	cmd := s.RunCmd("goimports", "-l", target)
	cmd.Dir = s.ProjectRoot
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files
}
