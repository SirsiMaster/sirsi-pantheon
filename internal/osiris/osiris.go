// Package osiris — Checkpoint Guardian
//
// Named after Osiris, the god of resurrection and the afterlife.
// Osiris guards against session loss by detecting uncommitted work,
// measuring session drift, and warning before session end.
//
// Rule A18 (Incremental Commits): Sessions should never end with
// large amounts of uncommitted work. Osiris enforces this.
//
// Architecture:
//
//	┌────────────┐      ┌───────────┐      ┌──────────────┐
//	│ Git Status  │─────▶│  Analyzer  │─────▶│  Checkpoint  │
//	│ (porcelain) │      │ (risk lvl) │      │  (warning)   │
//	└────────────┘      └───────────┘      └──────────────┘
package osiris

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// RiskLevel indicates how much uncommitted work is at risk.
type RiskLevel string

const (
	RiskNone     RiskLevel = "none"     // Clean tree
	RiskLow      RiskLevel = "low"      // 1-5 files changed
	RiskModerate RiskLevel = "moderate" // 6-15 files changed
	RiskHigh     RiskLevel = "high"     // 16-30 files changed
	RiskCritical RiskLevel = "critical" // 30+ files or 2+ hours since last commit
)

// Checkpoint represents the current state of uncommitted work.
type Checkpoint struct {
	// Git status
	UncommittedFiles  int           `json:"uncommitted_files"`
	UntrackedFiles    int           `json:"untracked_files"`
	StagedFiles       int           `json:"staged_files"`
	ModifiedFiles     int           `json:"modified_files"`
	DeletedFiles      int           `json:"deleted_files"`
	TotalChanges      int           `json:"total_changes"`
	LinesAdded        int           `json:"lines_added"`
	LinesDeleted      int           `json:"lines_deleted"`
	LastCommitTime    time.Time     `json:"last_commit_time"`
	LastCommitHash    string        `json:"last_commit_hash"`
	LastCommitMessage string        `json:"last_commit_message"`
	TimeSinceCommit   time.Duration `json:"time_since_commit"`

	// Risk assessment
	Risk    RiskLevel `json:"risk"`
	Warning string    `json:"warning,omitempty"`

	// Repo info
	Branch   string `json:"branch"`
	RepoRoot string `json:"repo_root"`
}

// Injectable command runner for testability.
var runCommand = defaultRunCommand

