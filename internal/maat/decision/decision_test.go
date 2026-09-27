package decision

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAppendReadFilterSort(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.jsonl")

	old := New("reservation grant", "codex-pantheon", "m5", "window overlap", "codex-pantheon", "granted", "no conflict", "")
	old["time"] = "2026-09-25T00:00:00Z"
	old["host"] = "m5"
	if err := Append(path, old); err != nil {
		t.Fatalf("append old: %v", err)
	}

	recent := New("guard", "claude-pantheon", "m1", "diff coverage", "main", "PASS", "coverage 92%", "ci-run-42")
	recent["time"] = "2026-09-26T12:00:00Z"
	recent["host"] = "m1"
	if err := Append(path, recent); err != nil {
		t.Fatalf("append recent: %v", err)
	}

	lines, err := Read(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d", len(lines))
	}
	if lines[0].ID == "" || lines[0].ID == lines[1].ID {
		t.Fatalf("expected distinct content-derived IDs, got %q %q", lines[0].ID, lines[1].ID)
	}

	SortDesc(lines)
	if lines[0].Host() != "m1" {
		t.Fatalf("want newest first (m1), got %s", lines[0].Host())
	}

	since, _ := time.Parse(time.RFC3339, "2026-09-26T00:00:00Z")
	recentOnly := Filter(lines, "", "", since)
	if len(recentOnly) != 1 || recentOnly[0].Host() != "m1" {
		t.Fatalf("since filter: want 1 line (m1), got %d", len(recentOnly))
	}

	byKind := Filter(lines, "guard", "", time.Time{})
	if len(byKind) != 1 || byKind[0].Determination() != "PASS" {
		t.Fatalf("kind filter: want 1 PASS guard line, got %+v", byKind)
	}
}

func TestReadMissingFile(t *testing.T) {
	lines, err := Read(filepath.Join(t.TempDir(), "nope.jsonl"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if lines != nil {
		t.Fatalf("want nil lines for missing file, got %v", lines)
	}
}
