package thoth

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// JournalSyncOptions configures the journal auto-sync.
type JournalSyncOptions struct {
	RepoRoot string
	Since    string // git log --since value (e.g. "24 hours ago")
	DryRun   bool
}

// JournalEntry is a single auto-generated journal entry from git history.
type JournalEntry struct {
	Number  int
	Date    string
	Title   string
	Commits []CommitInfo
}

// CommitInfo captures essential git commit data.
type CommitInfo struct {
	Hash    string
	Subject string
	Date    string
	Files   int
	Adds    int
	Dels    int
}

// SyncJournal harvests recent git commits and appends structured journal
// entries to .thoth/journal.md. This closes the "ghost transcript" gap —
// even if the IDE loses conversations, git commits become journal entries.
func SyncJournal(opts JournalSyncOptions) (int, error) {
	if opts.RepoRoot == "" {
		return 0, fmt.Errorf("thoth journal-sync: repo root required")
	}
	if opts.Since == "" {
		opts.Since = "24 hours ago"
	}

	journalPath := filepath.Join(opts.RepoRoot, ".thoth", "journal.md")

	// Read existing journal to find the last entry number
	lastEntry := findLastEntryNumber(journalPath)

	// Get recent commits
	commits, err := getRecentCommits(opts.RepoRoot, opts.Since)
	if err != nil {
		return 0, fmt.Errorf("thoth journal-sync: git log failed: %w", err)
	}
	if len(commits) == 0 {
		return 0, nil // nothing to sync
	}

	// Group commits into a single journal entry
	entry := JournalEntry{
		Number: lastEntry + 1,
		Date:   time.Now().Format("2006-01-02 15:04"),
		Title:  buildEntryTitle(commits),
	}
	entry.Commits = commits

	if opts.DryRun {
		fmt.Printf("Would append Entry %03d with %d commits\n", entry.Number, len(commits))
		return len(commits), nil
	}

	// Format and append to journal
	block := formatJournalEntry(entry)

	f, err := os.OpenFile(journalPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, fmt.Errorf("thoth journal-sync: open journal: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(block); err != nil {
		return 0, fmt.Errorf("thoth journal-sync: write journal: %w", err)
	}

	return len(commits), nil
}

// findLastEntryNumber parses journal.md to find the highest entry number.
func findLastEntryNumber(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}

	re := regexp.MustCompile(`## Entry (\d+)`)
	matches := re.FindAllStringSubmatch(string(data), -1)
	highest := 0
	for _, m := range matches {
		if n, err := strconv.Atoi(m[1]); err == nil && n > highest {
			highest = n
		}
	}
	return highest
}

// getRecentCommits queries git log for commits since the given timeframe.
func getRecentCommits(repoRoot, since string) ([]CommitInfo, error) {
	// Get commits with stats in a parseable format
	out, err := exec.Command("git", "-C", repoRoot, "log",
		"--since="+since,
		"--format=%H|%s|%aI",
		"--shortstat",
	).CombinedOutput()
	if err != nil {
		return nil, err
	}

	return parseGitLog(string(out)), nil
}

// parseGitLog extracts CommitInfo from git log --format=%H|%s|%aI --shortstat output.
func parseGitLog(output string) []CommitInfo {
	var commits []CommitInfo
	lines := strings.Split(strings.TrimSpace(output), "\n")

	var current *CommitInfo
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Commit line: hash|subject|date
		if parts := strings.SplitN(line, "|", 3); len(parts) == 3 && len(parts[0]) >= 40 {
			if current != nil {
				commits = append(commits, *current)
			}
			current = &CommitInfo{
				Hash:    parts[0][:8], // short hash
				Subject: parts[1],
				Date:    parts[2],
			}
			continue
		}

		// Stat line: "3 files changed, 150 insertions(+), 20 deletions(-)"
		if current != nil && strings.Contains(line, "changed") {
			current.Files, current.Adds, current.Dels = parseStatLine(line)
		}
	}
	if current != nil {
		commits = append(commits, *current)
	}

	return commits
}

// parseStatLine extracts numbers from a git --shortstat line.
func parseStatLine(line string) (files, adds, dels int) {
	re := regexp.MustCompile(`(\d+)`)
	matches := re.FindAllString(line, -1)
	if len(matches) >= 1 {
		files, _ = strconv.Atoi(matches[0])
	}
	if len(matches) >= 2 {
		adds, _ = strconv.Atoi(matches[1])
	}
	if len(matches) >= 3 {
		dels, _ = strconv.Atoi(matches[2])
	}
	return
}

// buildEntryTitle creates a descriptive title from the commit subjects.
func buildEntryTitle(commits []CommitInfo) string {
	if len(commits) == 1 {
		return fmt.Sprintf("\"%s\"", commits[0].Subject)
	}

	// Summarize: look for common prefixes like fix, feat, docs, etc.
	prefixes := map[string]int{}
	for _, c := range commits {
		subject := strings.ToLower(c.Subject)
		for _, p := range []string{"fix", "feat", "docs", "refactor", "test", "ci", "chore"} {
			if strings.HasPrefix(subject, p) {
				prefixes[p]++
				break
			}
		}
	}

	// Find dominant prefix
	dominant := ""
	maxCount := 0
	for p, count := range prefixes {
		if count > maxCount {
			dominant = p
			maxCount = count
		}
	}

	totalFiles := 0
	totalAdds := 0
	for _, c := range commits {
		totalFiles += c.Files
		totalAdds += c.Adds
	}

	if dominant != "" {
		return fmt.Sprintf("\"%d commits (%s-focused), %d files, +%d lines\"",
			len(commits), dominant, totalFiles, totalAdds)
	}
	return fmt.Sprintf("\"%d commits, %d files changed\"", len(commits), totalFiles)
}

// formatJournalEntry formats a JournalEntry as markdown for appending.
func formatJournalEntry(e JournalEntry) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("\n## Entry %03d — %s — %s (AUTO-SYNC)\n\n", e.Number, e.Date, e.Title))
	sb.WriteString("> 🤖 This entry was auto-generated by `thoth sync` from git history.\n\n")

	totalFiles, totalAdds, totalDels := 0, 0, 0
	for _, c := range e.Commits {
		totalFiles += c.Files
		totalAdds += c.Adds
		totalDels += c.Dels
	}

	sb.WriteString(fmt.Sprintf("**Summary**: %d commits, %d files changed, +%d/-%d lines.\n\n",
		len(e.Commits), totalFiles, totalAdds, totalDels))

	sb.WriteString("**Commits**:\n")
	for _, c := range e.Commits {
		sb.WriteString(fmt.Sprintf("- `%s` %s", c.Hash, c.Subject))
		if c.Files > 0 {
			sb.WriteString(fmt.Sprintf(" (%d files, +%d/-%d)", c.Files, c.Adds, c.Dels))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\n---\n")

	return sb.String()
}
