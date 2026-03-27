// Package horus — publish.go
//
// Horus Auto-Publish: Build-in-Public automation.
// Takes session data and generates/updates the public HTML artifacts:
//   - docs/build-log.html — session summaries with metrics
//   - docs/case-studies.html — newest stories at top, timestamped
//
// Can be triggered by:
//   - Makefile target: `make publish`
//   - Pre-push hook addition
//   - Menu bar "Publish" action
//
// Named after Horus: the All-Seeing Eye publishes what it sees.
package horus

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PublishConfig configures the Horus auto-publish system.
type PublishConfig struct {
	RepoRoot      string // Root of the repository
	JournalPath   string // Path to .thoth/journal.md
	BuildLogPath  string // Path to docs/build-log.html
	CaseStudyPath string // Path to docs/case-studies.html
	CaseStudyDir  string // Path to docs/case-studies/ directory
}

// PublishResult contains the outcome of a publish operation.
type PublishResult struct {
	BuildLogUpdated  bool   `json:"build_log_updated"`
	CaseStudyUpdated bool   `json:"case_study_updated"`
	EntriesAdded     int    `json:"entries_added"`
	Timestamp        string `json:"timestamp"`
	Error            string `json:"error,omitempty"`
}

// SessionEntry is a structured representation of a session for publishing.
type SessionEntry struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Date      string    `json:"date"`
	Summary   string    `json:"summary"`
	Metrics   []string  `json:"metrics,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// CaseStudyEntry is a structured case study for the public page.
type CaseStudyEntry struct {
	Title     string    `json:"title"`
	Date      string    `json:"date"`
	Body      string    `json:"body"`
	Tags      []string  `json:"tags,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// DefaultPublishConfig returns standard paths relative to repo root.
func DefaultPublishConfig(repoRoot string) PublishConfig {
	return PublishConfig{
		RepoRoot:      repoRoot,
		JournalPath:   filepath.Join(repoRoot, ".thoth", "journal.md"),
		BuildLogPath:  filepath.Join(repoRoot, "docs", "build-log.html"),
		CaseStudyPath: filepath.Join(repoRoot, "docs", "case-studies.html"),
		CaseStudyDir:  filepath.Join(repoRoot, "docs", "case-studies"),
	}
}

// Publish executes the auto-publish pipeline.
// It reads session data and updates the public HTML artifacts.
func Publish(cfg PublishConfig) (*PublishResult, error) {
	result := &PublishResult{
		Timestamp: time.Now().Format(time.RFC3339),
	}

	if cfg.RepoRoot == "" {
		return nil, fmt.Errorf("horus publish: repo root is required")
	}

	// Collect case studies from markdown files
	studies, err := collectCaseStudies(cfg.CaseStudyDir)
	if err == nil && len(studies) > 0 {
		if writeErr := writeCaseStudyHTML(cfg.CaseStudyPath, studies); writeErr != nil {
			result.Error = fmt.Sprintf("case study write: %v", writeErr)
		} else {
			result.CaseStudyUpdated = true
			result.EntriesAdded += len(studies)
		}
	}

	// Update build log with latest session data
	sessions, err := collectSessionEntries(cfg.JournalPath)
	if err == nil && len(sessions) > 0 {
		if err := writeBuildLogHTML(cfg.BuildLogPath, sessions); err != nil {
			result.Error = fmt.Sprintf("build log write: %v", err)
		} else {
			result.BuildLogUpdated = true
			result.EntriesAdded += len(sessions)
		}
	}

	return result, nil
}

// collectCaseStudies reads .md files from the case studies directory.
func collectCaseStudies(dir string) ([]CaseStudyEntry, error) {
	if dir == "" {
		return nil, fmt.Errorf("case study dir not set")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var studies []CaseStudyEntry
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}

		study := parseCaseStudyMD(entry.Name(), string(data))
		studies = append(studies, study)
	}

	return studies, nil
}

