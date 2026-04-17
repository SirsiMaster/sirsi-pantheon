package seba

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── Seba Diagram Engine Tests ───────────────────────────────────────

func TestAllDiagramTypes(t *testing.T) {
	t.Parallel()
	all := AllDiagramTypes()
	// 6 core + 9 registered = 15
	if len(all) < 15 {
		t.Errorf("AllDiagramTypes() = %d types, want at least 15", len(all))
	}
}

func TestGenerateDiagram_Hierarchy(t *testing.T) {
	t.Parallel()
	r, err := GenerateDiagram("", DiagramHierarchy)
	if err != nil {
		t.Fatalf("hierarchy: %v", err)
	}
	if r.Type != DiagramHierarchy {
		t.Errorf("type = %s", r.Type)
	}
	if !strings.Contains(r.Mermaid, "Ra") {
		t.Error("hierarchy should mention Ra")
	}
	if !strings.Contains(r.Mermaid, "Net") {
		t.Error("hierarchy should mention Net")
	}
	if !strings.Contains(r.Mermaid, "graph TD") {
		t.Error("hierarchy should be a TD graph")
	}
}

func TestGenerateDiagram_Memory(t *testing.T) {
	t.Parallel()
	r, err := GenerateDiagram("", DiagramMemory)
	if err != nil {
		t.Fatalf("memory: %v", err)
	}
	if !strings.Contains(r.Mermaid, "Thoth") {
		t.Error("memory should mention Thoth")
	}
	if !strings.Contains(r.Mermaid, "Knowledge Items") {
		t.Error("memory should mention Knowledge Items")
	}
}

func TestGenerateDiagram_Governance(t *testing.T) {
	t.Parallel()
	r, err := GenerateDiagram("", DiagramGovernance)
	if err != nil {
		t.Fatalf("governance: %v", err)
	}
	if !strings.Contains(r.Mermaid, "Ma'at") {
		t.Error("governance should mention Ma'at")
	}
	if !strings.Contains(r.Mermaid, "Isis") {
		t.Error("governance should mention Isis")
	}
	if !strings.Contains(r.Mermaid, "Pulse") {
		t.Error("governance should mention Pulse")
	}
}

func TestGenerateDiagram_Pipeline(t *testing.T) {
	t.Parallel()
	r, err := GenerateDiagram("", DiagramPipeline)
	if err != nil {
		t.Fatalf("pipeline: %v", err)
	}
	if !strings.Contains(r.Mermaid, "Pre-Push Gate") {
		t.Error("pipeline should mention Pre-Push Gate")
	}
	if !strings.Contains(r.Mermaid, "metrics.json") {
		t.Error("pipeline should mention metrics.json")
	}
}

func TestGenerateDiagram_Unknown(t *testing.T) {
	t.Parallel()
	_, err := GenerateDiagram("", DiagramType("bogus"))
	if err == nil {
		t.Error("unknown type should error")
	}
}

func TestGenerateDiagram_DataFlow(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Create a fake cmd/pantheon/ with deity files
	cmdDir := filepath.Join(tmp, "cmd", "sirsi")
	os.MkdirAll(cmdDir, 0o755)
	os.WriteFile(filepath.Join(cmdDir, "anubis.go"), []byte("package main"), 0o644)
	os.WriteFile(filepath.Join(cmdDir, "maat.go"), []byte("package main"), 0o644)

	r, err := GenerateDiagram(tmp, DiagramDataFlow)
	if err != nil {
		t.Fatalf("dataflow: %v", err)
	}
	if !strings.Contains(r.Mermaid, "Anubis") {
		t.Error("dataflow should discover anubis")
	}
	if !strings.Contains(r.Mermaid, "Maat") {
		t.Error("dataflow should discover maat")
	}
	if !strings.Contains(r.Mermaid, "Filesystem") {
		t.Error("dataflow should mention Filesystem")
	}
}

