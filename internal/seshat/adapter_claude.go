package seshat

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxArchiveTranscriptBytes = 16 << 20
	maxArchiveSourceBytes     = 256 << 20
	maxArchiveSourceItems     = 10_000
	maxArchiveWalkEntries     = 100_000
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
	records, err := a.IngestArchive(since)
	items := make([]KnowledgeItem, 0, len(records))
	for _, record := range records {
		items = append(items, KnowledgeItem{
			Title: record.Title, Summary: record.Summary, References: record.References,
		})
	}
	return items, err
}

// IngestArchive retains complete message text and stable source identity for
// Ma'at. Errors are aggregate and content-free; successfully parsed siblings
// remain available to the caller for a partial import.
func (a *ClaudeAdapter) IngestArchive(since time.Time) ([]SourceRecord, error) {
	return a.IngestArchiveContext(context.Background(), since)
}

func (a *ClaudeAdapter) IngestArchiveContext(ctx context.Context, since time.Time) ([]SourceRecord, error) {
	return a.ingestArchiveContext(ctx, since, maxArchiveTranscriptBytes, maxArchiveSourceBytes, maxArchiveSourceItems)
}

func (a *ClaudeAdapter) ingestArchiveContext(ctx context.Context, since time.Time, transcriptLimit, sourceLimit int64, itemLimit int) ([]SourceRecord, error) {
	projectsDir := filepath.Join(a.claudeDir(), "projects")
	if _, err := os.Stat(projectsDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("Claude projects directory not found: %s", projectsDir)
	}

	root, err := filepath.Abs(a.claudeDir())
	if err != nil {
		return nil, errors.New("Claude source root is unavailable")
	}
	instanceDigest := sha256.Sum256([]byte(filepath.Clean(root)))
	instanceID := fmt.Sprintf("claude:%x", instanceDigest[:16])
	var records []SourceRecord
	var partial bool
	var scannedBytes int64
	var walkedEntries int

	err = filepath.Walk(projectsDir, func(path string, info os.FileInfo, walkErr error) error {
		if ctx.Err() != nil {
			partial = true
			return ctx.Err()
		}
		walkedEntries++
		if walkedEntries > maxArchiveWalkEntries {
			partial = true
			return filepath.SkipAll
		}
		if walkErr != nil {
			partial = true
			return nil
		}
		if info.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > transcriptLimit || len(records) >= itemLimit {
			partial = true
			return nil
		}

		if info.ModTime().Before(since) {
			return nil
		}
		remaining := sourceLimit - scannedBytes
		if remaining <= 0 {
			partial = true
			return filepath.SkipAll
		}
		fileLimit := transcriptLimit
		if remaining < fileLimit {
			fileLimit = remaining
		}
		record, readBytes, parseErr := a.parseArchiveTranscriptContext(ctx, path, instanceID, fileLimit)
		scannedBytes += readBytes
		if parseErr != nil {
			partial = true
		}
		if record != nil {
			records = append(records, *record)
		}
		return nil
	})
	if err != nil {
		partial = true
	}
	if partial {
		return records, errors.New("Claude source was only partially readable")
	}
	return records, nil
}

// parseTranscript preserves the compatibility parser used by Seshat tests and
// legacy callers while the canonical Ma'at path uses the richer record.
func (a *ClaudeAdapter) parseTranscript(path string) (*KnowledgeItem, error) {
	record, err := a.parseArchiveTranscript(path, "legacy-claude")
	if record == nil {
		return nil, err
	}
	return &KnowledgeItem{Title: record.Title, Summary: record.Summary, References: record.References}, err
}

func (a *ClaudeAdapter) parseArchiveTranscript(path, instanceID string) (*SourceRecord, error) {
	record, _, err := a.parseArchiveTranscriptContext(context.Background(), path, instanceID, maxArchiveTranscriptBytes)
	return record, err
}

func (a *ClaudeAdapter) parseArchiveTranscriptContext(ctx context.Context, path, instanceID string, limit int64) (*SourceRecord, int64, error) {
	return a.parseArchiveTranscriptWithHook(ctx, path, instanceID, limit, nil)
}

func (a *ClaudeAdapter) parseArchiveTranscriptWithHook(ctx context.Context, path, instanceID string, limit int64, afterIdentityCheck func() error) (*SourceRecord, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil || !before.Mode().IsRegular() || before.Size() < 0 {
		return nil, 0, errors.New("transcript file identity is unavailable")
	}
	pathInfo, pathErr := os.Lstat(path)
	if pathErr != nil || pathInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(before, pathInfo) {
		return nil, 0, errors.New("transcript path changed before bounded read")
	}
	if afterIdentityCheck != nil {
		if err := afterIdentityCheck(); err != nil {
			return nil, 0, errors.New("transcript changed before bounded read")
		}
	}
	data, readErr := io.ReadAll(io.LimitReader(contextReader{ctx: ctx, reader: f}, limit+1))
	if readErr != nil || ctx.Err() != nil || int64(len(data)) > limit {
		return nil, int64(len(data)), errors.New("transcript exceeded its bounded read or was canceled")
	}
	after, statErr := f.Stat()
	if statErr != nil || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) || int64(len(data)) != before.Size() {
		return nil, int64(len(data)), errors.New("transcript changed during bounded read")
	}

	var userMessages []string
	var assistantMessages []string
	var sessionID string
	var firstTimestamp, lastTimestamp string
	messageCount := 0
	var content strings.Builder
	var malformed bool

	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB line buffer

	for scanner.Scan() {
		if ctx.Err() != nil {
			return nil, int64(len(data)), ctx.Err()
		}
		var msg claudeMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			malformed = true
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
		content.WriteString(role)
		content.WriteString(": ")
		content.WriteString(strings.TrimSpace(text))
		content.WriteString("\n\n")
		if role == "user" && len(userMessages) < 5 {
			userMessages = append(userMessages, truncate(strings.TrimSpace(text), 150))
		}
		if role == "assistant" && len(assistantMessages) < 3 {
			assistantMessages = append(assistantMessages, truncate(strings.TrimSpace(text), 150))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, int64(len(data)), err
	}

	if messageCount < 2 {
		if malformed {
			return nil, int64(len(data)), errors.New("transcript contains no complete conversation")
		}
		return nil, int64(len(data)), nil // too small to be useful
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

	if sessionID == "" {
		relative, relErr := filepath.Rel(filepath.Join(a.claudeDir(), "projects"), path)
		if relErr != nil {
			return nil, int64(len(data)), errors.New("transcript identity is unavailable")
		}
		fallback := sha256.Sum256([]byte(filepath.Clean(relative)))
		sessionID = fmt.Sprintf("path:%x", fallback[:])
	}
	refs := []KIReference{
		{Type: "source", Value: "claude"},
		{Type: "conversation_id", Value: sessionID},
	}
	if relative, relErr := filepath.Rel(filepath.Join(a.claudeDir(), "projects"), path); relErr == nil {
		refs = append(refs, KIReference{Type: "file", Value: filepath.ToSlash(relative)})
	}
	var recordErr error
	if malformed {
		recordErr = errors.New("transcript contains unreadable records")
	}
	record := &SourceRecord{
		Source: "claude", InstanceID: instanceID, NativeItemID: sessionID,
		Schema: SchemaVersion, ExtractedAt: time.Now().UTC(), Title: title,
		Summary: fmt.Sprintf("%d messages (%s to %s): %s", messageCount, firstTimestamp, lastTimestamp, summary),
		Content: content.String(), References: refs,
	}
	if recordErr != nil {
		return record, int64(len(data)), recordErr
	}
	return record, int64(len(data)), nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}
