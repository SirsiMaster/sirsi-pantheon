package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFirstMeaningfulLine(t *testing.T) {
	cases := map[string]string{
		"":                              "",
		"\n\n  \n":                      "",
		"hello":                         "hello",
		"\n\n  first real line\nsecond": "first real line",
		"\x1b[32mgreen result\x1b[0m":   "green result", // ANSI stripped
	}
	for in, want := range cases {
		if got := firstMeaningfulLine(in); got != want {
			t.Errorf("firstMeaningfulLine(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStripANSI(t *testing.T) {
	if got := stripANSI("\x1b[1;33m𓆄 Ma'at\x1b[0m audit OK"); got != "𓆄 Ma'at audit OK" {
		t.Errorf("stripANSI = %q", got)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("  short  ", 140); got != "short" {
		t.Errorf("truncate trims+keeps short = %q", got)
	}
	long := "0123456789"
	if got := truncate(long, 5); got != "0123…" {
		t.Errorf("truncate(%q,5) = %q, want 0123…", long, got)
	}
}

func TestFormatTrashSize(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1 KB"},
		{1536, "1 KB"},
		{1 << 20, "1.0 MB"},
		{int64(1.5 * float64(1<<20)), "1.5 MB"},
		{1 << 30, "1.0 GB"},
	}
	for _, c := range cases {
		if got := formatTrashSize(c.in); got != c.want {
			t.Errorf("formatTrashSize(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTrashInfo_Empty(t *testing.T) {
	tmp := t.TempDir()
	trashDir := filepath.Join(tmp, ".Trash")
	if err := os.Mkdir(trashDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// No files → items=0, size=0.
	// trashInfo reads ~/.Trash so we can't inject the dir directly, but we
	// can verify dirSize on an empty dir returns 0 (covers the helper).
	if got := dirSize(tmp, ".Trash"); got != 0 {
		t.Errorf("dirSize empty dir = %d, want 0", got)
	}
}

func TestDirSize(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "a"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "b"), []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}
	// dirSize takes (parent, name) — pass the parent and the base of tmp.
	got := dirSize(filepath.Dir(tmp), filepath.Base(tmp))
	if got != 10 {
		t.Errorf("dirSize = %d, want 10", got)
	}
}

func TestMin(t *testing.T) {
	if min(3, 5) != 3 || min(5, 3) != 3 || min(4, 4) != 4 {
		t.Error("min failed")
	}
}
