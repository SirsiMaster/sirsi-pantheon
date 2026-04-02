package seshat

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// AppleNotesSourceAdapter reads notes from Apple Notes via AppleScript.
type AppleNotesSourceAdapter struct{}

func (a *AppleNotesSourceAdapter) Name() string        { return "apple-notes" }
func (a *AppleNotesSourceAdapter) Description() string { return "Apple Notes (read via AppleScript)" }

// Ingest reads all Apple Notes and converts them to Knowledge Items.
func (a *AppleNotesSourceAdapter) Ingest(since time.Time) ([]KnowledgeItem, error) {
	// Get list of notes with their names, bodies, and modification dates
	script := `
		set output to ""
		tell application "Notes"
			repeat with n in notes
				set noteName to name of n
				set noteBody to plaintext of n
				set modDate to modification date of n
				-- Use tab as field separator, newline record separator
				set output to output & noteName & "	" & (modDate as string) & "	" & noteBody & "
---SESHAT_NOTE_SEP---
"
			end repeat
		end tell
		return output
	`

	cmd := exec.Command("osascript", "-e", script)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("AppleScript error (is Notes.app accessible?): %w", err)
	}

	notes := strings.Split(string(out), "---SESHAT_NOTE_SEP---")
	var items []KnowledgeItem

	for _, note := range notes {
		note = strings.TrimSpace(note)
		if note == "" {
			continue
		}

		parts := strings.SplitN(note, "\t", 3)
		if len(parts) < 3 {
			continue
		}

		title := strings.TrimSpace(parts[0])
		body := strings.TrimSpace(parts[2])

		if title == "" || body == "" {
			continue
		}

		items = append(items, KnowledgeItem{
			Title:   title,
			Summary: truncate(body, 300),
			References: []KIReference{
				{Type: "source", Value: "apple-notes"},
			},
		})
	}

	return items, nil
}

// AppleNotesTargetAdapter writes Knowledge Items to Apple Notes.
type AppleNotesTargetAdapter struct {
	// Folder is the Apple Notes folder to write to. Defaults to "Seshat".
	Folder string
}

func (a *AppleNotesTargetAdapter) Name() string        { return "apple-notes" }
func (a *AppleNotesTargetAdapter) Description() string { return "Apple Notes (write via AppleScript)" }

func (a *AppleNotesTargetAdapter) folder() string {
	if a.Folder != "" {
		return a.Folder
	}
	return "Seshat"
}

// Export writes Knowledge Items to Apple Notes in the configured folder.
func (a *AppleNotesTargetAdapter) Export(items []KnowledgeItem) error {
	folder := a.folder()

	// Ensure the target folder exists
	createFolder := fmt.Sprintf(`
		tell application "Notes"
			if not (exists folder "%s") then
				make new folder with properties {name:"%s"}
			end if
		end tell
	`, folder, folder)

	if err := exec.Command("osascript", "-e", createFolder).Run(); err != nil {
		return fmt.Errorf("create Notes folder '%s': %w", folder, err)
	}

	for _, ki := range items {
		// Escape quotes for AppleScript
		title := strings.ReplaceAll(ki.Title, `"`, `\"`)
		body := strings.ReplaceAll(ki.Summary, `"`, `\"`)

		// Build references section
		var refs []string
		for _, ref := range ki.References {
			refs = append(refs, fmt.Sprintf("• %s: %s", ref.Type, ref.Value))
		}
		if len(refs) > 0 {
			body += "\n\n---\nReferences:\n" + strings.Join(refs, "\n")
		}
		body = strings.ReplaceAll(body, `"`, `\"`)

		script := fmt.Sprintf(`
			tell application "Notes"
				tell folder "%s"
					make new note with properties {name:"%s", body:"%s"}
				end tell
			end tell
		`, folder, title, body)

		if err := exec.Command("osascript", "-e", script).Run(); err != nil {
			fmt.Printf("  ⚠️  Failed to create note '%s': %v\n", ki.Title, err)
			continue
		}
	}

	return nil
}
