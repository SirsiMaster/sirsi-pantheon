//go:build darwin

package platform

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDarwinMoveToTrash_Native verifies the native (no-Finder) trash path: a file
// is MOVED to ~/.Trash (recoverable), not deleted, and works without any
// Automation permission (the whole point — it must work from a launchd agent).
func TestDarwinMoveToTrash_Native(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	src := filepath.Join(home, ".sirsi-trash-smoke.txt")
	if err := os.WriteFile(src, []byte("smoke"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	dest := filepath.Join(home, ".Trash", filepath.Base(src))
	defer os.Remove(dest) // clean up the trashed item

	d := &Darwin{}
	if err := d.MoveToTrash(src); err != nil {
		t.Fatalf("MoveToTrash: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("original still present — file was not moved")
	}
	if _, err := os.Stat(dest); err != nil {
		t.Errorf("file not recoverable from ~/.Trash: %v", err)
	}
}

// Collision: a same-named item already in Trash must not be clobbered — the new
// one lands under a timestamped name and both survive.
func TestDarwinMoveToTrash_NoClobber(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	base := ".sirsi-trash-clobber.txt"
	existing := filepath.Join(home, ".Trash", base)
	if err := os.WriteFile(existing, []byte("pre-existing"), 0o644); err != nil {
		t.Fatalf("seed trash item: %v", err)
	}
	defer os.Remove(existing)

	src := filepath.Join(home, base)
	if err := os.WriteFile(src, []byte("new"), 0o644); err != nil {
		t.Fatalf("seed src: %v", err)
	}
	d := &Darwin{}
	if err := d.MoveToTrash(src); err != nil {
		t.Fatalf("MoveToTrash: %v", err)
	}
	// The pre-existing trash item is untouched.
	if b, _ := os.ReadFile(existing); string(b) != "pre-existing" {
		t.Error("pre-existing Trash item was clobbered")
	}
	// Clean up any timestamped sibling the move created.
	matches, _ := filepath.Glob(filepath.Join(home, ".Trash", base+" *"))
	for _, m := range matches {
		os.Remove(m)
	}
}
