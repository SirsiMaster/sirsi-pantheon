package seshat

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// GeminiAdapter ingests Gemini AI conversations from Google Takeout exports
// and from the local Gemini data directory.
type GeminiAdapter struct {
	// TakeoutDir is the path to an extracted Google Takeout containing Gemini data.
	// Typically: ~/Downloads/Takeout/Gemini Apps/
	TakeoutDir string

	// LocalDir overrides the default Gemini local data directory.
	// If empty, uses ~/.gemini/
	LocalDir string
}

func (a *GeminiAdapter) Name() string { return "gemini" }
func (a *GeminiAdapter) Description() string {
	return "Gemini AI conversations (Takeout export + local)"
}

func (a *GeminiAdapter) localDir() string {
	if a.LocalDir != "" {
		return a.LocalDir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".gemini")
}

func (a *GeminiAdapter) takeoutDir() string {
	if a.TakeoutDir != "" {
		return a.TakeoutDir
	}
	home, _ := os.UserHomeDir()
	// Check common Takeout download locations
	candidates := []string{
		filepath.Join(home, "Downloads", "Takeout", "Gemini Apps"),
		filepath.Join(home, "Downloads", "Takeout", "Google Gemini"),
		filepath.Join(home, "Desktop", "Takeout", "Gemini Apps"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return candidates[0] // default even if not found
}

// geminiTakeoutConversation represents a conversation in Google Takeout format.
type geminiTakeoutConversation struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Messages []struct {
		Role      string `json:"role"`
		Content   string `json:"content"`
		Timestamp string `json:"timestamp"`
	} `json:"messages"`
	CreateTime string `json:"create_time"`
	UpdateTime string `json:"update_time"`
}

// Ingest extracts Gemini conversations from Takeout exports and local data.
func (a *GeminiAdapter) Ingest(since time.Time) ([]KnowledgeItem, error) {
	var items []KnowledgeItem

	// Source 1: Google Takeout export (JSON files)
	takeoutItems, err := a.ingestTakeout(since)
	if err != nil {
		fmt.Printf("  ⚠️  Gemini Takeout: %v\n", err)
	} else {
		items = append(items, takeoutItems...)
	}

	// Source 2: Local .gemini/ conversations
	localItems, err := a.ingestLocal(since)
	if err != nil {
		fmt.Printf("  ⚠️  Gemini local: %v\n", err)
	} else {
		items = append(items, localItems...)
	}

	return items, nil
}

func (a *GeminiAdapter) ingestTakeout(since time.Time) ([]KnowledgeItem, error) {
	dir := a.takeoutDir()
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, fmt.Errorf("Takeout directory not found: %s (export from takeout.google.com)", dir)
	}

	var items []KnowledgeItem

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		// Try parsing as a single conversation
		var conv geminiTakeoutConversation
		if err := json.Unmarshal(data, &conv); err == nil && conv.Title != "" {
			ki := a.conversationToKI(conv, path)
			if ki != nil {
				items = append(items, *ki)
			}
			return nil
		}

		// Try parsing as an array of conversations
		var convs []geminiTakeoutConversation
		if err := json.Unmarshal(data, &convs); err == nil {
			for _, c := range convs {
				ki := a.conversationToKI(c, path)
				if ki != nil {
					items = append(items, *ki)
				}
			}
		}

		return nil
	})

	return items, err
}

func (a *GeminiAdapter) conversationToKI(conv geminiTakeoutConversation, sourcePath string) *KnowledgeItem {
	if conv.Title == "" && len(conv.Messages) == 0 {
		return nil
	}

	// Build summary from first user message
	summary := conv.Title
	for _, msg := range conv.Messages {
		if msg.Role == "user" {
			summary = truncate(msg.Content, 200)
			break
		}
	}

	// Build content from full conversation
	var content strings.Builder
	for _, msg := range conv.Messages {
		content.WriteString(fmt.Sprintf("**%s**: %s\n\n", msg.Role, msg.Content))
	}

	title := conv.Title
	if title == "" {
		title = fmt.Sprintf("Gemini conversation %s", conv.ID)
	}

	return &KnowledgeItem{
		Title:   title,
		Summary: summary,
		References: []KIReference{
			{Type: "source", Value: "gemini"},
			{Type: "conversation_id", Value: conv.ID},
			{Type: "file", Value: sourcePath},
		},
	}
}

func (a *GeminiAdapter) ingestLocal(since time.Time) ([]KnowledgeItem, error) {
	conversationsDir := filepath.Join(a.localDir(), "conversations")
	if _, err := os.Stat(conversationsDir); os.IsNotExist(err) {
		return nil, nil // no local conversations, not an error
	}

	entries, err := os.ReadDir(conversationsDir)
	if err != nil {
		return nil, fmt.Errorf("read Gemini conversations: %w", err)
	}

	var items []KnowledgeItem
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil || info.ModTime().Before(since) {
			continue
		}

		// Try to read metadata
		metaPath := filepath.Join(conversationsDir, entry.Name(), "metadata.json")
		data, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}

		var conv geminiTakeoutConversation
		if err := json.Unmarshal(data, &conv); err != nil {
			continue
		}

		ki := a.conversationToKI(conv, metaPath)
		if ki != nil {
			items = append(items, *ki)
		}
	}

	return items, nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
