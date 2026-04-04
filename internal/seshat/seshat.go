// Package seshat implements the Gemini Bridge — bidirectional knowledge sync
// between Gemini AI Mode, NotebookLM, and Antigravity IDE.
//
// 𓁆 Seshat — Goddess of writing, wisdom, and measurement.
// She is the keeper of records and the inventor of writing itself.
//
// Seshat bridges three knowledge systems:
//   - Gemini AI Mode (Google's conversational AI)
//   - NotebookLM (source-grounded research notebooks)
//   - Antigravity IDE (persistent Knowledge Items)
//
// The bridge operates in six directions:
//  1. Gemini → NotebookLM   (extract + upload as sources)
//  2. NotebookLM → Gemini   (query + inject into GEMINI.md)
//  3. NotebookLM → Antigravity (distill + inject as KI)
//  4. Antigravity → NotebookLM (export KIs as sources)
//  5. Gemini → Antigravity   (extract + inject as KI)
//  6. Antigravity → Gemini   (export KIs → GEMINI.md context)
package seshat

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/stele"
)

// SchemaVersion is the normalized conversation format version.
const SchemaVersion = "1.0.0"

// MaxSourcesPerNotebook is the NotebookLM limit.
const MaxSourcesPerNotebook = 50

// Conversation represents a normalized conversation extracted from any source.
type Conversation struct {
	ID           string           `json:"id"`
	Title        string           `json:"title"`
	StartedAt    string           `json:"startedAt"`
	MessageCount int              `json:"messageCount"`
	Messages     []Message        `json:"messages"`
	Metadata     ConversationMeta `json:"metadata"`
}

// Message represents a single turn in a conversation.
type Message struct {
	Role      string `json:"role"` // "user", "assistant", "system"
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
}

// ConversationMeta holds extracted metadata about a conversation.
type ConversationMeta struct {
	SourceType     string   `json:"sourceType"`
	SourceFile     string   `json:"sourceFile"`
	Topics         []string `json:"topics"`
	FilesModified  []string `json:"filesModified"`
	CodeBlockCount int      `json:"codeBlockCount"`
	ExtractedAt    string   `json:"extractedAt"`
	SchemaVersion  string   `json:"schemaVersion"`
}

// ExtractionResult holds the output of an extraction run.
type ExtractionResult struct {
	Source            string         `json:"source"`
	ExtractedAt       string         `json:"extractedAt"`
	SchemaVersion     string         `json:"schemaVersion"`
	ConversationCount int            `json:"conversationCount"`
	Conversations     []Conversation `json:"conversations"`
}

// KnowledgeItem represents an Antigravity IDE Knowledge Item.
type KnowledgeItem struct {
	Title      string        `json:"title"`
	Summary    string        `json:"summary"`
	References []KIReference `json:"references"`
}

// KIReference is a reference inside a Knowledge Item's metadata.
type KIReference struct {
	Type  string `json:"type"` // "file", "conversation_id", "url"
	Value string `json:"value"`
}

// KITimestamps tracks the lifecycle of a Knowledge Item.
type KITimestamps struct {
	Created  string `json:"created"`
	Modified string `json:"modified"`
	Accessed string `json:"accessed"`
}

// Paths returns the standard filesystem paths for the bridge.
type Paths struct {
	AntigravityDir   string
	KnowledgeDir     string
	BrainDir         string
	ConversationsDir string
}

// DefaultPaths returns the standard Antigravity paths based on $HOME.
func DefaultPaths() Paths {
	home, _ := os.UserHomeDir()
	agDir := filepath.Join(home, ".gemini", "antigravity")
	return Paths{
		AntigravityDir:   agDir,
		KnowledgeDir:     filepath.Join(agDir, "knowledge"),
		BrainDir:         filepath.Join(agDir, "brain"),
		ConversationsDir: filepath.Join(agDir, "conversations"),
	}
}

// WriteKnowledgeItem writes a Knowledge Item to the Antigravity knowledge directory.
func WriteKnowledgeItem(paths Paths, name string, ki KnowledgeItem, artifacts map[string]string) error {
	// Create safe directory name
	dirName := strings.ToLower(strings.TrimSpace(name))
	dirName = strings.ReplaceAll(dirName, " ", "_")
	dirName = strings.ReplaceAll(dirName, "-", "_")
	if len(dirName) > 60 {
		dirName = dirName[:60]
	}

	kiPath := filepath.Join(paths.KnowledgeDir, dirName)
	artifactsPath := filepath.Join(kiPath, "artifacts")

	// Create directories
	if err := os.MkdirAll(artifactsPath, 0755); err != nil {
		return fmt.Errorf("create KI directory: %w", err)
	}

	// Write metadata.json
	metaBytes, err := json.MarshalIndent(ki, "", "    ")
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	if err = os.WriteFile(filepath.Join(kiPath, "metadata.json"), metaBytes, 0644); err != nil {
		return fmt.Errorf("write metadata: %w", err)
	}

	// Write timestamps.json
	now := time.Now().Format(time.RFC3339)
	ts := KITimestamps{Created: now, Modified: now, Accessed: now}
	tsBytes, err := json.MarshalIndent(ts, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal timestamps: %w", err)
	}
	if err := os.WriteFile(filepath.Join(kiPath, "timestamps.json"), tsBytes, 0644); err != nil {
		return fmt.Errorf("write timestamps: %w", err)
	}

	// Write artifact files
	for filename, content := range artifacts {
		artPath := filepath.Join(artifactsPath, filename)
		artDir := filepath.Dir(artPath)
		if err := os.MkdirAll(artDir, 0755); err != nil {
			return fmt.Errorf("create artifact dir: %w", err)
		}
		if err := os.WriteFile(artPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("write artifact %s: %w", filename, err)
		}
	}

	stele.Inscribe("seshat", stele.TypeSeshatIngest, "", map[string]string{
		"name":      name,
		"artifacts": fmt.Sprintf("%d", len(artifacts)),
	})
	return nil
}

// ListKnowledgeItems returns all Knowledge Items in the Antigravity knowledge directory.
func ListKnowledgeItems(paths Paths) ([]string, error) {
	entries, err := os.ReadDir(paths.KnowledgeDir)
	if err != nil {
		return nil, fmt.Errorf("read knowledge dir: %w", err)
	}

	var items []string
	for _, entry := range entries {
		if entry.IsDir() {
			items = append(items, entry.Name())
		}
	}
	return items, nil
}

// ReadKnowledgeItem reads a Knowledge Item's metadata from disk.
func ReadKnowledgeItem(paths Paths, name string) (*KnowledgeItem, error) {
	metaPath := filepath.Join(paths.KnowledgeDir, name, "metadata.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, fmt.Errorf("read metadata: %w", err)
	}

	var ki KnowledgeItem
	if err := json.Unmarshal(data, &ki); err != nil {
		return nil, fmt.Errorf("parse metadata: %w", err)
	}
	return &ki, nil
}
