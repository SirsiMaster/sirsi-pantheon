package seba

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── Phase 1 Mapper Tests ────────────────────────────────────────────

func TestMapperRegistry(t *testing.T) {
	t.Parallel()
	if len(mapperRegistry) < 17 {
		t.Errorf("mapperRegistry has %d entries, want at least 17", len(mapperRegistry))
	}
}

func TestAllDiagramTypes_IncludesMappers(t *testing.T) {
	t.Parallel()
	all := AllDiagramTypes()
	// 6 core + 9 registered = 15
	if len(all) < 15 {
		t.Errorf("AllDiagramTypes = %d, want at least 15", len(all))
	}
}

func TestGenerateCallGraph(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	modDir := filepath.Join(tmp, "internal", "alpha")
	os.MkdirAll(modDir, 0o755)

	os.WriteFile(filepath.Join(modDir, "a.go"), []byte(`package alpha

func Foo() { Bar() }
func Bar() {}
`), 0o644)

	r, err := GenerateDiagram(tmp, DiagramCallGraph)
	if err != nil {
		t.Fatalf("callgraph: %v", err)
	}
	if !strings.Contains(r.Mermaid, "alpha_Foo") {
		t.Error("should contain Foo")
	}
	if !strings.Contains(r.Mermaid, "alpha_Bar") {
		t.Error("should contain Bar")
	}
}

func TestGenerateCallGraph_Empty(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	r, err := GenerateDiagram(tmp, DiagramCallGraph)
	if err != nil {
		t.Fatalf("callgraph empty: %v", err)
	}
	if r == nil {
		t.Fatal("result should not be nil")
	}
}

func TestGenerateCommandTree(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	cmdDir := filepath.Join(tmp, "cmd", "myapp")
	os.MkdirAll(cmdDir, 0o755)

	os.WriteFile(filepath.Join(cmdDir, "root.go"), []byte(`package main

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:   "myapp",
	Short: "My test app",
}

var subCmd = &cobra.Command{
	Use:   "sub",
	Short: "A subcommand",
}

func init() {
	rootCmd.AddCommand(subCmd)
}
`), 0o644)

	r, err := GenerateDiagram(tmp, DiagramCommandTree)
	if err != nil {
		t.Fatalf("commandtree: %v", err)
	}
	if !strings.Contains(r.Mermaid, "myapp") {
		t.Error("should contain myapp")
	}
	if !strings.Contains(r.Mermaid, "sub") {
		t.Error("should contain sub")
	}
	if !strings.Contains(r.Mermaid, "rootCmd --> subCmd") {
		t.Error("should show parent-child relationship")
	}
}

func TestGenerateCommandTree_NoCmdDir(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	_, err := GenerateDiagram(tmp, DiagramCommandTree)
	if err == nil {
		t.Error("should error when no cmd/ dir")
	}
}

func TestGenerateCommandWiring(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	cmdDir := filepath.Join(tmp, "cmd", "app")
	os.MkdirAll(cmdDir, 0o755)
	os.MkdirAll(filepath.Join(tmp, "internal", "foo"), 0o755)

	os.WriteFile(filepath.Join(cmdDir, "serve.go"), []byte(`package main

import "github.com/example/mymod/internal/foo"

var _ = foo.X
`), 0o644)

	// Write go.mod so module prefix is detectable
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module github.com/example/mymod\ngo 1.21\n"), 0o644)

	r, err := GenerateDiagram(tmp, DiagramCommandWiring)
	if err != nil {
		t.Fatalf("wiring: %v", err)
	}
	if !strings.Contains(r.Mermaid, "serve") {
		t.Error("should contain serve.go")
	}
	if !strings.Contains(r.Mermaid, "foo") {
		t.Error("should contain foo module")
	}
}

func TestGenerateModuleDataFlow(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	modDir := filepath.Join(tmp, "internal", "alpha")
	os.MkdirAll(modDir, 0o755)

	os.WriteFile(filepath.Join(modDir, "a.go"), []byte(`package alpha

import (
	"os"
	"encoding/json"
)

func Read() {
	os.ReadFile("test.txt")
	json.Unmarshal(nil, nil)
}
func Write() {
	os.WriteFile("out.txt", nil, 0644)
}
`), 0o644)

	r, err := GenerateDiagram(tmp, DiagramModuleDataFlow)
	if err != nil {
		t.Fatalf("dataflow: %v", err)
	}
	if !strings.Contains(r.Mermaid, "alpha") {
		t.Error("should contain alpha module")
	}
	if !strings.Contains(r.Mermaid, "Filesystem") {
		t.Error("should mention Filesystem")
	}
}

