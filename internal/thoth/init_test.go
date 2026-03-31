package thoth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── init.go Tests ───────────────────────────────────────────────────

func TestDetectProject_Go(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module github.com/example/myproject\n\ngo 1.21\n"), 0o644)

	info := DetectProject(tmp)
	if info.Language != "Go" {
		t.Errorf("DetectProject(Go) language = %q, want Go", info.Language)
	}
	if info.Name != "myproject" {
		t.Errorf("DetectProject(Go) name = %q, want myproject", info.Name)
	}
}

func TestDetectProject_TypeScript(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "package.json"), []byte(`{"name": "my-app", "version": "2.0.0"}`), 0o644)

	info := DetectProject(tmp)
	if info.Language != "TypeScript/JavaScript" {
		t.Errorf("DetectProject(TS) language = %q", info.Language)
	}
	if info.Name != "my-app" {
		t.Errorf("DetectProject(TS) name = %q, want my-app", info.Name)
	}
	if info.Version != "2.0.0" {
		t.Errorf("DetectProject(TS) version = %q, want 2.0.0", info.Version)
	}
}

func TestDetectProject_NextJS(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "package.json"), []byte(`{"name": "next-app"}`), 0o644)
	os.WriteFile(filepath.Join(tmp, "next.config.js"), []byte("module.exports = {}"), 0o644)

	info := DetectProject(tmp)
	if info.Language != "TypeScript (Next.js)" {
		t.Errorf("DetectProject(Next.js) language = %q", info.Language)
	}
}

func TestDetectProject_Rust(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "Cargo.toml"), []byte(`[package]\nname = "rustproj"\nversion = "0.2.0"`), 0o644)

	info := DetectProject(tmp)
	if info.Language != "Rust" {
		t.Errorf("DetectProject(Rust) language = %q", info.Language)
	}
	if info.Name != "rustproj" {
		t.Errorf("DetectProject(Rust) name = %q, want rustproj", info.Name)
	}
}

func TestDetectProject_Python(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "pyproject.toml"), []byte("[tool.poetry]\nname = \"pyapp\""), 0o644)

	info := DetectProject(tmp)
	if info.Language != "Python" {
		t.Errorf("DetectProject(Python) language = %q", info.Language)
	}
}

func TestDetectProject_Unknown(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	info := DetectProject(tmp)
	if info.Language != "unknown" {
		t.Errorf("DetectProject(empty) language = %q, want unknown", info.Language)
	}
}

func TestCountSourceLines(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Create a Go file with 10 lines
	content := strings.Repeat("package main\n", 10)
	os.WriteFile(filepath.Join(tmp, "main.go"), []byte(content), 0o644)

	count := CountSourceLines(tmp)
	if count < 10 {
		t.Errorf("CountSourceLines() = %d, want >= 10", count)
	}
}

func TestCountSourceLines_ExcludesDirs(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Create files in excluded dirs
	for _, dir := range []string{".git", "node_modules", "vendor"} {
		os.MkdirAll(filepath.Join(tmp, dir), 0o755)
		os.WriteFile(filepath.Join(tmp, dir, "big.go"), []byte(strings.Repeat("line\n", 1000)), 0o644)
	}

	count := CountSourceLines(tmp)
	if count != 0 {
		t.Errorf("CountSourceLines(excluded) = %d, want 0", count)
	}
}

func TestScanArchitecture(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	for _, dir := range []string{"cmd", "internal", "docs", ".git", "node_modules"} {
		os.MkdirAll(filepath.Join(tmp, dir), 0o755)
	}
	os.WriteFile(filepath.Join(tmp, "main.go"), []byte("package main"), 0o644)

	dirs := ScanArchitecture(tmp)

	// Should include cmd, internal, docs but NOT .git or node_modules
	has := map[string]bool{}
	for _, d := range dirs {
		has[d] = true
	}
	if !has["cmd"] || !has["internal"] || !has["docs"] {
		t.Errorf("ScanArchitecture() = %v, missing expected dirs", dirs)
	}
	if has[".git"] || has["node_modules"] {
		t.Errorf("ScanArchitecture() = %v, should exclude .git and node_modules", dirs)
	}
}

func TestInit_NonInteractive(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Create a Go project
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module github.com/test/proj\n\ngo 1.21\n"), 0o644)
	os.MkdirAll(filepath.Join(tmp, "cmd"), 0o755)

	err := Init(InitOptions{RepoRoot: tmp, Yes: true})
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	// Verify files were created
	if _, err := os.Stat(filepath.Join(tmp, ".thoth", "memory.yaml")); err != nil {
		t.Error("memory.yaml not created")
	}
	if _, err := os.Stat(filepath.Join(tmp, ".thoth", "journal.md")); err != nil {
		t.Error("journal.md not created")
	}
	if _, err := os.Stat(filepath.Join(tmp, ".thoth", "artifacts", "README.md")); err != nil {
		t.Error("artifacts/README.md not created")
	}

	// Check memory content
	data, _ := os.ReadFile(filepath.Join(tmp, ".thoth", "memory.yaml"))
	content := string(data)
	if !strings.Contains(content, "project: proj") {
		t.Errorf("memory.yaml missing project name, got:\n%s", content)
	}
	if !strings.Contains(content, "language: Go") {
		t.Errorf("memory.yaml missing language, got:\n%s", content)
	}
}

