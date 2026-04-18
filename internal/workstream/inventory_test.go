package workstream

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScanInventory(t *testing.T) {
	// ScanInventory uses exec.LookPath internally (AllLaunchers().Installed()),
	// so we test that it returns a valid struct with the right shape.
	inv := ScanInventory(&mockPlatform{home: t.TempDir(), shell: "/bin/zsh"})

	if inv.Version != inventoryVersion {
		t.Errorf("version = %d, want %d", inv.Version, inventoryVersion)
	}
	if inv.OS == "" {
		t.Error("OS should not be empty")
	}
	if inv.Arch == "" {
		t.Error("Arch should not be empty")
	}
	if inv.Shell != "zsh" {
		t.Errorf("Shell = %q, want zsh", inv.Shell)
	}
	if len(inv.Tools) != len(AllLaunchers()) {
		t.Errorf("Tools count = %d, want %d", len(inv.Tools), len(AllLaunchers()))
	}
	if inv.ScannedAt.IsZero() {
		t.Error("ScannedAt should be set")
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inventory.json")

	inv := &Inventory{
		Version:   1,
		ScannedAt: time.Now().Truncate(time.Second),
		OS:        "darwin",
		Arch:      "arm64",
		Shell:     "zsh",
		HomeDir:   "/Users/test",
		Tools: []ToolStatus{
			{ID: "claude", Name: "Claude Code", Kind: "ai", Installed: true},
			{ID: "vscode", Name: "VS Code", Kind: "ide", Installed: false},
		},
		GitRepos: []GitRepo{
			{Name: "myproject", Path: "/Users/test/Development/myproject"},
		},
	}

	// Save
	origPath := InventoryPath
	InventoryPath = func() string { return path }
	defer func() { InventoryPath = origPath }()

	if err := SaveInventory(inv); err != nil {
		t.Fatal(err)
	}

	// Load
	loaded, err := LoadInventory()
	if err != nil {
		t.Fatal(err)
	}

	if loaded.Version != inv.Version {
		t.Errorf("version mismatch: %d vs %d", loaded.Version, inv.Version)
	}
	if loaded.OS != inv.OS {
		t.Errorf("OS mismatch: %s vs %s", loaded.OS, inv.OS)
	}
	if len(loaded.Tools) != 2 {
		t.Errorf("tools count = %d, want 2", len(loaded.Tools))
	}
	if len(loaded.GitRepos) != 1 {
		t.Errorf("repos count = %d, want 1", len(loaded.GitRepos))
	}
	if loaded.GitRepos[0].Name != "myproject" {
		t.Errorf("repo name = %s, want myproject", loaded.GitRepos[0].Name)
	}
}

func TestLoadInventory_Missing(t *testing.T) {
	origPath := InventoryPath
	InventoryPath = func() string { return filepath.Join(t.TempDir(), "nonexistent.json") }
	defer func() { InventoryPath = origPath }()

	_, err := LoadInventory()
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestInstalledFilters(t *testing.T) {
	inv := &Inventory{
		Tools: []ToolStatus{
			{ID: "claude", Kind: "ai", Installed: true},
			{ID: "codex", Kind: "ai", Installed: false},
			{ID: "vscode", Kind: "ide", Installed: true},
			{ID: "cursor", Kind: "ide", Installed: true},
			{ID: "zed", Kind: "ide", Installed: false},
		},
	}

	ai := inv.InstalledAI()
	if len(ai) != 1 || ai[0].ID != "claude" {
		t.Errorf("InstalledAI = %v, want [claude]", ai)
	}

	ides := inv.InstalledIDEs()
	if len(ides) != 2 {
		t.Errorf("InstalledIDEs count = %d, want 2", len(ides))
	}
}

func TestIsStale(t *testing.T) {
	fresh := &Inventory{ScannedAt: time.Now()}
	if fresh.IsStale() {
		t.Error("fresh inventory should not be stale")
	}

	old := &Inventory{ScannedAt: time.Now().Add(-8 * 24 * time.Hour)}
	if !old.IsStale() {
		t.Error("8-day-old inventory should be stale")
	}

	boundary := &Inventory{ScannedAt: time.Now().Add(-7*24*time.Hour + time.Minute)}
	if boundary.IsStale() {
		t.Error("just-under-7-day inventory should not be stale")
	}
}

func TestDiscoverGitRepos(t *testing.T) {
	home := t.TempDir()

	// Create Development/project-a/.git
	os.MkdirAll(filepath.Join(home, "Development", "project-a", ".git"), 0755)
	// Create Development/org/project-b/.git (level 2)
	os.MkdirAll(filepath.Join(home, "Development", "org", "project-b", ".git"), 0755)
	// Create Development/not-a-repo (no .git)
	os.MkdirAll(filepath.Join(home, "Development", "not-a-repo"), 0755)
	// Create Projects/project-c/.git
	os.MkdirAll(filepath.Join(home, "Projects", "project-c", ".git"), 0755)

	repos := discoverGitRepos(home)

	names := make(map[string]bool)
	for _, r := range repos {
		names[r.Name] = true
	}

	if !names["project-a"] {
		t.Error("should find project-a")
	}
	if !names["project-b"] {
		t.Error("should find project-b at level 2")
	}
	if !names["project-c"] {
		t.Error("should find project-c in Projects/")
	}
	if names["not-a-repo"] {
		t.Error("should not find not-a-repo")
	}
	if len(repos) != 3 {
		t.Errorf("expected 3 repos, got %d: %v", len(repos), repos)
	}
}

func TestInventoryJSON(t *testing.T) {
	inv := &Inventory{
		Version:   1,
		ScannedAt: time.Now(),
		OS:        "darwin",
		Arch:      "arm64",
		Tools:     []ToolStatus{{ID: "claude", Name: "Claude Code", Kind: "ai", Installed: true}},
	}
	data, err := json.MarshalIndent(inv, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Error("JSON should not be empty")
	}
}

// mockPlatform is a minimal mock for testing ScanInventory.
type mockPlatform struct {
	home  string
	shell string
}

func (m *mockPlatform) Getenv(key string) string {
	if key == "SHELL" {
		return m.shell
	}
	return ""
}
func (m *mockPlatform) UserHomeDir() (string, error)                        { return m.home, nil }
func (m *mockPlatform) Getwd() (string, error)                              { return m.home, nil }
func (m *mockPlatform) Command(name string, args ...string) ([]byte, error) { return nil, nil }
func (m *mockPlatform) Processes() ([]string, error)                        { return nil, nil }
func (m *mockPlatform) Name() string                                        { return "mock" }
func (m *mockPlatform) SupportsTrash() bool                                 { return false }
func (m *mockPlatform) MoveToTrash(path string) error                       { return nil }
func (m *mockPlatform) ProtectedPrefixes() []string                         { return nil }
func (m *mockPlatform) OpenBrowser(url string) error                        { return nil }
func (m *mockPlatform) PickFolder() (string, error)                         { return "", nil }
func (m *mockPlatform) ReadDir(dirname string) ([]os.DirEntry, error)       { return nil, nil }
func (m *mockPlatform) Kill(pid int) error                                  { return nil }
