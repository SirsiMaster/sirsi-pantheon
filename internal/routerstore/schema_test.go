package routerstore

import (
	"path/filepath"
	"testing"
)

func TestCurrentSchemaMax(t *testing.T) {
	got := CurrentSchemaMax()
	if got <= 0 {
		t.Fatalf("CurrentSchemaMax() = %d, want > 0", got)
	}
	// Must equal the last entry in the migrations slice.
	want := migrations[len(migrations)-1].version
	if got != want {
		t.Fatalf("CurrentSchemaMax() = %d, want %d", got, want)
	}
}

func TestReadLiveSchemaVersion_emptyPath(t *testing.T) {
	v, err := ReadLiveSchemaVersion("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != 0 {
		t.Fatalf("empty path: got %d, want 0", v)
	}
}

func TestReadLiveSchemaVersion_memoryPath(t *testing.T) {
	v, err := ReadLiveSchemaVersion(":memory:")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != 0 {
		t.Fatalf(":memory: path: got %d, want 0", v)
	}
}

func TestReadLiveSchemaVersion_freshDB(t *testing.T) {
	// A freshly-created store (via Open) is fully migrated — its user_version
	// should equal CurrentSchemaMax.
	dir := t.TempDir()
	path := filepath.Join(dir, "router.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	s.Close()

	got, err := ReadLiveSchemaVersion(path)
	if err != nil {
		t.Fatalf("ReadLiveSchemaVersion: %v", err)
	}
	want := CurrentSchemaMax()
	if got != want {
		t.Fatalf("fresh DB schema version = %d, want %d (CurrentSchemaMax)", got, want)
	}
}

func TestReadLiveSchemaVersion_nonexistent(t *testing.T) {
	// A nonexistent file should open as schema 0 (SQLite creates the file)
	// or return an error — but NOT return CurrentSchemaMax silently.
	// In practice modernc creates an empty file with user_version=0.
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.db")
	got, err := ReadLiveSchemaVersion(path)
	if err != nil {
		// Acceptable — some platforms refuse to open missing files.
		return
	}
	if got != 0 {
		t.Fatalf("nonexistent DB: got schema %d, want 0", got)
	}
}

func TestDefaultStorePath(t *testing.T) {
	p := DefaultStorePath()
	if p == "" {
		t.Fatal("DefaultStorePath() returned empty string")
	}
	if filepath.Base(p) != "router.db" {
		t.Fatalf("DefaultStorePath() base = %q, want router.db", filepath.Base(p))
	}
}
