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
	claudeCfg := filepath.Join(home, ".claude.json")
	write := func(s string) {
		if err := os.WriteFile(claudeCfg, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if claudeMCPLinked() {
		t.Fatal("no config → must be not linked")
	}

	// Top-level proper registration → linked.
	write(`{"mcpServers":{"sirsi":{"command":"sirsi","args":["mcp"]}}}`)
	if !claudeMCPLinked() {
		t.Error("valid top-level sirsi server should be linked")
	}

	// Per-project registration (how Claude Code often stores it) → linked.
	write(`{"projects":{"/x":{"mcpServers":{"sirsi":{"command":"/usr/local/bin/sirsi","args":["mcp"]}}}}}`)
	if !claudeMCPLinked() {
		t.Error("valid per-project sirsi server should be linked")
	}

	// FALSE-POSITIVE GUARDS (codex P0): a stray "sirsi" string must NOT read as linked.
	write(`{"someNote":"i love sirsi","mcpServers":{"other":{"command":"x","args":["y"]}}}`)
	if claudeMCPLinked() {
		t.Error("stray 'sirsi' string must not count as linked")
	}
	// sirsi present but wrong command → not linked.
	write(`{"mcpServers":{"sirsi":{"command":"notsirsi","args":["mcp"]}}}`)
	if claudeMCPLinked() {
		t.Error("wrong command must not count as linked")
	}
	// sirsi present, right command, but missing the mcp arg → not linked.
	write(`{"mcpServers":{"sirsi":{"command":"sirsi","args":["serve"]}}}`)
	if claudeMCPLinked() {
		t.Error("missing 'mcp' arg must not count as linked")
	}

	// Cursor config must NOT make Claude read as linked (no cross-client conflation).
	write(`{"mcpServers":{}}`)
	if err := os.MkdirAll(filepath.Join(home, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".cursor", "mcp.json"),
		[]byte(`{"mcpServers":{"sirsi":{"command":"sirsi","args":["mcp"]}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if claudeMCPLinked() {
		t.Error("Cursor config must not count as Claude linkage")
	}
	if !cursorMCPLinked() {
		t.Error("Cursor config with valid sirsi server should be cursor-linked")
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
