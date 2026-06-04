package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Scan for Waste":      "scan-for-waste",
		"𓆄 Ma'at — Quality":   "maat-quality",
		"  leading/trailing ": "leadingtrailing",
		"":                    "result",
		"!!!":                 "result",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
	// Long titles are bounded.
	long := strings.Repeat("a", 100)
	if got := slugify(long); len(got) > 60 {
		t.Errorf("slugify long stem = %d chars, want <=60", len(got))
	}
}

func TestClaudeMCPLinked(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// No config → not linked.
	if claudeMCPLinked() {
		t.Fatal("expected not linked with no config")
	}
	// A .claude.json registering the sirsi MCP server → linked.
	cfg := `{"mcpServers":{"sirsi":{"command":"sirsi","args":["mcp"]}}}`
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if !claudeMCPLinked() {
		t.Error("expected linked when ~/.claude.json contains the sirsi server")
	}

	// A config WITHOUT sirsi → not linked (no false positive).
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"mcpServers":{"other":{}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if claudeMCPLinked() {
		t.Error("expected not linked when sirsi is absent from config")
	}
}

func TestIDEAppInstalled(t *testing.T) {
	dir := t.TempDir()
	// Stub a Cursor.app bundle in a temp "Applications" dir.
	if err := os.MkdirAll(filepath.Join(dir, "Cursor.app"), 0o755); err != nil {
		t.Fatal(err)
	}
	orig := appSearchDirs
	appSearchDirs = func() []string { return []string{dir} }
	defer func() { appSearchDirs = orig }()

	if !ideAppInstalled("cursor") {
		t.Error("expected cursor detected via .app bundle")
	}
	if ideAppInstalled("zed") {
		t.Error("zed has no .app here — should be false")
	}
	if ideAppInstalled("not-an-ide") {
		t.Error("unknown tool id should be false")
	}
}
