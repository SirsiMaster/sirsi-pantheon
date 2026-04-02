package seshat

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ThothAdapter exports Knowledge Items into a Thoth project memory system.
// This is the primary integration point between Seshat (cross-platform knowledge)
// and Thoth (project-level memory).
type ThothAdapter struct {
	// ProjectDir is the project directory containing .thoth/.
	// If empty, uses the current working directory.
	ProjectDir string
}

func (a *ThothAdapter) Name() string { return "thoth" }
func (a *ThothAdapter) Description() string {
	return "Thoth project memory (.thoth/ knowledge injection)"
}

func (a *ThothAdapter) thothDir() string {
	dir := a.ProjectDir
	if dir == "" {
		dir, _ = os.Getwd()
	}
	return filepath.Join(dir, ".thoth")
}

// Export writes Knowledge Items as Thoth artifacts in the project's .thoth/ directory.
// Items are written to .thoth/seshat/ as Markdown files with metadata headers.
func (a *ThothAdapter) Export(items []KnowledgeItem) error {
	seshatDir := filepath.Join(a.thothDir(), "seshat")
	if err := os.MkdirAll(seshatDir, 0755); err != nil {
		return fmt.Errorf("create .thoth/seshat/: %w", err)
	}

	for _, ki := range items {
		filename := sanitizeFilename(ki.Title)
		if filename == "" {
			continue
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("# %s\n\n", ki.Title))
		sb.WriteString(fmt.Sprintf("**Grafted by Seshat**: %s\n", time.Now().Format("2006-01-02 15:04")))

		// Source provenance
		for _, ref := range ki.References {
			if ref.Type == "source" {
				sb.WriteString(fmt.Sprintf("**Origin**: %s\n", ref.Value))
			}
		}
		sb.WriteString("\n---\n\n")
		sb.WriteString(ki.Summary)
		sb.WriteString("\n")

		outPath := filepath.Join(seshatDir, filename+".md")
		if err := os.WriteFile(outPath, []byte(sb.String()), 0644); err != nil {
			fmt.Printf("  ⚠️  Failed to write %s: %v\n", outPath, err)
			continue
		}
	}

	fmt.Printf("  𓁟 Grafted %d items into Thoth (.thoth/seshat/)\n", len(items))
	return nil
}