func defaultRunCommand(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// Assess evaluates the current repository state and returns a Checkpoint.
// If repoDir is empty, it uses the current working directory.
func Assess(repoDir string) (*Checkpoint, error) {
	if repoDir == "" {
		repoDir = "."
	}

	cp := &Checkpoint{}

	// Find repo root
	root, err := runCommand(repoDir, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("osiris: not a git repository: %w", err)
	}
	cp.RepoRoot = root

	// Get current branch
	branch, err := runCommand(root, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err == nil {
		cp.Branch = branch
	}

	// Parse git status (porcelain for machine readability)
	status, err := runCommand(root, "git", "status", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("osiris: git status failed: %w", err)
	}

	cp.parseStatus(status)

	// Get last commit time
	commitInfo, err := runCommand(root, "git", "log", "-1", "--format=%H|%s|%aI")
	if err == nil && commitInfo != "" {
		cp.parseLastCommit(commitInfo)
	}

	// Get diff stats (lines added/deleted)
	diffStat, err := runCommand(root, "git", "diff", "--stat", "--numstat")
	if err == nil {
		cp.parseDiffStat(diffStat)
	}

	// Assess risk
	cp.assessRisk()

	return cp, nil
}

// parseStatus parses `git status --porcelain` output.
func (cp *Checkpoint) parseStatus(status string) {
	if status == "" {
		return
	}

	for _, line := range strings.Split(status, "\n") {
		if len(line) < 2 {
			continue
		}

		x := line[0] // index status
		y := line[1] // worktree status

		switch {
		case x == '?' && y == '?':
			cp.UntrackedFiles++
		case x == 'D' || y == 'D':
			cp.DeletedFiles++
		case x != ' ' && x != '?':
			cp.StagedFiles++
			if y != ' ' {
				cp.ModifiedFiles++
			}
		case y == 'M':
			cp.ModifiedFiles++
		}

		cp.UncommittedFiles++
	}

	cp.TotalChanges = cp.UncommittedFiles
}

// parseLastCommit parses `git log -1 --format=%H|%s|%aI` output.
func (cp *Checkpoint) parseLastCommit(info string) {
	parts := strings.SplitN(info, "|", 3)
	if len(parts) < 3 {
		return
	}

	cp.LastCommitHash = parts[0][:minInt(7, len(parts[0]))]
	cp.LastCommitMessage = parts[1]
	if t, err := time.Parse(time.RFC3339, parts[2]); err == nil {
		cp.LastCommitTime = t
		cp.TimeSinceCommit = time.Since(t)
	}
}

// parseDiffStat parses `git diff --numstat` output.
func (cp *Checkpoint) parseDiffStat(stat string) {
	if stat == "" {
		return
	}
	for _, line := range strings.Split(stat, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		added, err1 := strconv.Atoi(fields[0])
		deleted, err2 := strconv.Atoi(fields[1])
		if err1 == nil && err2 == nil {
			cp.LinesAdded += added
			cp.LinesDeleted += deleted
		}
	}
}

// assessRisk determines the risk level based on uncommitted work.
func (cp *Checkpoint) assessRisk() {
	// Time-based escalation
	hoursSinceCommit := cp.TimeSinceCommit.Hours()

	switch {
	case cp.TotalChanges == 0:
		cp.Risk = RiskNone
	case cp.TotalChanges >= 30 || hoursSinceCommit >= 2:
		cp.Risk = RiskCritical
		cp.Warning = fmt.Sprintf("𓋹 OSIRIS WARNING: %d uncommitted files, %.0f min since last commit — commit now!",
			cp.TotalChanges, cp.TimeSinceCommit.Minutes())
	case cp.TotalChanges >= 16:
		cp.Risk = RiskHigh
		cp.Warning = fmt.Sprintf("𓋹 Osiris: %d uncommitted files — consider a checkpoint commit", cp.TotalChanges)
	case cp.TotalChanges >= 6:
		cp.Risk = RiskModerate
		cp.Warning = fmt.Sprintf("𓋹 Osiris: %d uncommitted files", cp.TotalChanges)
	default:
		cp.Risk = RiskLow
	}
}

// ShouldWarn returns true if the checkpoint warrants a user-visible warning.
func (cp *Checkpoint) ShouldWarn() bool {
	return cp.Risk == RiskHigh || cp.Risk == RiskCritical
}

// Summary returns a one-line summary for the menu bar.
func (cp *Checkpoint) Summary() string {
	if cp.TotalChanges == 0 {
		return "✅ Clean tree"
	}
	timePart := ""
	if !cp.LastCommitTime.IsZero() {
		timePart = fmt.Sprintf(" (last commit: %s ago)", formatDuration(cp.TimeSinceCommit))
	}
	return fmt.Sprintf("📄 %d files changed%s", cp.TotalChanges, timePart)
}

// StatusIcon returns an emoji for the current risk level.
func (cp *Checkpoint) StatusIcon() string {
	switch cp.Risk {
	case RiskNone:
		return "✅"
	case RiskLow:
		return "🟢"
	case RiskModerate:
		return "🟡"
	case RiskHigh:
		return "🟠"
	case RiskCritical:
		return "🔴"
	default:
		return "⚪"
	}
}

// FormatReport returns a multi-line report for terminal or notification.
func (cp *Checkpoint) FormatReport() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("𓋹 Osiris Checkpoint — %s\n", cp.StatusIcon()))
	sb.WriteString(fmt.Sprintf("  Branch:     %s\n", cp.Branch))
	sb.WriteString(fmt.Sprintf("  Repo:       %s\n", filepath.Base(cp.RepoRoot)))
	sb.WriteString(fmt.Sprintf("  Risk:       %s\n", cp.Risk))
	sb.WriteString(fmt.Sprintf("  Changes:    %d files (%d staged, %d modified, %d untracked, %d deleted)\n",
		cp.TotalChanges, cp.StagedFiles, cp.ModifiedFiles, cp.UntrackedFiles, cp.DeletedFiles))
	if cp.LinesAdded > 0 || cp.LinesDeleted > 0 {
		sb.WriteString(fmt.Sprintf("  Diff:       +%d / -%d lines\n", cp.LinesAdded, cp.LinesDeleted))
	}
	if !cp.LastCommitTime.IsZero() {
		sb.WriteString(fmt.Sprintf("  Last commit: %s (%s ago)\n", cp.LastCommitHash, formatDuration(cp.TimeSinceCommit)))
		sb.WriteString(fmt.Sprintf("  Message:    %s\n", cp.LastCommitMessage))
	}
	if cp.Warning != "" {
		sb.WriteString(fmt.Sprintf("\n  ⚠️  %s\n", cp.Warning))
	}
	return sb.String()
}

// formatDuration returns a human-friendly duration string.
func formatDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		if m > 0 {
			return fmt.Sprintf("%dh%dm", h, m)
		}
		return fmt.Sprintf("%dh", h)
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