func TestGenerateCommitHeatmap(t *testing.T) {
	t.Parallel()
	// Only works in a git repo
	r, err := generateCommitHeatmap("/Users/thekryptodragon/Development/sirsi-pantheon")
	if err != nil {
		t.Skipf("not in git repo: %v", err)
	}
	if !strings.Contains(r.Mermaid, "xychart-beta") {
		t.Error("should use xychart-beta")
	}
	if !strings.Contains(r.Mermaid, "Commit Activity") {
		t.Error("should have title")
	}
}

func TestGenerateFileHotspots(t *testing.T) {
	t.Parallel()
	r, err := generateFileHotspots("/Users/thekryptodragon/Development/sirsi-pantheon")
	if err != nil {
		t.Skipf("not in git repo: %v", err)
	}
	if !strings.Contains(r.Mermaid, "xychart-beta") {
		t.Error("should use xychart-beta")
	}
}

func TestGenerateReleaseTimeline(t *testing.T) {
	t.Parallel()
	r, err := generateReleaseTimeline("/Users/thekryptodragon/Development/sirsi-pantheon")
	if err != nil {
		t.Skipf("not in git repo: %v", err)
	}
	if !strings.Contains(r.Mermaid, "timeline") {
		t.Error("should use timeline diagram")
	}
}

func TestGenerateDepTree(t *testing.T) {
	t.Parallel()
	r, err := generateDepTree("/Users/thekryptodragon/Development/sirsi-pantheon")
	if err != nil {
		t.Skipf("not in Go project: %v", err)
	}
	if !strings.Contains(r.Mermaid, "graph TD") {
		t.Error("should be a TD graph")
	}
}

func TestGenerateDepTree_FromGoMod(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte(`module example.com/test
go 1.21

require (
	github.com/spf13/cobra v1.8.0
	github.com/stretchr/testify v1.9.0
)
`), 0o644)

	r, err := generateDepTreeFromGoMod(tmp)
	if err != nil {
		t.Fatalf("deptree go.mod: %v", err)
	}
	if !strings.Contains(r.Mermaid, "cobra") {
		t.Error("should contain cobra dep")
	}
}

func TestGenerateCIPipeline(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	wfDir := filepath.Join(tmp, ".github", "workflows")
	os.MkdirAll(wfDir, 0o755)

	os.WriteFile(filepath.Join(wfDir, "ci.yml"), []byte(`name: CI
on: push
jobs:
  lint:
    runs-on: ubuntu-latest
  test:
    needs: lint
    runs-on: ubuntu-latest
  build:
    needs: [lint, test]
    runs-on: ubuntu-latest
`), 0o644)

	r, err := GenerateDiagram(tmp, DiagramCIPipeline)
	if err != nil {
		t.Fatalf("ci: %v", err)
	}
	if !strings.Contains(r.Mermaid, "lint") {
		t.Error("should contain lint job")
	}
	if !strings.Contains(r.Mermaid, "test") {
		t.Error("should contain test job")
	}
}

func TestGenerateCIPipeline_NoCI(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	_, err := GenerateDiagram(tmp, DiagramCIPipeline)
	if err == nil {
		t.Error("should error when no CI config found")
	}
}

// ── Helper tests ────────────────────────────────────────────────────

func TestSanitizeMermaidID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"alpha.Foo", "alpha_Foo"},
		{"a/b/c", "a_b_c"},
		{"1start", "n1start"},
		{"normal", "normal"},
	}
	for _, tt := range tests {
		got := sanitizeMermaidID(tt.input)
		if got != tt.want {
			t.Errorf("sanitize(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSimplifyModName(t *testing.T) {
	t.Parallel()
	if got := simplifyModName("github.com/foo/bar@v1.2.3"); got != "bar" {
		t.Errorf("simplify = %q", got)
	}
	if got := simplifyModName("github.com/foo/bar"); got != "bar" {
		t.Errorf("simplify = %q", got)
	}
}

func TestDetectModulePrefix(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/mymod\ngo 1.21\n"), 0o644)

	prefix := detectModulePrefix(tmp)
	if prefix != "example.com/mymod" {
		t.Errorf("prefix = %q", prefix)
	}
}

func TestAppendUnique(t *testing.T) {
	t.Parallel()
	s := appendUnique([]string{"a"}, "b")
	if len(s) != 2 {
		t.Errorf("len = %d", len(s))
	}
	s = appendUnique(s, "a")
	if len(s) != 2 {
		t.Errorf("duplicate added: len = %d", len(s))
	}
}
