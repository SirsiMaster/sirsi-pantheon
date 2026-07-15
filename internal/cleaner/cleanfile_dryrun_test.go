package cleaner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

// CleanFile now honors Rule A1's "every destructive op has a no-op preview":
// dryRun reports the size, does NOT touch the file, and logs a "dry-run"
// decision rather than a "trash"/"delete" (so the ledger can't claim a removal
// that never happened).
func TestCleanFile_DryRunPreviewsWithoutRemoving(t *testing.T) {
	m := &platform.Mock{}
	platform.Set(m)
	defer platform.Reset()

	dir := t.TempDir()
	fp := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(fp, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	log := &DecisionLog{SessionID: "t", path: filepath.Join(dir, "d.json")}

	size, err := CleanFile(fp, true, "reason", "g", "h", log) // dryRun=true
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if size <= 0 {
		t.Errorf("dry-run should still report the size, got %d", size)
	}
	if _, err := os.Stat(fp); err != nil {
		t.Fatal("dry-run must NOT remove the file")
	}
	if len(m.TrashCalls) != 0 {
		t.Fatalf("dry-run must not call MoveToTrash, got %v", m.TrashCalls)
	}
	found := false
	for _, d := range log.Decisions {
		switch d.Action {
		case "dry-run":
			found = true
		case "trash", "delete":
			t.Fatalf("dry-run must not record a %q decision", d.Action)
		}
	}
	if !found {
		t.Fatal("dry-run must record a 'dry-run' decision")
	}
}
