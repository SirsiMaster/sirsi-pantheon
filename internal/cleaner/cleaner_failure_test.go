package cleaner

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

// CleanFile + DeleteFile are the last-line deletion engine; Rule A16 requires
// their FAILURE path be tested. The existing tests used a Mock whose MoveToTrash
// always succeeded, so the "trash failed" branch was never exercised, and
// DeleteFile(useTrash=true) was never tested at all. Mock trash error-injection
// closes both gaps.

func TestCleanFile_TrashFailureSurfacedAndNotRecorded(t *testing.T) {
	platform.Set(&platform.Mock{TrashErr: errors.New("trash unavailable")})
	defer platform.Reset()

	dir := t.TempDir()
	fp := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(fp, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	log := &DecisionLog{SessionID: "t", path: filepath.Join(dir, "d.json")}

	if _, err := CleanFile(fp, false, "reason", "g", "h", log); err == nil {
		t.Fatal("CleanFile must surface the MoveToTrash error")
	}
	// A failed trash must NOT be recorded as a successful 'trash' decision —
	// the ledger would otherwise claim the file was reversibly removed when it
	// is still in place.
	for _, d := range log.Decisions {
		if d.Action == "trash" {
			t.Fatalf("a failed trash must not record a 'trash' decision: %+v", d)
		}
	}
}

func TestDeleteFile_TrashFailureReturnsError(t *testing.T) {
	platform.Set(&platform.Mock{TrashErr: errors.New("trash unavailable")})
	defer platform.Reset()

	dir := t.TempDir()
	fp := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(fp, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := DeleteFile(fp, false, true); err == nil { // useTrash=true
		t.Fatal("DeleteFile(useTrash) must surface the trash error")
	}
}

func TestDeleteFile_TrashSuccess(t *testing.T) {
	m := &platform.Mock{}
	platform.Set(m)
	defer platform.Reset()

	dir := t.TempDir()
	fp := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(fp, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	size, err := DeleteFile(fp, false, true) // useTrash=true, never covered before
	if err != nil {
		t.Fatalf("trash delete should succeed: %v", err)
	}
	if size <= 0 {
		t.Errorf("expected positive freed size, got %d", size)
	}
	if len(m.TrashCalls) != 1 || m.TrashCalls[0] != fp {
		t.Fatalf("file should have been moved to trash, got %v", m.TrashCalls)
	}
}
