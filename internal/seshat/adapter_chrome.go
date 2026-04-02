package seshat

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	// Pure-Go SQLite driver — no CGO required (Rule A3 compliance).
	_ "modernc.org/sqlite"
)

// ChromeHistoryAdapter ingests Google Search history from Chrome's local SQLite database.
type ChromeHistoryAdapter struct {
	// ProfileDir overrides the default Chrome profile directory.
	// If empty, uses ~/Library/Application Support/Google/Chrome/Default.
	ProfileDir string
}

func (a *ChromeHistoryAdapter) Name() string { return "chrome-history" }
func (a *ChromeHistoryAdapter) Description() string {
	return "Google Search history from Chrome browser"
}

func (a *ChromeHistoryAdapter) profileDir() string {
	if a.ProfileDir != "" {
		return a.ProfileDir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "Default")
}

// Ingest reads Chrome's History SQLite database and extracts Google searches.
func (a *ChromeHistoryAdapter) Ingest(since time.Time) ([]KnowledgeItem, error) {
	historyDB := filepath.Join(a.profileDir(), "History")

	if _, err := os.Stat(historyDB); os.IsNotExist(err) {
		return nil, fmt.Errorf("Chrome history not found at %s", historyDB)
	}

	// Chrome locks the DB while running — copy to temp file
	tmpDB := filepath.Join(os.TempDir(), "seshat_chrome_history.db")
	src, err := os.ReadFile(historyDB)
	if err != nil {
		return nil, fmt.Errorf("read Chrome history: %w", err)
	}
	if err = os.WriteFile(tmpDB, src, 0600); err != nil {
		return nil, fmt.Errorf("copy history to temp: %w", err)
	}
	defer os.Remove(tmpDB)

	db, err := sql.Open("sqlite", tmpDB+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open Chrome history DB: %w", err)
	}
	defer db.Close()

	// Chrome stores timestamps as microseconds since Jan 1, 1601
	// Convert Go time to Chrome time
	chromeEpoch := time.Date(1601, 1, 1, 0, 0, 0, 0, time.UTC)
	sinceChrome := since.Sub(chromeEpoch).Microseconds()

	query := `
		SELECT url, title, visit_count, last_visit_time
		FROM urls
		WHERE url LIKE '%google.com/search%'
		  AND last_visit_time > ?
		ORDER BY last_visit_time DESC
		LIMIT 500
	`

	rows, err := db.Query(query, sinceChrome)
	if err != nil {
		return nil, fmt.Errorf("query Chrome history: %w", err)
	}
	defer rows.Close()

	var items []KnowledgeItem
	for rows.Next() {
		var url, title string
		var visitCount int
		var lastVisit int64

		if err := rows.Scan(&url, &title, &visitCount, &lastVisit); err != nil {
			continue
		}

		visitTime := chromeEpoch.Add(time.Duration(lastVisit) * time.Microsecond)

		items = append(items, KnowledgeItem{
			Title:   title,
			Summary: fmt.Sprintf("Google search (%d visits, last: %s)", visitCount, visitTime.Format("2006-01-02 15:04")),
			References: []KIReference{
				{Type: "url", Value: url},
				{Type: "source", Value: "chrome-history"},
			},
		})
	}

	return items, nil
}
