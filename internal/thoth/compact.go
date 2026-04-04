package thoth

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/stele"
)

// CompactOptions configures the compact behavior.
type CompactOptions struct {
	RepoRoot string
	Summary  string // free-text session summary
	MaxAge   int    // days to retain journal entries (0 = no pruning)
	MaxKeep  int    // max journal entries to keep (0 = no limit)
}

// Compact persists session decisions into memory.yaml and creates a journal entry.
// This is designed to be called before context compression (e.g., /compact)
// so that key decisions survive across sessions.
func Compact(opts CompactOptions) error {
	if opts.RepoRoot == "" {
		return fmt.Errorf("thoth compact: repo root required")
	}
	if strings.TrimSpace(opts.Summary) == "" {
		return fmt.Errorf("thoth compact: summary required")
	}

	thothDir := filepath.Join(opts.RepoRoot, ".thoth")
	memoryPath := filepath.Join(thothDir, "memory.yaml")
	journalPath := filepath.Join(thothDir, "journal.md")

	// Verify .thoth directory exists
	if _, err := os.Stat(thothDir); os.IsNotExist(err) {
		return fmt.Errorf("thoth compact: .thoth directory not found — run 'thoth init' first")
	}

	// Step 1: Append decisions to memory.yaml
	if err := appendSessionDecisions(memoryPath, opts.Summary); err != nil {
		return fmt.Errorf("thoth compact: update memory: %w", err)
	}

	// Step 2: Create journal entry
	if err := appendCompactEntry(journalPath, opts.Summary); err != nil {
		return fmt.Errorf("thoth compact: update journal: %w", err)
	}

	// Step 3: Prune if requested
	if opts.MaxAge > 0 || opts.MaxKeep > 0 {
		_, err := PruneJournal(PruneOptions{
			RepoRoot: opts.RepoRoot,
			MaxAge:   opts.MaxAge,
			MaxKeep:  opts.MaxKeep,
		})
		if err != nil {
			return fmt.Errorf("thoth compact: prune journal: %w", err)
		}
	}

	stele.Inscribe("thoth", stele.TypeThothCompact, opts.RepoRoot, map[string]string{
		"summary": opts.Summary,
	})
	return nil
}

const sessionDecisionsHeader = "## Session Decisions"

// appendSessionDecisions adds decision lines under the "## Session Decisions" section.
func appendSessionDecisions(memoryPath, summary string) error {
	data, err := os.ReadFile(memoryPath)
	if err != nil {
		return err
	}

	content := string(data)
	datestamp := time.Now().Format("2006-01-02")

	// Build the new decision lines
	lines := strings.Split(strings.TrimSpace(summary), "\n")
	var newLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Strip leading "- " if present
		line = strings.TrimPrefix(line, "- ")
		newLines = append(newLines, fmt.Sprintf("# %s: %s", datestamp, line))
	}
	newContent := strings.Join(newLines, "\n")

	// Check if section already exists
	if strings.Contains(content, sessionDecisionsHeader) {
		// Append after the existing section header
		idx := strings.Index(content, sessionDecisionsHeader)
		afterHeader := idx + len(sessionDecisionsHeader)
		content = content[:afterHeader] + "\n" + newContent + content[afterHeader:]
	} else {
		// Add section at the end
		content = strings.TrimRight(content, "\n") + "\n\n" + sessionDecisionsHeader + "\n" + newContent + "\n"
	}

	return os.WriteFile(memoryPath, []byte(content), 0644)
}

// appendCompactEntry creates a journal entry marked (COMPACT).
func appendCompactEntry(journalPath, summary string) error {
	entryNum := findLastEntryNumber(journalPath) + 1
	now := time.Now()

	entry := fmt.Sprintf("\n## Entry %03d — %s — Session Compact (COMPACT)\n\n",
		entryNum, now.Format("2006-01-02 15:04"))
	entry += "> Persisted via `thoth compact` before context compression.\n\n"
	entry += "**Decisions**:\n"

	for _, line := range strings.Split(strings.TrimSpace(summary), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			entry += fmt.Sprintf("- %s\n", strings.TrimPrefix(line, "- "))
		}
	}
	entry += "\n---\n"

	f, err := os.OpenFile(journalPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(entry)
	return err
}

// PruneOptions configures journal pruning.
type PruneOptions struct {
	RepoRoot string
	MaxAge   int // days; 0 = no age limit
	MaxKeep  int // entries; 0 = no count limit
}

// PruneJournal removes old entries from journal.md. Returns count of removed entries.
func PruneJournal(opts PruneOptions) (int, error) {
	journalPath := filepath.Join(opts.RepoRoot, ".thoth", "journal.md")
	data, err := os.ReadFile(journalPath)
	if err != nil {
		return 0, fmt.Errorf("thoth prune: %w", err)
	}

	content := string(data)

	// Split into header and entries
	// Header = everything before the first "## Entry"
	entryRe := regexp.MustCompile(`(?m)^## Entry \d+`)
	firstIdx := entryRe.FindStringIndex(content)
	if firstIdx == nil {
		return 0, nil // no entries to prune
	}

	header := content[:firstIdx[0]]
	entriesSection := content[firstIdx[0]:]

	// Parse individual entries
	entryLocs := entryRe.FindAllStringIndex(entriesSection, -1)
	type parsedEntry struct {
		text   string
		number int
		date   time.Time
	}

	var entries []parsedEntry
	dateRe := regexp.MustCompile(`## Entry (\d+) — (\d{4}-\d{2}-\d{2})`)

	for i, loc := range entryLocs {
		end := len(entriesSection)
		if i+1 < len(entryLocs) {
			end = entryLocs[i+1][0]
		}
		text := entriesSection[loc[0]:end]

		var num int
		var date time.Time
		if m := dateRe.FindStringSubmatch(text); len(m) >= 3 {
			num, _ = strconv.Atoi(m[1])
			date, _ = time.Parse("2006-01-02", m[2])
		}

		entries = append(entries, parsedEntry{text: text, number: num, date: date})
	}

	if len(entries) == 0 {
		return 0, nil
	}

	// Filter entries
	var kept []parsedEntry
	now := time.Now()
	removed := 0

	for _, e := range entries {
		// Check age
		if opts.MaxAge > 0 && !e.date.IsZero() {
			age := now.Sub(e.date)
			if age > time.Duration(opts.MaxAge)*24*time.Hour {
				removed++
				continue
			}
		}
		kept = append(kept, e)
	}

	// Check count limit (keep most recent)
	if opts.MaxKeep > 0 && len(kept) > opts.MaxKeep {
		excess := len(kept) - opts.MaxKeep
		removed += excess
		kept = kept[excess:]
	}

	if removed == 0 {
		return 0, nil
	}

	// Rebuild file
	var sb strings.Builder
	sb.WriteString(header)
	for _, e := range kept {
		sb.WriteString(e.text)
	}

	return removed, os.WriteFile(journalPath, []byte(sb.String()), 0644)
}