func TestGenerateDiagram_Modules(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Create fake internal modules with imports
	modA := filepath.Join(tmp, "internal", "alpha")
	modB := filepath.Join(tmp, "internal", "beta")
	os.MkdirAll(modA, 0o755)
	os.MkdirAll(modB, 0o755)

	os.WriteFile(filepath.Join(modA, "a.go"), []byte(`package alpha
import "github.com/SirsiMaster/sirsi-pantheon/internal/beta"
var _ = beta.X
`), 0o644)
	os.WriteFile(filepath.Join(modB, "b.go"), []byte("package beta\nvar X = 1\n"), 0o644)

	r, err := GenerateDiagram(tmp, DiagramModules)
	if err != nil {
		t.Fatalf("modules: %v", err)
	}
	if !strings.Contains(r.Mermaid, "alpha") {
		t.Error("modules should contain alpha")
	}
	if !strings.Contains(r.Mermaid, "beta") {
		t.Error("modules should contain beta")
	}
	if !strings.Contains(r.Mermaid, "alpha --> beta") {
		t.Error("modules should show alpha --> beta edge")
	}
}

func TestGenerateAllDiagrams(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	os.MkdirAll(filepath.Join(tmp, "internal", "foo"), 0o755)
	os.MkdirAll(filepath.Join(tmp, "cmd", "sirsi"), 0o755)

	results, err := GenerateAllDiagrams(tmp)
	if err != nil {
		t.Fatalf("GenerateAll: %v", err)
	}
	if len(results) < 6 {
		t.Errorf("expected at least 6 diagrams, got %d", len(results))
	}
}

func TestRenderDiagramsHTML(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	diagrams := []*DiagramResult{
		{Type: DiagramHierarchy, Title: "Test Hierarchy", Mermaid: "graph TD\n  A --> B"},
		{Type: DiagramPipeline, Title: "Test Pipeline", Mermaid: "graph LR\n  C --> D"},
	}

	outPath := filepath.Join(tmp, "diagrams.html")
	err := RenderDiagramsHTML(diagrams, outPath)
	if err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "Test Hierarchy") {
		t.Error("HTML should contain Test Hierarchy")
	}
	if !strings.Contains(content, "Test Pipeline") {
		t.Error("HTML should contain Test Pipeline")
	}
	if !strings.Contains(content, "mermaid") {
		t.Error("HTML should reference mermaid")
	}
	if !strings.Contains(content, "slide-") {
		t.Error("HTML should contain slide elements")
	}
}

func TestScanModuleDeps(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	modA := filepath.Join(tmp, "internal", "alpha")
	modB := filepath.Join(tmp, "internal", "beta")
	os.MkdirAll(modA, 0o755)
	os.MkdirAll(modB, 0o755)

	os.WriteFile(filepath.Join(modA, "a.go"), []byte(`package alpha
import "github.com/SirsiMaster/sirsi-pantheon/internal/beta"
var _ = beta.X
`), 0o644)
	os.WriteFile(filepath.Join(modB, "b.go"), []byte("package beta\nvar X = 1\n"), 0o644)

	deps, modules := scanModuleDeps(tmp)
	if len(modules) != 2 {
		t.Errorf("modules = %d, want 2", len(modules))
	}
	if len(deps) != 1 {
		t.Errorf("deps = %d, want 1", len(deps))
	}
	if len(deps) > 0 && (deps[0].From != "alpha" || deps[0].To != "beta") {
		t.Errorf("dep = %s -> %s, want alpha -> beta", deps[0].From, deps[0].To)
	}
}

func TestScanModuleDeps_Empty(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	deps, modules := scanModuleDeps(tmp)
	if len(deps) != 0 || len(modules) != 0 {
		t.Error("empty project should have no deps or modules")
	}
}

func TestDiscoverDeities(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	cmdDir := filepath.Join(tmp, "cmd", "sirsi")
	os.MkdirAll(cmdDir, 0o755)

	os.WriteFile(filepath.Join(cmdDir, "anubis.go"), []byte("package main"), 0o644)
	os.WriteFile(filepath.Join(cmdDir, "seba.go"), []byte("package main"), 0o644)

	found := discoverDeities(tmp)
	if len(found) != 2 {
		t.Errorf("found %d deities, want 2", len(found))
	}
}

func TestDiscoverDeities_Empty(t *testing.T) {
	t.Parallel()
	found := discoverDeities("/nonexistent")
	if len(found) != 0 {
		t.Errorf("found %d deities from bad path, want 0", len(found))
	}
}