func TestInit_WithOverrides(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	err := Init(InitOptions{
		RepoRoot: tmp,
		Name:     "custom-name",
		Language: "Kotlin",
		Version:  "3.0.0",
		Yes:      true,
	})
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmp, ".thoth", "memory.yaml"))
	content := string(data)
	if !strings.Contains(content, "project: custom-name") {
		t.Error("memory.yaml missing custom name")
	}
	if !strings.Contains(content, "language: Kotlin") {
		t.Error("memory.yaml missing custom language")
	}
	if !strings.Contains(content, "version: 3.0.0") {
		t.Error("memory.yaml missing custom version")
	}
}

func TestInit_ExistingMemoryBlocksWithoutYes(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	os.MkdirAll(filepath.Join(tmp, ".thoth"), 0o755)
	os.WriteFile(filepath.Join(tmp, ".thoth", "memory.yaml"), []byte("existing"), 0o644)

	err := Init(InitOptions{RepoRoot: tmp})
	if err == nil {
		t.Error("Init() should error when memory exists and --yes not set")
	}
}

func TestInjectIDERules_CreatesNew(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	injected := InjectIDERules(tmp)
	if len(injected) == 0 {
		t.Error("InjectIDERules() should create IDE rules files")
	}

	// Check CLAUDE.md was created
	data, err := os.ReadFile(filepath.Join(tmp, "CLAUDE.md"))
	if err != nil {
		t.Fatal("CLAUDE.md not created")
	}
	if !strings.Contains(string(data), ".thoth/memory.yaml") {
		t.Error("CLAUDE.md missing Thoth instruction")
	}
}

func TestInjectIDERules_AppendsToExisting(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Create existing CLAUDE.md
	os.WriteFile(filepath.Join(tmp, "CLAUDE.md"), []byte("# Existing rules\n"), 0o644)

	injected := InjectIDERules(tmp)

	found := false
	for _, ide := range injected {
		if strings.Contains(ide, "Claude Code (appended)") {
			found = true
		}
	}
	if !found {
		t.Errorf("InjectIDERules() = %v, expected 'Claude Code (appended)'", injected)
	}

	data, _ := os.ReadFile(filepath.Join(tmp, "CLAUDE.md"))
	if !strings.Contains(string(data), "# Existing rules") {
		t.Error("CLAUDE.md lost existing content")
	}
	if !strings.Contains(string(data), ".thoth/memory.yaml") {
		t.Error("CLAUDE.md missing Thoth injection")
	}
}

func TestInjectIDERules_SkipsAlreadyInjected(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Create CLAUDE.md that already has Thoth
	os.WriteFile(filepath.Join(tmp, "CLAUDE.md"), []byte("Read .thoth/memory.yaml first\n"), 0o644)

	injected := InjectIDERules(tmp)

	for _, ide := range injected {
		if strings.Contains(ide, "Claude Code") {
			t.Errorf("Should not re-inject into CLAUDE.md, got %v", injected)
		}
	}
}

func TestExtractJSON(t *testing.T) {
	t.Parallel()
	data := `{"name": "my-project", "version": "1.2.3", "private": true}`

	if got := extractJSON(data, "name"); got != "my-project" {
		t.Errorf("extractJSON(name) = %q", got)
	}
	if got := extractJSON(data, "version"); got != "1.2.3" {
		t.Errorf("extractJSON(version) = %q", got)
	}
	if got := extractJSON(data, "missing"); got != "" {
		t.Errorf("extractJSON(missing) = %q, want empty", got)
	}
}

func TestCreateSessionWorkflow(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	created := createSessionWorkflow(tmp)
	if !created {
		t.Error("createSessionWorkflow() should return true for new project")
	}

	// Check file exists
	if _, err := os.Stat(filepath.Join(tmp, ".agent", "workflows", "session-start.md")); err != nil {
		t.Error("session-start.md not created")
	}

	// Second call should be idempotent
	created2 := createSessionWorkflow(tmp)
	if created2 {
		t.Error("createSessionWorkflow() should return false when already exists")
	}
}

func TestTemplateFS(t *testing.T) {
	t.Parallel()

	// Verify all embedded templates are accessible
	for _, name := range []string{"templates/memory.yaml", "templates/journal.md", "templates/session-start.md"} {
		data, err := templateFS.ReadFile(name)
		if err != nil {
			t.Errorf("templateFS.ReadFile(%q) error: %v", name, err)
		}
		if len(data) == 0 {
			t.Errorf("templateFS.ReadFile(%q) returned empty", name)
		}
	}
}
