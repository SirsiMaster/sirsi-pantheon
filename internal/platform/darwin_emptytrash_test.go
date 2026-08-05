//go:build darwin

package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// EmptyTrash is the only operation in this codebase with NO undo. Every test
// here is a way it could destroy something it was never asked to.

func withFakeTrash(t *testing.T) (home, trash string) {
	t.Helper()
	home = t.TempDir()
	trash = filepath.Join(home, ".Trash")
	if err := os.MkdirAll(trash, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	return home, trash
}

func TestEmptyTrash_DeletesOnlyNamedEntries(t *testing.T) {
	_, trash := withFakeTrash(t)
	keep := filepath.Join(trash, "keep.txt")
	gone := filepath.Join(trash, "gone.txt")
	if err := os.WriteFile(keep, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gone, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}

	d := &Darwin{}
	deleted, freed, err := d.EmptyTrash([]string{gone})
	if err != nil {
		t.Fatalf("EmptyTrash: %v", err)
	}
	if len(deleted) != 1 {
		t.Fatalf("deleted %d entries, want 1", len(deleted))
	}
	if freed != 10 {
		t.Fatalf("freed = %d, want 10", freed)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal("an unnamed entry was destroyed — EmptyTrash must delete ONLY what it was given")
	}
}

// Containment is the whole safety property: a path outside ~/.Trash must be
// REFUSED, not skipped. A permanent delete that quietly declines is worse than
// one that errors, because the caller reports success either way.
func TestEmptyTrash_RefusesPathOutsideTrash(t *testing.T) {
	home, _ := withFakeTrash(t)
	precious := filepath.Join(home, "Documents", "thesis.txt")
	if err := os.MkdirAll(filepath.Dir(precious), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(precious, []byte("years of work"), 0o600); err != nil {
		t.Fatal(err)
	}

	d := &Darwin{}
	_, _, err := d.EmptyTrash([]string{precious})
	if err == nil {
		t.Fatal("a path outside ~/.Trash must be REFUSED, not deleted")
	}
	if !strings.Contains(err.Error(), "not a direct child") {
		t.Fatalf("error should name the containment rule, got %v", err)
	}
	if _, serr := os.Stat(precious); serr != nil {
		t.Fatal("the out-of-Trash file was DELETED — catastrophic")
	}
}

// A symlink inside the Trash must not steer the delete to its target.
func TestEmptyTrash_SymlinkDoesNotEscape(t *testing.T) {
	home, trash := withFakeTrash(t)
	target := filepath.Join(home, "Documents", "real.txt")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("real"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(trash, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	d := &Darwin{}
	if _, _, err := d.EmptyTrash([]string{link}); err != nil {
		t.Fatalf("deleting a symlink that LIVES in Trash is legitimate: %v", err)
	}
	if _, serr := os.Stat(target); serr != nil {
		t.Fatal("the symlink TARGET outside Trash was deleted — the delete followed the link")
	}
	if _, lerr := os.Lstat(link); lerr == nil {
		t.Fatal("the symlink itself should be gone")
	}
}

func TestEmptyTrash_RefusesTheTrashDirectoryItself(t *testing.T) {
	_, trash := withFakeTrash(t)
	d := &Darwin{}
	if _, _, err := d.EmptyTrash([]string{trash}); err == nil {
		t.Fatal("removing ~/.Trash itself must be refused — empty its CONTENTS, never the directory")
	}
	if _, serr := os.Stat(trash); serr != nil {
		t.Fatal("~/.Trash was removed")
	}
}

func TestTrashContents_ListsWithSizesAndSkipsDSStore(t *testing.T) {
	_, trash := withFakeTrash(t)
	if err := os.WriteFile(filepath.Join(trash, "a.bin"), make([]byte, 42), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(trash, ".DS_Store"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	d := &Darwin{}
	got, err := d.TrashContents()
	if err != nil {
		t.Fatalf("TrashContents: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("entries = %d, want 1 (.DS_Store must not be listed as user content)", len(got))
	}
	if got[0].Bytes != 42 {
		t.Fatalf("size = %d, want 42", got[0].Bytes)
	}
}

// An empty or absent Trash is a normal state, never an error.
func TestTrashContents_EmptyIsNotAnError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home) // no .Trash at all
	d := &Darwin{}
	got, err := d.TrashContents()
	if err != nil {
		t.Fatalf("absent Trash must not error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("entries = %d, want 0", len(got))
	}
}
