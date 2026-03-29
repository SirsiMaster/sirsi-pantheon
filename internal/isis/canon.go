package isis

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/thoth"
)

// CanonStrategy detects documentation drift and triggers Thoth sync
// to restore alignment between source code and project memory.
//
// Canon drift occurs when:
//   - memory.yaml identity fields are stale (module count, test count, etc.)
//   - journal.md hasn't been updated with recent commits
//   - CHANGELOG.md is missing entries for recent tags
type CanonStrategy struct {
	ProjectRoot string
}

// NewCanonStrategy creates a CanonStrategy.
func NewCanonStrategy(projectRoot string) *CanonStrategy {
	return &CanonStrategy{ProjectRoot: projectRoot}
}

// Name returns the strategy name.
func (s *CanonStrategy) Name() string { return "canon" }

// CanHeal returns true for canon-related findings.
func (s *CanonStrategy) CanHeal(finding Finding) bool {
	msg := strings.ToLower(finding.Message)
	domain := strings.ToLower(finding.Domain)

	return domain == "canon" ||
		strings.Contains(msg, "canon") ||
		strings.Contains(msg, "thoth") ||
		strings.Contains(msg, "memory.yaml") ||
		strings.Contains(msg, "journal") ||
		strings.Contains(msg, "changelog")
}

// Heal triggers Thoth sync to restore canon alignment.
func (s *CanonStrategy) Heal(finding Finding, dryRun bool) HealResult {
	result := HealResult{
		Finding:  finding,
		Strategy: s.Name(),
		DryRun:   dryRun,
	}

	// Check if memory.yaml exists
	memoryPath := filepath.Join(s.ProjectRoot, ".thoth", "memory.yaml")
	if _, err := os.Stat(memoryPath); os.IsNotExist(err) {
		result.Action = "no .thoth/memory.yaml found — cannot sync"
		return result
	}

	if dryRun {
		// Check if drift exists
		drift := s.detectDrift()
		if len(drift) == 0 {
			result.Healed = true
			result.Action = "canon is aligned — no drift detected"
		} else {
			result.Action = fmt.Sprintf("would fix %d canon drift(s): %s", len(drift), strings.Join(drift, ", "))
			result.FilesChanged = []string{".thoth/memory.yaml", ".thoth/journal.md"}
		}
		return result
	}

	// Run Thoth memory sync
	if err := thoth.Sync(thoth.SyncOptions{
		RepoRoot:   s.ProjectRoot,
		UpdateDate: true,
	}); err != nil {
		result.Error = fmt.Errorf("thoth memory sync: %w", err)
		result.Action = "memory sync failed"
		return result
	}

	// Run Thoth journal sync
	commitCount, err := thoth.SyncJournal(thoth.JournalSyncOptions{
		RepoRoot: s.ProjectRoot,
		Since:    "24 hours ago",
	})
	if err != nil {
		result.Error = fmt.Errorf("thoth journal sync: %w", err)
		result.Action = "journal sync failed"
		return result
	}

	result.Healed = true
	result.FilesChanged = []string{".thoth/memory.yaml"}
	if commitCount > 0 {
		result.FilesChanged = append(result.FilesChanged, ".thoth/journal.md")
		result.Action = fmt.Sprintf("synced memory.yaml + appended %d commit(s) to journal", commitCount)
	} else {
		result.Action = "synced memory.yaml (journal already up-to-date)"
	}

	return result
}

// detectDrift checks for common canon drift signals.
func (s *CanonStrategy) detectDrift() []string {
	var drift []string

	memoryPath := filepath.Join(s.ProjectRoot, ".thoth", "memory.yaml")
	info, err := os.Stat(memoryPath)
	if err != nil {
		return drift
	}

	// If memory.yaml is older than 24 hours, it's likely stale
	if time.Since(info.ModTime()) > 24*time.Hour {
		drift = append(drift, "memory.yaml >24h stale")
	}

	// Check if journal.md is missing recent commits
	journalPath := filepath.Join(s.ProjectRoot, ".thoth", "journal.md")
	if jInfo, err := os.Stat(journalPath); err == nil {
		if time.Since(jInfo.ModTime()) > 48*time.Hour {
			drift = append(drift, "journal.md >48h stale")
		}
	}

	return drift
}
