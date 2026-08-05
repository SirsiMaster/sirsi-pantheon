package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal"
	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

// `sirsi anubis empty-trash` — the missing lever.
//
// `internal/jackal/advisory.go` has set Remediation = "Empty Trash" on the
// trash finding since it was written, and `internal/cleaner/safety.go` documents
// "Permanent delete only via explicit EmptyTrash after review" — but no such
// command existed. An alarming check that names a remedy nobody can run is the
// ADR-033 defect: the finding reads actionable and is not.
//
// Everything else in Anubis is trash-FIRST and recoverable. This is the one
// operation with no undo, so it is deliberately not part of `clean`, never
// implied by `--confirm`, and requires its own explicit `--yes`.
var emptyTrashYes bool

var anubisEmptyTrashCmd = &cobra.Command{
	Use:   "empty-trash",
	Short: "PERMANENTLY delete everything in ~/.Trash (dry-run by default; no undo)",
	Long: `Permanently delete the contents of ~/.Trash.

This is the only Sirsi operation with no undo. Every other clean path moves
items to the Trash and leaves them recoverable; this removes them for good.

Dry-run by default: it lists what would go and what it would free. Pass --yes
to actually delete. The Trash directory itself is never removed, and any path
that does not live directly in ~/.Trash is refused rather than skipped.`,
	RunE: runAnubisEmptyTrash,
}

func init() {
	anubisEmptyTrashCmd.Flags().BoolVar(&emptyTrashYes, "yes", false,
		"actually delete permanently (without this, lists only)")
	anubisCmd.AddCommand(anubisEmptyTrashCmd)
}

func runAnubisEmptyTrash(_ *cobra.Command, _ []string) error {
	// Through the interface, not a hardcoded Darwin: the platform package owns
	// which implementations have a trash, and non-darwin stubs REFUSE rather
	// than silently succeeding. Hardcoding the concrete type also broke the
	// cross-platform build — caught by CI lint, not by me.
	d := platform.Current()
	if !d.SupportsTrash() {
		return fmt.Errorf("this platform has no trash Sirsi can empty")
	}
	entries, err := d.TrashContents()
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Println("𓁟 Trash is already empty — nothing to delete.")
		return nil
	}

	var total int64
	paths := make([]string, 0, len(entries))
	for _, e := range entries {
		total += e.Bytes
		paths = append(paths, e.Path)
	}

	if !emptyTrashYes {
		fmt.Printf("𓁟 %d item(s) in Trash, %s total:\n", len(entries), jackal.FormatSize(total))
		for _, e := range entries {
			fmt.Printf("   · %s  (%s)\n", e.Name, jackal.FormatSize(e.Bytes))
		}
		fmt.Println("\nThis is PERMANENT — there is no undo, and Sirsi cannot restore these.")
		fmt.Println("Re-run with --yes to delete them.")
		return nil
	}

	deleted, freed, err := d.EmptyTrash(paths)
	// Report what DID happen even on error: a partial permanent delete must
	// never be summarized as a clean failure, or the user cannot tell what
	// survived.
	if len(deleted) > 0 {
		fmt.Printf("𓆄 Permanently deleted %d item(s) — %s freed.\n", len(deleted), jackal.FormatSize(freed))
	}
	if err != nil {
		return fmt.Errorf("empty trash stopped after %d item(s): %w", len(deleted), err)
	}
	return nil
}
