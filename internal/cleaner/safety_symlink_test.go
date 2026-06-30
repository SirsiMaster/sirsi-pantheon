package cleaner

import (
	"os"
	"path/filepath"
	"testing"
)

// TestValidatePath_SymlinkLeafToProtected: a symlink with an innocent name whose
// TARGET is a protected location must be BLOCKED — the lexical name alone looks
// safe, so without symlink resolution the protected-path guarantee (Rule A1) is
// bypassable. /System is a darwin protected prefix and always exists.
func TestValidatePath_SymlinkLeafToProtected(t *testing.T) {
	if _, err := os.Stat("/System/Library"); err != nil {
		t.Skip("/System/Library not present (non-darwin)")
	}
	link := filepath.Join(t.TempDir(), "cache") // innocent-looking name
	if err := os.Symlink("/System/Library", link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if err := ValidatePath(link); err == nil {
		t.Fatal("a symlink named 'cache' pointing into /System/Library must be BLOCKED")
	}
}

// TestValidatePath_SymlinkedParentToProtected: deletion of a path UNDER a
// symlinked parent that resolves into a protected location must be BLOCKED.
func TestValidatePath_SymlinkedParentToProtected(t *testing.T) {
	if _, err := os.Stat("/System/Library"); err != nil {
		t.Skip("/System/Library not present (non-darwin)")
	}
	parent := filepath.Join(t.TempDir(), "root")
	if err := os.Symlink("/System", parent); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	target := filepath.Join(parent, "Library") // resolves to /System/Library
	if err := ValidatePath(target); err == nil {
		t.Fatal("a path under a parent symlinked into /System must be BLOCKED")
	}
}

// TestValidatePath_PlainTempFileAllowed is the sanity counter-case: a normal,
// non-symlinked temp path is still allowed (the hardening must not over-block).
func TestValidatePath_PlainTempFileAllowed(t *testing.T) {
	f := filepath.Join(t.TempDir(), "node_modules")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePath(f); err != nil {
		t.Fatalf("a plain temp file must be allowed, got %v", err)
	}
}