// parseCaseStudyMD extracts a CaseStudyEntry from markdown content.
func parseCaseStudyMD(filename, content string) CaseStudyEntry {
	lines := strings.Split(content, "\n")
	title := strings.TrimSuffix(filename, ".md")
	body := content
	date := time.Now().Format("2006-01-02")

	// Extract title from first H1
	for _, line := range lines {
		if strings.HasPrefix(line, "# ") {
			title = strings.TrimPrefix(line, "# ")
			break
		}
	}

	// Extract date from metadata if present
	for _, line := range lines {
		if strings.HasPrefix(line, "**Date") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				date = strings.TrimSpace(parts[1])
				date = strings.TrimSuffix(date, "**")
				date = strings.TrimSpace(date)
			}
			break
		}
	}

	return CaseStudyEntry{
		Title:     title,
		Date:      date,
		Body:      body,
		Timestamp: time.Now(),
	}
}

// collectSessionEntries reads the Thoth journal and extracts session entries.
func collectSessionEntries(journalPath string) ([]SessionEntry, error) {
	if journalPath == "" {
		return nil, fmt.Errorf("journal path not set")
	}

	data, err := os.ReadFile(journalPath)
	if err != nil {
		return nil, err
	}

	return parseJournalSessions(string(data)), nil
}

// parseJournalSessions extracts session entries from the Thoth journal.
func parseJournalSessions(content string) []SessionEntry {
	var sessions []SessionEntry
	lines := strings.Split(content, "\n")
	var current *SessionEntry
	var bodyLines []string

	for _, line := range lines {
		// Session header: ## Session N: Title
		if strings.HasPrefix(line, "## Session") || strings.HasPrefix(line, "## session") {
			// Save previous session
			if current != nil {
				current.Summary = strings.TrimSpace(strings.Join(bodyLines, "\n"))
				sessions = append(sessions, *current)
			}

			current = &SessionEntry{Timestamp: time.Now()}
			bodyLines = nil

			// Parse "## Session 18: Title" or "## Session 18 — Title"
			header := strings.TrimPrefix(line, "## ")
			header = strings.TrimPrefix(header, "Session ")
			header = strings.TrimPrefix(header, "session ")

			// Split on ":" or "—"
			var numStr, title string
			if idx := strings.IndexAny(header, ":—"); idx > 0 {
				numStr = strings.TrimSpace(header[:idx])
				title = strings.TrimSpace(header[idx+1:])
				// Handle "—" which is multi-byte
				if strings.HasPrefix(header[idx:], "—") {
					title = strings.TrimSpace(header[idx+3:]) // "—" is 3 bytes
				}
			} else {
				numStr = strings.TrimSpace(header)
			}

			if n := parseInt(numStr); n > 0 {
				current.Number = n
			}
			current.Title = title
			continue
		}

		// Date line
		if current != nil && strings.HasPrefix(line, "**Date") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				current.Date = strings.TrimSpace(strings.Trim(parts[1], "* "))
			}
			continue
		}

		if current != nil {
			bodyLines = append(bodyLines, line)
		}
	}

	// Save last session
	if current != nil {
		current.Summary = strings.TrimSpace(strings.Join(bodyLines, "\n"))
		sessions = append(sessions, *current)
	}

	return sessions
}

func parseInt(s string) int {
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		} else {
			break
		}
	}
	return n
}

// ── HTML Generation ──────────────────────────────────────────────────────

// writeBuildLogHTML generates the build-log.html file.
func writeBuildLogHTML(path string, sessions []SessionEntry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	var sb strings.Builder
	sb.WriteString(buildLogHTMLHeader())

	// Write sessions newest first
	for i := len(sessions) - 1; i >= 0; i-- {
		s := sessions[i]
		sb.WriteString(fmt.Sprintf(`  <article class="session-entry">
    <h2>Session %d: %s</h2>
    <time>%s</time>
    <div class="summary">%s</div>
  </article>
`, s.Number, escapeHTML(s.Title), s.Date, escapeHTML(truncate(s.Summary, 500))))
	}

	sb.WriteString(buildLogHTMLFooter())
	return os.WriteFile(path, []byte(sb.String()), 0o644)
}

