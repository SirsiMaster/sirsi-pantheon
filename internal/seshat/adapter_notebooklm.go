package seshat

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// NotebookLMAdapter exports Knowledge Items as Markdown source documents
// ready for upload to Google NotebookLM.
//
// NotebookLM has no public API — this adapter prepares files for manual
// or automated (Puppeteer/Playwright) upload. Each KI becomes a Markdown
// file in the output directory, formatted for NotebookLM's source ingestion.
type NotebookLMAdapter struct {
	// OutputDir is the directory where exported Markdown files are written.
	// If empty, uses ~/.config/seshat/notebooklm_export/
	OutputDir string
}

func (a *NotebookLMAdapter) Name() string { return "notebooklm" }
func (a *NotebookLMAdapter) Description() string {
	return "Google NotebookLM (export as source documents)"
}

func (a *NotebookLMAdapter) outputDir() string {
	if a.OutputDir != "" {
		return a.OutputDir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "seshat", "notebooklm_export")
}

// Export writes Knowledge Items as Markdown files for NotebookLM upload.
// Each file is formatted to maximize NotebookLM's source grounding:
// - Clear title and metadata header
// - Structured content with section headers
// - References as footnotes
//
// NotebookLM supports max 50 sources per notebook.
func (a *NotebookLMAdapter) Export(items []KnowledgeItem) error {
	dir := a.outputDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create NotebookLM export dir: %w", err)
	}

	if len(items) > MaxSourcesPerNotebook {
		fmt.Printf("  ⚠️  NotebookLM limit: %d sources max per notebook, exporting first %d of %d\n",
			MaxSourcesPerNotebook, MaxSourcesPerNotebook, len(items))
		items = items[:MaxSourcesPerNotebook]
	}

	for i, ki := range items {
		filename := sanitizeFilename(ki.Title)
		if filename == "" {
			filename = fmt.Sprintf("ki_%03d", i+1)
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("# %s\n\n", ki.Title))
		sb.WriteString("**Source**: Seshat Knowledge Grafting Engine\n")
		sb.WriteString(fmt.Sprintf("**Exported**: %s\n\n", time.Now().Format("2006-01-02 15:04")))
		sb.WriteString("---\n\n")

		// Content section
		if ki.Summary != "" {
			sb.WriteString("## Summary\n\n")
			sb.WriteString(ki.Summary)
			sb.WriteString("\n\n")
		}

		// References section
		if len(ki.References) > 0 {
			sb.WriteString("## References\n\n")
			for _, ref := range ki.References {
				sb.WriteString(fmt.Sprintf("- **%s**: %s\n", ref.Type, ref.Value))
			}
			sb.WriteString("\n")
		}

		outPath := filepath.Join(dir, filename+".md")
		if err := os.WriteFile(outPath, []byte(sb.String()), 0644); err != nil {
			return fmt.Errorf("write %s: %w", outPath, err)
		}
	}

	fmt.Printf("  📝 Exported %d sources to %s\n", len(items), dir)
	fmt.Printf("  → Upload to NotebookLM: https://notebooklm.google.com\n")
	return nil
}

// sanitizeFilename creates a safe filename from a title.
func sanitizeFilename(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		if r == ' ' {
			return '_'
		}
		return -1
	}, s)
	if len(s) > 60 {
		s = s[:60]
	}
	return s
}
