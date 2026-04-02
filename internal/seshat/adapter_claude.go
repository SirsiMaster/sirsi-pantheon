package seshat

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ClaudeAdapter ingests Claude AI conversations from local .jsonl transcripts.
type ClaudeAdapter struct {
	// ClaudeDir overrides the default Claude data directory.
	// If empty, uses ~/.claude/
	ClaudeDir string
}

func (a *ClaudeAdapter) Name() string        { return "claude" }
func (a *ClaudeAdapter) Description() string { return "Claude AI conversations from local transcripts" }

func (a *ClaudeAdapter) claudeDir() string {
	if a.ClaudeDir != "" {
		return a.ClaudeDir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

// claudeMessage represents a single message in a Claude .jsonl transcript.
type claudeMessage struct {
	Type      string `json:"type"`
	Role      string `json:"role"`
	SessionID string `json:"sessionId"`
	Timestamp string `json:"timestamp"`
	Message   struct {
		Role    string `json:"role"`
		Content any    `json:"content"`
	} `json:"message"`
}

// Ingest reads Claude .jsonl transcript files and extracts conversations.
func (a *ClaudeAdapter) Ingest(since time.Time) ([]KnowledgeItem, error) {
	projectsDir := filepath.Join(a.claudeDir(), "projects")
	if _, err := os.Stat(projectsDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("Claude projects directory not found: %s", projectsDir)
	}

	var items []KnowledgeItem

	err := filepath.Walk(projectsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}

		if info.ModTime().Before(since) {
			return nil
		}

		ki, err := a.parseTranscript(path)
		if err != nil {
			return nil // skip unreadable transcripts
		}
		if ki != nil {
			items = append(items, *ki)
		}
		return nil
	})

	return items, err
}

func (a *ClaudeAdapter) parseTranscript(path string) (*KnowledgeItem, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var userMessages []string
	var assistantMessages []string
	var sessionID string
	var firstTimestamp, lastTimestamp string
	messageCount := 0

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB line buffer

	for scanner.Scan() {
		var msg claudeMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}

		if msg.SessionID != "" && sessionID == "" {
			sessionID = msg.SessionID
		}
		if msg.Timestamp != "" {
			if firstTimestamp == "" {
				firstTimestamp = msg.Timestamp
			}
			lastTimestamp = msg.Timestamp
		}

		role := msg.Role
		if role == "" {
			role = msg.Message.Role
		}

		// Extract text content
		var text string
		switch content := msg.Message.Content.(type) {
		case string:
			text = content
		case []any:
			for _, block := range content {
				if m, ok := block.(map[string]any); ok {
					if t, ok := m["text"].(string); ok {
						text += t + " "
					}
				}
			}
		}

		if text == "" {
			continue
		}

		messageCount++
		if role == "user" && len(userMessages) < 5 {
			userMessages = append(userMessages, truncate(strings.TrimSpace(text), 150))
		}
		if role == "assistant" && len(assistantMessages) < 3 {
			assistantMessages = append(assistantMessages, truncate(strings.TrimSpace(text), 150))
		}
	}

	if messageCount < 2 {
		return nil, nil // too small to be useful
	}

	// Build title from first user message
	title := "Claude conversation"
	if len(userMessages) > 0 {
		title = truncate(userMessages[0], 80)
	}

	// Build summary from user messages
	summary := strings.Join(userMessages, " | ")
	if len(summary) > 300 {
		summary = summary[:300] + "..."
	}

	return &KnowledgeItem{
		Title:   title,
		Summary: fmt.Sprintf("%d messages (%s to %s): %s", messageCount, firstTimestamp, lastTimestamp, summary),
		References: []KIReference{
			{Type: "source", Value: "claude"},
			{Type: "file", Value: path},
			{Type: "conversation_id", Value: sessionID},
		},
	}, nil
}
