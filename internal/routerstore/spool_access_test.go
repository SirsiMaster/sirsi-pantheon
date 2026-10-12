package routerstore

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSpoolSlotPermissionFailureIsNotSaturation(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses directory write permissions")
	}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "slots"), 0o500); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	_, err = acquireSlotIn(root, "slots")
	if !errors.Is(err, fs.ErrPermission) || strings.Contains(err.Error(), "in flight") {
		t.Fatalf("access denial must retain its cause, not claim saturation: %v", err)
	}
	if !strings.Contains(err.Error(), "request not published") {
		t.Fatal(err)
	}
}

func TestSpoolSlotInvalidDirectoryIsNotSaturation(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "slots"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	_, err = acquireSlotIn(root, "slots")
	if err == nil || strings.Contains(err.Error(), "in flight") {
		t.Fatalf("bad directory: %v", err)
	}
}

func TestSpoolSlotStaleRecoveryRemainsBounded(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "slots"), 0o700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "slots", "00")
	if err := os.WriteFile(p, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * spoolStaleAfter)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	slot, err := acquireSlotIn(root, "slots")
	if err != nil || slot != filepath.Join("slots", "00") {
		t.Fatalf("stale recovery %q: %v", slot, err)
	}
}