// writeCaseStudyHTML generates the case-studies.html file.
func writeCaseStudyHTML(path string, studies []CaseStudyEntry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	var sb strings.Builder
	sb.WriteString(caseStudyHTMLHeader())

	// Newest first
	for i := len(studies) - 1; i >= 0; i-- {
		s := studies[i]
		sb.WriteString(fmt.Sprintf(`  <article class="case-study">
    <h2>%s</h2>
    <time>%s</time>
    <div class="body">%s</div>
  </article>
`, escapeHTML(s.Title), s.Date, escapeHTML(truncate(s.Body, 1000))))
	}

	sb.WriteString(caseStudyHTMLFooter())
	return os.WriteFile(path, []byte(sb.String()), 0o644)
}

func buildLogHTMLHeader() string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Pantheon Build Log — Building in Public</title>
  <meta name="description" content="Real-time build log of the Sirsi Pantheon development journey.">
  <style>
    :root {
      --gold: #C8A951; --black: #0F0F0F; --lapis: #1A1A5E;
      --surface: #1a1a2e; --text: #e0e0e0;
    }
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body {
      font-family: 'Inter', system-ui, sans-serif;
      background: var(--black); color: var(--text);
      max-width: 900px; margin: 0 auto; padding: 2rem;
    }
    header { text-align: center; margin-bottom: 3rem; }
    header h1 { color: var(--gold); font-size: 2rem; }
    header p { color: #888; margin-top: 0.5rem; }
    .session-entry {
      background: var(--surface); border-radius: 12px;
      padding: 1.5rem; margin-bottom: 1.5rem;
      border-left: 4px solid var(--gold);
    }
    .session-entry h2 { color: var(--gold); font-size: 1.2rem; margin-bottom: 0.5rem; }
    .session-entry time { color: #888; font-size: 0.85rem; }
    .session-entry .summary {
      margin-top: 0.75rem; line-height: 1.6;
      white-space: pre-wrap; font-size: 0.9rem;
    }
    footer { text-align: center; color: #666; margin-top: 3rem; font-size: 0.8rem; }
  </style>
</head>
<body>
  <header>
    <h1>𓂀 Pantheon Build Log</h1>
    <p>Building in Public — Every Session Documented</p>
  </header>
  <main>
`
}

func buildLogHTMLFooter() string {
	return fmt.Sprintf(`  </main>
  <footer>
    <p>Auto-generated by 𓂀 Horus at %s</p>
  </footer>
</body>
</html>
`, time.Now().Format("2006-01-02 15:04"))
}

func caseStudyHTMLHeader() string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Pantheon Case Studies</title>
  <meta name="description" content="Real-world case studies from the Sirsi Pantheon ecosystem.">
  <style>
    :root {
      --gold: #C8A951; --black: #0F0F0F; --lapis: #1A1A5E;
      --surface: #1a1a2e; --text: #e0e0e0;
    }
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body {
      font-family: 'Inter', system-ui, sans-serif;
      background: var(--black); color: var(--text);
      max-width: 900px; margin: 0 auto; padding: 2rem;
    }
    header { text-align: center; margin-bottom: 3rem; }
    header h1 { color: var(--gold); font-size: 2rem; }
    header p { color: #888; margin-top: 0.5rem; }
    .case-study {
      background: var(--surface); border-radius: 12px;
      padding: 1.5rem; margin-bottom: 1.5rem;
      border-left: 4px solid var(--lapis);
    }
    .case-study h2 { color: var(--gold); font-size: 1.2rem; margin-bottom: 0.5rem; }
    .case-study time { color: #888; font-size: 0.85rem; }
    .case-study .body {
      margin-top: 0.75rem; line-height: 1.6;
      white-space: pre-wrap; font-size: 0.9rem;
    }
    footer { text-align: center; color: #666; margin-top: 3rem; font-size: 0.8rem; }
  </style>
</head>
<body>
  <header>
    <h1>𓂀 Pantheon Case Studies</h1>
    <p>Real-World Stories from the Pantheon Ecosystem</p>
  </header>
  <main>
`
}

func caseStudyHTMLFooter() string {
	return fmt.Sprintf(`  </main>
  <footer>
    <p>Auto-generated by 𓂀 Horus at %s</p>
  </footer>
</body>
</html>
`, time.Now().Format("2006-01-02 15:04"))
}

// ── Helpers ──────────────────────────────────────────────────────────────

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
