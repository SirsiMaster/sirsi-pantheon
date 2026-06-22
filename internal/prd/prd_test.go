package prd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// --- types / schema tests ---

func TestPRDRoundTrip(t *testing.T) {
	p := PRD{
		Agent:    "claude-test",
		App:      "TestApp",
		DoneWhen: "all tasks done or owner-gated",
		Tasks: []Task{
			{ID: "T1", Desc: "Do something", Status: StatusOpen},
			{ID: "T2", Desc: "Done thing", Status: StatusDone},
		},
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var back PRD
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.Agent != p.Agent || len(back.Tasks) != len(p.Tasks) {
		t.Fatalf("round-trip mismatch: got %+v", back)
	}
}

// --- agentPrefix tests ---

func TestAgentPrefix(t *testing.T) {
	cases := []struct {
		agent string
		want  string
	}{
		{"claude-pantheon", "P"},
		{"claude-finalwishes", "F"},
		{"claude-assiduous", "A"},
		{"claude-nexus", "N"},
		{"claude-home", "H"},
		{"unknown", "T"},
	}
	for _, c := range cases {
		got := agentPrefix(c.agent)
		if got != c.want {
			t.Errorf("agentPrefix(%q) = %q, want %q", c.agent, got, c.want)
		}
	}
}

// --- canon reader tests ---

func TestParseADRIndex(t *testing.T) {
	dir := t.TempDir()
	docsDir := filepath.Join(dir, "docs")
	if err := os.Mkdir(docsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `# ADR Index

| ID | Title | Status | Date |
|----|-------|--------|------|
| [ADR-001](ADR-001.md) | Founding Architecture | Accepted | 2026-01-01 |
| [ADR-021](ADR-021.md) | Deities Not Single-Repo | **Proposed** | 2026-05-31 |
| [ADR-027](ADR-027.md) | Router Menubar Surface | **Proposed** (routed) | 2026-06-08 |
`
	if err := os.WriteFile(filepath.Join(docsDir, "ADR-INDEX.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewCanonReader(dir)
	tasks, refs, err := r.parseADRIndex()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Fatalf("want 2 unshipped ADR tasks, got %d: %+v", len(tasks), tasks)
	}
	if tasks[0].ID != "ADR-021" {
		t.Errorf("want ADR-021, got %s", tasks[0].ID)
	}
	if tasks[1].ID != "ADR-027" {
		t.Errorf("want ADR-027, got %s", tasks[1].ID)
	}
	if len(refs) == 0 {
		t.Error("expected at least one canon ref")
	}
}

func TestParseArchGantt(t *testing.T) {
	dir := t.TempDir()
	docsDir := filepath.Join(dir, "docs")
	if err := os.Mkdir(docsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `# Architecture Design

## 5. Recommended Implementation Order ⚠️ MANDATORY (Neith's Triad §2)

` + "```mermaid" + `
gantt
    title Pantheon v1.0 Timeline
    dateFormat  YYYY-MM-DD
    section Core Stability
    Terminal Metric Overflow Fix      :active, a1, 2026-03-29, 1d
    LSP Renice Logic Audit            :a2, after a1, 2d
    section Release
    Fleet Validation Sweep            :a3, 2026-04-01, 3d
` + "```" + `

## 6. Key Decision Points
`
	if err := os.WriteFile(filepath.Join(docsDir, "ARCHITECTURE_DESIGN.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewCanonReader(dir)
	tasks, refs, err := r.parseArchitectureDocs()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 3 {
		t.Fatalf("want 3 gantt tasks, got %d: %+v", len(tasks), tasks)
	}
	if len(refs) == 0 {
		t.Error("expected at least one arch ref")
	}
}

func TestParseRoadmap(t *testing.T) {
	dir := t.TempDir()
	content := `# Hardening Roadmap

## 🛤 Phase 1: Interface Injection (COMPLETE ✅)

Some description.

## 🛤 Phase 2: Deity Hardening (ACTIVE 🚧)

More detail.

## 🛤 Phase 3: Production Hardening (NEXT)

Future work.
`
	if err := os.WriteFile(filepath.Join(dir, "PANTHEON_ROADMAP.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewCanonReader(dir)
	tasks, refs, err := r.parseRoadmapDocs()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 3 {
		t.Fatalf("want 3 roadmap tasks, got %d: %+v", len(tasks), tasks)
	}
	// Phase 1 is complete.
	if !tasks[0].Shipped {
		t.Errorf("Phase 1 should be marked shipped")
	}
	// Phases 2 and 3 are not complete.
	if tasks[1].Shipped || tasks[2].Shipped {
		t.Errorf("phases 2+3 should not be shipped")
	}
	if len(refs) == 0 {
		t.Error("expected at least one roadmap ref")
	}
}

// --- merge tests ---

func TestMergePreservesExistingTasks(t *testing.T) {
	existing := &PRD{
		Agent:    "claude-test",
		DoneWhen: "all tasks done or owner-gated",
		Tasks: []Task{
			{ID: "P1", Desc: "existing manual task", Status: StatusOwnerGated, Note: "waiting on owner"},
		},
	}
	derived := []DerivedTask{
		{ID: "ADR-042", Desc: "Brand new thing", Source: SourceADR},
	}
	checker := &ShipmentChecker{} // no git — everything unshipped
	out, result := mergeTasks(existing, derived, checker, "claude-test")

	if len(result.Kept) != 1 || result.Kept[0] != "P1" {
		t.Errorf("expected P1 kept, got %v", result.Kept)
	}
	if len(result.Added) != 1 {
		t.Errorf("expected 1 added, got %d: %v", len(result.Added), result.Added)
	}
	// P1 must retain its owner-gated status.
	var p1 Task
	for _, t := range out {
		if t.ID == "P1" {
			p1 = t
		}
	}
	if p1.Status != StatusOwnerGated {
		t.Errorf("P1 status changed from owner-gated to %s", p1.Status)
	}
}

func TestMergeSkipsDuplicateRef(t *testing.T) {
	existing := &PRD{
		Tasks: []Task{
			{ID: "P1", Desc: "Implement ADR-021", Ref: "ADR-021", Status: StatusOpen},
		},
	}
	derived := []DerivedTask{
		{ID: "ADR-021", Desc: "Implement ADR-021: Deities Not Single-Repo", Source: SourceADR},
	}
	checker := &ShipmentChecker{}
	_, result := mergeTasks(existing, derived, checker, "claude-test")

	if len(result.Added) != 0 {
		t.Errorf("expected 0 added (duplicate ref), got %d: %v", len(result.Added), result.Added)
	}
}

// --- FindRouterHome tests ---

func TestFindRouterHomeSuccess(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, ".agents", "idea-router")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Create a subdir to simulate running from within the repo.
	subdir := filepath.Join(dir, "internal", "prd")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.Chdir(subdir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir("/tmp") }) //nolint:errcheck

	home, err := FindRouterHome()
	if err != nil {
		t.Fatalf("FindRouterHome: %v", err)
	}
	// Resolve symlinks for comparison: /tmp is a symlink to /private/tmp on macOS.
	wantRaw := filepath.Join(dir, ".agents")
	want, _ := filepath.EvalSymlinks(wantRaw)
	if want == "" {
		want = wantRaw
	}
	got, _ := filepath.EvalSymlinks(home)
	if got == "" {
		got = home
	}
	if got != want {
		t.Errorf("FindRouterHome = %q, want %q", home, want)
	}
}

// --- status / keywords tests ---

func TestKeywordsFrom(t *testing.T) {
	cases := []struct {
		input string
		want  int // minimum expected keyword count
	}{
		{"Terminal Metric Overflow Fix", 3},
		{"Do x", 0},
		{"the and for via", 0},
	}
	for _, c := range cases {
		got := keywordsFrom(c.input)
		if len(got) < c.want {
			t.Errorf("keywordsFrom(%q): want >=%d, got %d: %v", c.input, c.want, len(got), got)
		}
	}
}

// --- write / load round-trip ---

func TestWriteAndLoadPRD(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "claude-test.json")

	p := &PRD{
		Agent:    "claude-test",
		DoneWhen: "all tasks done or owner-gated",
		Tasks:    []Task{{ID: "T1", Desc: "test", Status: StatusOpen}},
	}
	if err := writePRD(path, p); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadExistingPRD(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Agent != "claude-test" || len(loaded.Tasks) != 1 {
		t.Errorf("loaded mismatch: %+v", loaded)
	}
}
