package seshat

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ExportKIToMarkdown exports a Knowledge Item as a NotebookLM-ready Markdown source.
func ExportKIToMarkdown(paths Paths, kiName string) (string, error) {
	ki, err := ReadKnowledgeItem(paths, kiName)
	if err != nil {
		return "", fmt.Errorf("read KI %s: %w", kiName, err)
	}

	artifactsDir := filepath.Join(paths.KnowledgeDir, kiName, "artifacts")
	entries, err := os.ReadDir(artifactsDir)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("read artifacts: %w", err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s\n\n", ki.Title))
	sb.WriteString(fmt.Sprintf("**Knowledge Item**: `%s`\n", kiName))
	sb.WriteString(fmt.Sprintf("**Summary**: %s\n", ki.Summary))
	sb.WriteString(fmt.Sprintf("**Exported**: %s\n", time.Now().Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("**Artifacts**: %d\n\n---\n\n", len(entries)))

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(artifactsDir, entry.Name()))
		if err != nil {
			continue
		}
		sb.WriteString(fmt.Sprintf("## Artifact: %s\n\n", entry.Name()))
		sb.Write(content)
		sb.WriteString("\n\n---\n\n")
	}

	return sb.String(), nil
}

// ExportAllKIsToMarkdown exports all Knowledge Items as Markdown files.
func ExportAllKIsToMarkdown(paths Paths, outputDir string) ([]string, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	items, err := ListKnowledgeItems(paths)
	if err != nil {
		return nil, err
	}

	var exported []string
	for _, name := range items {
		md, err := ExportKIToMarkdown(paths, name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "⚠️ Skipping %s: %v\n", name, err)
			continue
		}

		outFile := filepath.Join(outputDir, fmt.Sprintf("ki_%s.md", name))
		if err := os.WriteFile(outFile, []byte(md), 0644); err != nil {
			return nil, fmt.Errorf("write %s: %w", outFile, err)
		}
		exported = append(exported, outFile)
	}
	return exported, nil
}

// SyncKIToGeminiMD injects a Knowledge Item context section into a GEMINI.md file.
func SyncKIToGeminiMD(paths Paths, kiName, targetFile string) error {
	ki, err := ReadKnowledgeItem(paths, kiName)
	if err != nil {
		return fmt.Errorf("read KI: %w", err)
	}

	// Build context section
	markerStart := fmt.Sprintf("<!-- KI:%s:START -->", kiName)
	markerEnd := fmt.Sprintf("<!-- KI:%s:END -->", kiName)

	var sb strings.Builder
	sb.WriteString(markerStart + "\n")
	sb.WriteString(fmt.Sprintf("### 🧠 Knowledge Context: %s\n\n", ki.Title))
	sb.WriteString(fmt.Sprintf("> **Source**: Antigravity Knowledge Item `%s`\n", kiName))
	sb.WriteString(fmt.Sprintf("> **Synced**: %s\n\n", time.Now().Format("2006-01-02 15:04")))
	sb.WriteString(ki.Summary + "\n\n")
	sb.WriteString(markerEnd)

	section := sb.String()

	// Read or create target
	existing, err := os.ReadFile(targetFile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read target: %w", err)
	}

	content := string(existing)
	if strings.Contains(content, markerStart) {
		// Replace existing section
		startIdx := strings.Index(content, markerStart)
		endIdx := strings.Index(content, markerEnd) + len(markerEnd)
		content = content[:startIdx] + section + content[endIdx:]
	} else {
		content += "\n\n" + section
	}

	if err := os.WriteFile(targetFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("write target: %w", err)
	}
	return nil
}

// ListBrainConversations lists Antigravity brain conversation IDs sorted by recency.
func ListBrainConversations(paths Paths, lastN int) ([]string, error) {
	entries, err := os.ReadDir(paths.BrainDir)
	if err != nil {
		return nil, fmt.Errorf("read brain dir: %w", err)
	}

	type convEntry struct {
		name    string
		modTime time.Time
	}
	var convs []convEntry

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "tempmediaStorage" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		convs = append(convs, convEntry{name: entry.Name(), modTime: info.ModTime()})
	}

	sort.Slice(convs, func(i, j int) bool {
		return convs[i].modTime.After(convs[j].modTime)
	})

	if lastN > 0 && lastN < len(convs) {
		convs = convs[:lastN]
	}

	var ids []string
	for _, c := range convs {
		ids = append(ids, c.name)
	}
	return ids, nil
}

// SaveExtractionResult writes an ExtractionResult to a JSON file.
func SaveExtractionResult(result *ExtractionResult, outputDir string) (string, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}

	result.Source = "gemini-bridge"
	result.ExtractedAt = time.Now().Format(time.RFC3339)
	result.SchemaVersion = SchemaVersion
	result.ConversationCount = len(result.Conversations)

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal result: %w", err)
	}

	ts := time.Now().Format("20060102_150405")
	outFile := filepath.Join(outputDir, fmt.Sprintf("extracted_%s.json", ts))
	if err := os.WriteFile(outFile, data, 0644); err != nil {
		return "", fmt.Errorf("write result: %w", err)
	}
	return outFile, nil
}
