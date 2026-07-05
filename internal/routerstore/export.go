package routerstore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// quoteYAML wraps a value in double quotes and escapes embedded quotes,
// backslashes, and newlines. It mirrors internal/work.quoteYAML byte-for-byte
// so exported frontmatter is identical to what the file router writes.
func quoteYAML(v string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`)
	return `"` + r.Replace(v) + `"`
}

// ExportMarkdown writes every item in the store to dir, one <id>.md per item,
// in the file router's exact frontmatter+body format — the same bytes
// internal/work.SendTyped, Close, and SetWake would have produced for the same
// item, so internal/work.Get parses an exported file back to an equal Item
// (round-trip proven by TestExportMarkdownRoundTrip).
//
// This is the PRD Phase-1 audit/recovery path (docs/prd/ROUTER_V2_DURABLE_DISPATCH.md
// §4: "ExportMarkdown gives a human-readable audit trail and recovery path"):
// if the SQLite index is ever corrupted or distrusted, the queue can be dumped
// back to plain files the existing file router reads natively. It returns the
// number of items written; on error, files already written are left in place
// (a partial audit dump is still an audit dump).
func (s *Store) ExportMarkdown(dir string) (int, error) {
	items, err := s.ListAll()
	if err != nil {
		return 0, fmt.Errorf("routerstore: ExportMarkdown: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, fmt.Errorf("routerstore: ExportMarkdown: mkdir %q: %w", dir, err)
	}
	n := 0
	for _, it := range items {
		path := filepath.Join(dir, it.ID+".md")
		if err := os.WriteFile(path, []byte(renderMarkdown(it)), 0o644); err != nil {
			return n, fmt.Errorf("routerstore: ExportMarkdown %q: %w", it.ID, err)
		}
		n++
	}
	return n, nil
}

// renderMarkdown serializes one item in the canonical file-router layout.
// Field order and quoting mirror the file router's writers exactly:
//   - internal/work.SendTyped: from/to/title/[type] quoted, status/opened bare;
//   - internal/work.Close: `closed:` inserted directly after `opened:`, and a
//     `## Result` section appended (placeholder text when the result is empty);
//   - internal/work.SetWake: wake_* lines appended at the end of frontmatter,
//     in status/attempted_at/adapter/error order, empty fields omitted.
func renderMarkdown(it Item) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("from: " + quoteYAML(it.From) + "\n")
	b.WriteString("to: " + quoteYAML(it.To) + "\n")
	b.WriteString("title: " + quoteYAML(it.Title) + "\n")
	if it.Type != "" {
		b.WriteString("type: " + quoteYAML(it.Type) + "\n")
	}
	b.WriteString("status: " + it.Status + "\n")
	b.WriteString("opened: " + it.Opened + "\n")
	if it.Closed != "" {
		b.WriteString("closed: " + it.Closed + "\n")
	}
	if it.WakeStatus != "" {
		b.WriteString("wake_status: " + quoteYAML(it.WakeStatus) + "\n")
	}
	if it.WakeAttemptedAt != "" {
		b.WriteString("wake_attempted_at: " + quoteYAML(it.WakeAttemptedAt) + "\n")
	}
	if it.WakeAdapter != "" {
		b.WriteString("wake_adapter: " + quoteYAML(it.WakeAdapter) + "\n")
	}
	if it.WakeError != "" {
		b.WriteString("wake_error: " + quoteYAML(it.WakeError) + "\n")
	}
	b.WriteString("---\n\n## Instructions\n\n")
	b.WriteString(strings.TrimSpace(it.Instructions))
	b.WriteString("\n")
	if it.Status == "closed" || strings.TrimSpace(it.Result) != "" {
		result := strings.TrimSpace(it.Result)
		if result == "" {
			result = "(closed without result)"
		}
		b.WriteString("\n## Result\n\n" + result + "\n")
	}
	return b.String()
}

// ExportItem writes ONE item's canonical markdown into dir and returns the
// file path — the §2b axiom-8 dual-write: the store row is the dispatch
// authority; the file is the human audit view, byte-identical to what the
// file router's own writers would have produced.
func (s *Store) ExportItem(dir, id string) (string, error) {
	it, err := s.Get(id)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("routerstore: ExportItem: mkdir %q: %w", dir, err)
	}
	path := filepath.Join(dir, it.ID+".md")
	if err := os.WriteFile(path, []byte(renderMarkdown(it)), 0o644); err != nil {
		return "", fmt.Errorf("routerstore: ExportItem %q: %w", id, err)
	}
	return path, nil
}
